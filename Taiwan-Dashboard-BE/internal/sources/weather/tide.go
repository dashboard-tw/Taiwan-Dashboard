package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/sources/cwa"
)

const DefaultTideURL = cwa.Datastore + "F-A0021-001"

type TideStation struct {
	ID       string
	Name     string
	Lat, Lon float64
}

type Tide struct {
	StationID string
	At        time.Time
	Tide      string // 滿潮、乾潮
	HeightCM  *int   // 相對當地平均海平面
	Range     string // 大、中、小
	Lunar     string
}

func ParseTides(body []byte) ([]TideStation, []Tide, []string, error) {
	var file struct {
		Success string `json:"success"`
		Records struct {
			TideForecasts []struct {
				Location struct {
					ID          string `json:"LocationId"`
					Name        string `json:"LocationName"`
					Lat         string `json:"Latitude"`
					Lon         string `json:"Longitude"`
					TimePeriods struct {
						Daily []struct {
							Lunar     string `json:"LunarDate"`
							TideRange string `json:"TideRange"`
							Time      []struct {
								DateTime string `json:"DateTime"`
								Tide     string `json:"Tide"`
								Heights  struct {
									LocalMSL json.Number `json:"AboveLocalMSL"`
								} `json:"TideHeights"`
							} `json:"Time"`
						} `json:"Daily"`
					} `json:"TimePeriods"`
				} `json:"Location"`
			} `json:"TideForecasts"`
		} `json:"records"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &file); err != nil {
		return nil, nil, nil, fmt.Errorf("decode tide forecast: %w", err)
	}
	if file.Success != "true" {
		return nil, nil, nil, errors.New("氣象署回應 success 不是 true（授權碼可能錯誤）")
	}
	var stations []TideStation
	var tides []Tide
	var warnings []string
	for _, f := range file.Records.TideForecasts {
		loc := f.Location
		id := strings.TrimSpace(loc.ID)
		lat, err1 := strconv.ParseFloat(strings.TrimSpace(loc.Lat), 64)
		lon, err2 := strconv.ParseFloat(strings.TrimSpace(loc.Lon), 64)
		if id == "" || err1 != nil || err2 != nil || lat < 20 || lat > 27 || lon < 117 || lon > 123 {
			warnings = append(warnings, fmt.Sprintf("潮汐地點「%s」的代碼或座標不正確", loc.Name))
			continue
		}
		stations = append(stations, TideStation{ID: id, Name: strings.TrimSpace(loc.Name), Lat: lat, Lon: lon})
		seen := map[time.Time]bool{}
		for _, d := range loc.TimePeriods.Daily {
			for _, t := range d.Time {
				at, err := time.Parse(time.RFC3339, strings.TrimSpace(t.DateTime))
				kind := strings.TrimSpace(t.Tide)
				if err != nil || (kind != "滿潮" && kind != "乾潮") || seen[at] {
					continue
				}
				seen[at] = true
				var h *int
				if v, err := strconv.Atoi(t.Heights.LocalMSL.String()); err == nil {
					h = &v
				}
				tides = append(tides, Tide{StationID: id, At: at, Tide: kind, HeightCM: h,
					Range: strings.TrimSpace(d.TideRange), Lunar: strings.TrimSpace(d.Lunar)})
			}
		}
	}
	return stations, tides, warnings, nil
}

type TidePipeline struct {
	URL    string
	APIKey string
}

func (p *TidePipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "tide_forecast",
		Name:         "潮汐預報",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://opendata.cwa.gov.tw/dataset/forecast/F-A0021-001",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   3 * 24 * time.Hour,
	}
}

func (p *TidePipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := cwa.Get(ctx, env, p.URL, p.APIKey)
	if err != nil {
		return res, err
	}
	stations, tides, warnings, err := ParseTides(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(tides)
	if len(stations) == 0 || len(tides) == 0 {
		return res, errors.New("潮汐預報是空的")
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	rows := make([][]any, len(stations))
	for i, s := range stations {
		rows[i] = []any{s.ID, s.Name, fmt.Sprint(s.Lon), fmt.Sprint(s.Lat)}
	}
	// 縣市取最近的縣市界（岸邊的點常落在界線外）
	if _, err := pipeline.InsertRows(ctx, tx, `
		INSERT INTO ref.tide_station (station_id, name, county_code, geom)
		SELECT v.id, v.name,
		       (SELECT c.code FROM ref.county c
		         WHERE ST_DWithin(c.boundary, v.g, 0.2)
		         ORDER BY c.boundary <-> v.g LIMIT 1),
		       v.g
		  FROM (SELECT id, name, ST_SetSRID(ST_MakePoint(lon::float8, lat::float8), 4326) AS g FROM (VALUES`, rows,
		`) AS r(id, name, lon, lat)) v
		ON CONFLICT (station_id) DO UPDATE SET name = EXCLUDED.name, county_code = EXCLUDED.county_code,
			geom = EXCLUDED.geom, updated_at = now()
		WHERE (ref.tide_station.name, ref.tide_station.county_code, ref.tide_station.geom)
		      IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.county_code, EXCLUDED.geom)`); err != nil {
		return res, fmt.Errorf("upsert tide stations: %w", err)
	}

	rows = make([][]any, len(tides))
	for i, t := range tides {
		rows[i] = []any{t.StationID, t.At, t.Tide, t.HeightCM, pipeline.NullIfEmpty(t.Range), pipeline.NullIfEmpty(t.Lunar)}
	}
	n, err := pipeline.InsertRows(ctx, tx,
		`INSERT INTO data.tide_forecast (station_id, at, tide, height_cm, tide_range, lunar_date) VALUES`, rows, `
		ON CONFLICT (station_id, at) DO UPDATE SET tide = EXCLUDED.tide, height_cm = EXCLUDED.height_cm,
			tide_range = EXCLUDED.tide_range, lunar_date = EXCLUDED.lunar_date
		WHERE (data.tide_forecast.tide, data.tide_forecast.height_cm, data.tide_forecast.tide_range, data.tide_forecast.lunar_date)
		      IS DISTINCT FROM (EXCLUDED.tide, EXCLUDED.height_cm, EXCLUDED.tide_range, EXCLUDED.lunar_date)`)
	if err != nil {
		return res, fmt.Errorf("upsert tides: %w", err)
	}
	res.RowsInserted = n
	if _, err := tx.ExecContext(ctx, `DELETE FROM data.tide_forecast WHERE at < $1`, env.Now().Add(-48*time.Hour)); err != nil {
		return res, err
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}
