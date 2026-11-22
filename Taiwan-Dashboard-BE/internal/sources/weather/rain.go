package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/sources/cwa"
)

const DefaultRainURL = cwa.Datastore + "O-A0002-001"

const maxRainMm = 5000

type RainObservation struct {
	StationID  string
	Name       string
	CountyCode string
	Town       string
	Altitude   *float64
	Maintainer string
	Lon, Lat   *float64
	ObservedAt time.Time
	Values     map[string]*float64
}

var rainPeriods = []string{"Now", "Past10Min", "Past1hr", "Past3hr", "Past6Hr", "Past12hr", "Past24hr", "Past2days", "Past3days"}

func ParseRain(body []byte) ([]RainObservation, []string, error) {
	var file struct {
		Success string `json:"success"`
		Records struct {
			Station []struct {
				StationName string `json:"StationName"`
				StationID   string `json:"StationId"`
				Maintainer  string `json:"Maintainer"`
				ObsTime     struct {
					DateTime string `json:"DateTime"`
				} `json:"ObsTime"`
				GeoInfo struct {
					Coordinates []struct {
						CoordinateName string `json:"CoordinateName"`
						Lat            string `json:"StationLatitude"`
						Lon            string `json:"StationLongitude"`
					} `json:"Coordinates"`
					Altitude   string `json:"StationAltitude"`
					CountyCode string `json:"CountyCode"`
					TownName   string `json:"TownName"`
				} `json:"GeoInfo"`
				RainfallElement map[string]struct {
					Precipitation json.Number `json:"Precipitation"`
				} `json:"RainfallElement"`
			} `json:"Station"`
		} `json:"records"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &file); err != nil {
		return nil, nil, fmt.Errorf("decode rainfall: %w", err)
	}
	if file.Success != "true" {
		return nil, nil, errors.New("氣象署回應 success 不是 true（授權碼可能錯誤）")
	}
	var out []RainObservation
	var warnings []string
	faults := 0
	for _, s := range file.Records.Station {
		o := RainObservation{
			StationID: strings.TrimSpace(s.StationID), Name: strings.TrimSpace(s.StationName),
			CountyCode: strings.TrimSpace(s.GeoInfo.CountyCode), Town: strings.TrimSpace(s.GeoInfo.TownName),
			Maintainer: strings.TrimSpace(s.Maintainer), Values: map[string]*float64{},
		}
		if o.StationID == "" || len(o.StationID) > 20 {
			continue
		}
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(s.ObsTime.DateTime))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("雨量站 %s 的時間「%s」無法解析", o.StationID, s.ObsTime.DateTime))
			continue
		}
		o.ObservedAt = t
		for _, c := range s.GeoInfo.Coordinates {
			if c.CoordinateName != "WGS84" {
				continue
			}
			lat, err1 := pipeline.ParseNumber(c.Lat)
			lon, err2 := pipeline.ParseNumber(c.Lon)
			if err1 == nil && err2 == nil && lat != nil && lon != nil && *lat > 20 && *lat < 27 && *lon > 116 && *lon < 123 {
				o.Lat, o.Lon = lat, lon
			}
		}
		if v, err := pipeline.ParseNumber(s.GeoInfo.Altitude); err == nil {
			o.Altitude = v
		}
		for _, k := range rainPeriods {
			v, err := pipeline.ParseNumber(s.RainfallElement[k].Precipitation.String())
			if err != nil || v == nil {
				continue
			}
			if *v < 0 || *v > maxRainMm {
				if *v > maxRainMm || *v < -999 {
					faults++
				}
				continue
			}
			o.Values[k] = v
		}
		out = append(out, o)
	}
	if faults > 0 {
		warnings = append(warnings, fmt.Sprintf("%d 個雨量值超出合理範圍，不存", faults))
	}
	return out, warnings, nil
}

type RainPipeline struct {
	URL    string
	APIKey string
}

func (p *RainPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "rain_obs",
		Name:         "雨量（每 10 分鐘）",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://opendata.cwa.gov.tw/dataset/observation/O-A0002-001",
		UpdateFreq:   10 * time.Minute,
		StaleAfter:   time.Hour,
	}
}

const rainColumns = `today_mm, past10min_mm, past1hr_mm, past3hr_mm, past6hr_mm, past12hr_mm, past24hr_mm, past2days_mm, past3days_mm`

func (p *RainPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := cwa.Get(ctx, env, p.URL, p.APIKey)
	if err != nil {
		return res, err
	}
	obs, warnings, err := ParseRain(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(obs)
	if len(obs) == 0 {
		return res, errors.New("雨量資料沒有任何測站")
	}

	codes := map[string]bool{}
	rows, err := env.DB.QueryContext(ctx, `SELECT code FROM ref.county`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return res, err
		}
		codes[c] = true
	}
	rows.Close()

	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	cutoff := env.Now().Add(15 * time.Minute)
	for _, o := range obs {
		if o.ObservedAt.After(cutoff) {
			continue
		}
		var county any
		if codes[o.CountyCode] {
			county = o.CountyCode
		}
		var lon, lat any
		if o.Lon != nil {
			lon, lat = *o.Lon, *o.Lat
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ref.rain_station (station_id, name, county_code, town, altitude_m, maintainer, geom)
			VALUES ($1, $2, $3, $4, $5, $6, CASE WHEN $7::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($7, $8), 4326) END)
			ON CONFLICT (station_id) DO UPDATE SET
				name = EXCLUDED.name, county_code = EXCLUDED.county_code, town = EXCLUDED.town,
				altitude_m = EXCLUDED.altitude_m, maintainer = EXCLUDED.maintainer, geom = EXCLUDED.geom, updated_at = now()
			WHERE (ref.rain_station.name, ref.rain_station.county_code, ref.rain_station.town, ref.rain_station.geom)
			      IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.county_code, EXCLUDED.town, EXCLUDED.geom)`,
			o.StationID, o.Name, county, pipeline.NullIfEmpty(o.Town), o.Altitude, pipeline.NullIfEmpty(o.Maintainer), lon, lat); err != nil {
			return res, fmt.Errorf("upsert rain station %s: %w", o.StationID, err)
		}
		args := []any{o.StationID, o.ObservedAt}
		for _, k := range rainPeriods {
			args = append(args, o.Values[k])
		}
		values := `($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO data.rain_obs_current (station_id, observed_at, `+rainColumns+`) VALUES `+values+`
			ON CONFLICT (station_id) DO UPDATE SET observed_at = EXCLUDED.observed_at,
				today_mm = EXCLUDED.today_mm, past10min_mm = EXCLUDED.past10min_mm, past1hr_mm = EXCLUDED.past1hr_mm,
				past3hr_mm = EXCLUDED.past3hr_mm, past6hr_mm = EXCLUDED.past6hr_mm, past12hr_mm = EXCLUDED.past12hr_mm,
				past24hr_mm = EXCLUDED.past24hr_mm, past2days_mm = EXCLUDED.past2days_mm,
				past3days_mm = EXCLUDED.past3days_mm, ingested_at = now()
			WHERE data.rain_obs_current.observed_at < EXCLUDED.observed_at`, args...); err != nil {
			return res, fmt.Errorf("upsert current rain %s: %w", o.StationID, err)
		}
		if o.ObservedAt.Minute() == 0 {
			r, err := tx.ExecContext(ctx, `
				INSERT INTO data.rain_obs_history (station_id, observed_at, `+rainColumns+`) VALUES `+values+`
				ON CONFLICT (station_id, observed_at) DO NOTHING`, args...)
			if err != nil {
				return res, fmt.Errorf("insert rain history %s: %w", o.StationID, err)
			}
			n, _ := r.RowsAffected()
			res.RowsInserted += int(n)
		}
		res.ObserveTime(o.ObservedAt)
	}
	return res, tx.Commit()
}
