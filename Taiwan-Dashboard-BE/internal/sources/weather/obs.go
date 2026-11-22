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

const (
	DefaultObsHourlyURL = cwa.Datastore + "O-A0001-001"
	DefaultObsTenMinURL = cwa.Datastore + "O-A0003-001"
)

type WeatherObservation struct {
	StationID  string
	Name       string
	CountyCode string
	Town       string
	Altitude   *float64
	Lon, Lat   *float64
	ObservedAt time.Time
	Weather    string
	TempC      *float64
	Humidity   *float64
	Pressure   *float64
	WindDir    *float64
	WindMS     *float64
	GustMS     *float64
	HighC      *float64
	LowC       *float64
}

func (o WeatherObservation) Kind() string {
	if strings.HasPrefix(o.StationID, "46") {
		return "manned"
	}
	return "auto"
}

var obsRanges = map[string][2]float64{
	"temp": {-30, 48}, "humidity": {0, 100}, "pressure": {500, 1100},
	"dir": {0, 360}, "wind": {0, 90}, "gust": {0, 120},
}

func ParseObservations(body []byte) ([]WeatherObservation, []string, error) {
	type temp struct {
		TemperatureInfo struct {
			AirTemperature json.Number `json:"AirTemperature"`
		} `json:"TemperatureInfo"`
	}
	var file struct {
		Success string `json:"success"`
		Records struct {
			Station []struct {
				StationName string `json:"StationName"`
				StationID   string `json:"StationId"`
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
				Element struct {
					Weather          string      `json:"Weather"`
					WindDirection    json.Number `json:"WindDirection"`
					WindSpeed        json.Number `json:"WindSpeed"`
					AirTemperature   json.Number `json:"AirTemperature"`
					RelativeHumidity json.Number `json:"RelativeHumidity"`
					AirPressure      json.Number `json:"AirPressure"`
					GustInfo         struct {
						PeakGustSpeed json.Number `json:"PeakGustSpeed"`
					} `json:"GustInfo"`
					DailyExtreme struct {
						DailyHigh temp `json:"DailyHigh"`
						DailyLow  temp `json:"DailyLow"`
					} `json:"DailyExtreme"`
				} `json:"WeatherElement"`
			} `json:"Station"`
		} `json:"records"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &file); err != nil {
		return nil, nil, fmt.Errorf("decode weather observations: %w", err)
	}
	if file.Success != "true" {
		return nil, nil, errors.New("氣象署回應 success 不是 true（授權碼可能錯誤）")
	}
	var out []WeatherObservation
	var warnings []string
	faults := 0
	value := func(s, kind string) *float64 {
		v, err := pipeline.ParseNumber(s)
		if err != nil || v == nil || *v == -99 || *v == -999 {
			return nil
		}
		r, ok := obsRanges[kind]
		if ok && (*v < r[0] || *v > r[1]) {
			faults++
			return nil
		}
		return v
	}
	for _, s := range file.Records.Station {
		o := WeatherObservation{
			StationID: strings.TrimSpace(s.StationID), Name: strings.TrimSpace(s.StationName),
			CountyCode: strings.TrimSpace(s.GeoInfo.CountyCode), Town: strings.TrimSpace(s.GeoInfo.TownName),
		}
		if o.StationID == "" || len(o.StationID) > 20 || o.Name == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(s.ObsTime.DateTime))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("測站 %s 的時間「%s」無法解析", o.StationID, s.ObsTime.DateTime))
			continue
		}
		o.ObservedAt = t
		for _, c := range s.GeoInfo.Coordinates {
			if c.CoordinateName != "WGS84" {
				continue
			}
			lat, err1 := pipeline.ParseNumber(c.Lat)
			lon, err2 := pipeline.ParseNumber(c.Lon)
			if err1 == nil && err2 == nil && lat != nil && lon != nil && *lat > 10 && *lat < 27 && *lon > 114 && *lon < 123 {
				o.Lat, o.Lon = lat, lon
			}
		}
		if v, err := pipeline.ParseNumber(s.GeoInfo.Altitude); err == nil {
			o.Altitude = v
		}
		e := s.Element
		if w := strings.TrimSpace(e.Weather); w != "" && w != "-99" {
			o.Weather = w
		}
		o.TempC = value(e.AirTemperature.String(), "temp")
		o.Humidity = value(e.RelativeHumidity.String(), "humidity")
		o.Pressure = value(e.AirPressure.String(), "pressure")
		o.WindDir = value(e.WindDirection.String(), "dir")
		o.WindMS = value(e.WindSpeed.String(), "wind")
		o.GustMS = value(e.GustInfo.PeakGustSpeed.String(), "gust")
		o.HighC = value(e.DailyExtreme.DailyHigh.TemperatureInfo.AirTemperature.String(), "temp")
		o.LowC = value(e.DailyExtreme.DailyLow.TemperatureInfo.AirTemperature.String(), "temp")
		out = append(out, o)
	}
	if faults > 0 {
		warnings = append(warnings, fmt.Sprintf("%d 個觀測值超出合理範圍，不存", faults))
	}
	return out, warnings, nil
}

type ObsPipeline struct {
	HourlyURL string
	TenMinURL string
	APIKey    string
}

func (p *ObsPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "weather_obs",
		Name:         "天氣觀測（氣溫、濕度、風）",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://opendata.cwa.gov.tw/dataset/observation/O-A0001-001",
		UpdateFreq:   10 * time.Minute,
		StaleAfter:   2 * time.Hour,
	}
}

const obsColumns = `weather, temp_c, humidity_pct, pressure_hpa, wind_dir_deg, wind_ms, gust_ms, high_c, low_c`

func (p *ObsPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	latest := map[string]WeatherObservation{}
	for _, url := range []string{p.HourlyURL, p.TenMinURL} {
		body, err := cwa.Get(ctx, env, url, p.APIKey)
		if err != nil {
			return res, err
		}
		obs, warnings, err := ParseObservations(body)
		if err != nil {
			return res, err
		}
		res.Warnings = append(res.Warnings, warnings...)
		for _, o := range obs {
			if old, ok := latest[o.StationID]; !ok || o.ObservedAt.After(old.ObservedAt) {
				latest[o.StationID] = o
			}
		}
	}
	res.RowsFetched = len(latest)
	if len(latest) == 0 {
		return res, errors.New("天氣觀測資料沒有任何測站")
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
	for _, o := range latest {
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
			INSERT INTO ref.weather_station (station_id, name, kind, county_code, town, altitude_m, geom)
			VALUES ($1, $2, $3, $4, $5, $6, CASE WHEN $7::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($7, $8), 4326) END)
			ON CONFLICT (station_id) DO UPDATE SET
				name = EXCLUDED.name, kind = EXCLUDED.kind, county_code = EXCLUDED.county_code, town = EXCLUDED.town,
				altitude_m = EXCLUDED.altitude_m, geom = EXCLUDED.geom, updated_at = now()
			WHERE (ref.weather_station.name, ref.weather_station.county_code, ref.weather_station.town,
			       ref.weather_station.altitude_m, ref.weather_station.geom)
			      IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.county_code, EXCLUDED.town, EXCLUDED.altitude_m, EXCLUDED.geom)`,
			o.StationID, o.Name, o.Kind(), county, pipeline.NullIfEmpty(o.Town), o.Altitude, lon, lat); err != nil {
			return res, fmt.Errorf("upsert weather station %s: %w", o.StationID, err)
		}
		args := []any{o.StationID, o.ObservedAt, pipeline.NullIfEmpty(o.Weather), o.TempC, o.Humidity, o.Pressure,
			o.WindDir, o.WindMS, o.GustMS, o.HighC, o.LowC}
		values := `(station_id, observed_at, ` + obsColumns + `) VALUES (` + pipeline.Placeholders(len(args)) + `)`
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO data.weather_obs_current `+values+`
			ON CONFLICT (station_id) DO UPDATE SET observed_at = EXCLUDED.observed_at, `+
			pipeline.SetExcluded(strings.Split(obsColumns, ", "))+`, ingested_at = now()
			WHERE data.weather_obs_current.observed_at < EXCLUDED.observed_at`, args...); err != nil {
			return res, fmt.Errorf("upsert current weather %s: %w", o.StationID, err)
		}
		if o.ObservedAt.Minute() == 0 {
			r, err := tx.ExecContext(ctx, `
				INSERT INTO data.weather_obs_history `+values+`
				ON CONFLICT (station_id, observed_at) DO NOTHING`, args...)
			if err != nil {
				return res, fmt.Errorf("insert weather history %s: %w", o.StationID, err)
			}
			n, _ := r.RowsAffected()
			res.RowsInserted += int(n)
		}
		res.ObserveTime(o.ObservedAt)
	}
	return res, tx.Commit()
}
