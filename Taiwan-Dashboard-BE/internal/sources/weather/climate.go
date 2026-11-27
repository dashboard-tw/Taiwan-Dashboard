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

const (
	DefaultNormalURL    = cwa.Datastore + "C-B0027-001"
	DefaultRainDailyURL = cwa.Datastore + "C-B0025-001"
)

type ClimateNormal struct {
	StationID string
	Name      string
	Month     int
	TempMean  *float64
	TempMax   *float64
	TempMin   *float64
	RainMM    *float64
	RainDays  *float64
	StartYear *int
	EndYear   *int
}

func num(s string) *float64 {
	v, err := pipeline.ParseNumber(s)
	if err != nil {
		return nil
	}
	return v
}

func ParseNormals(body []byte) ([]ClimateNormal, error) {
	type monthly struct {
		Month        string `json:"Month"`
		Mean         string `json:"Mean"`
		Maximum      string `json:"Maximum"`
		Minimum      string `json:"Minimum"`
		Accumulation string `json:"Accumulation"`
		GE01Days     string `json:"GE01Days"`
	}
	type element struct {
		StartYear int       `json:"StationStartYear"`
		EndYear   int       `json:"StationEndYear"`
		Monthly   []monthly `json:"monthly"`
	}
	var file struct {
		Success string `json:"success"`
		Records struct {
			Data struct {
				SurfaceObs struct {
					Location []struct {
						Station struct {
							ID   string `json:"StationID"`
							Name string `json:"StationName"`
						} `json:"station"`
						Stats map[string]element `json:"stationObsStatistics"`
					} `json:"location"`
				} `json:"surfaceObs"`
			} `json:"data"`
		} `json:"records"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &file); err != nil {
		return nil, fmt.Errorf("decode climate normals: %w", err)
	}
	if file.Success != "true" {
		return nil, errors.New("氣象署回應 success 不是 true（授權碼可能錯誤）")
	}
	var out []ClimateNormal
	for _, loc := range file.Records.Data.SurfaceObs.Location {
		id := strings.TrimSpace(loc.Station.ID)
		if id == "" {
			continue
		}
		byMonth := map[int]*ClimateNormal{}
		get := func(m string) *ClimateNormal {
			n, err := strconv.Atoi(strings.TrimSpace(m))
			if err != nil || n < 1 || n > 12 {
				return nil
			}
			if byMonth[n] == nil {
				byMonth[n] = &ClimateNormal{StationID: id, Name: strings.TrimSpace(loc.Station.Name), Month: n}
			}
			return byMonth[n]
		}
		years := func(e element, c *ClimateNormal) {
			if e.StartYear > 0 && e.EndYear >= e.StartYear {
				s, en := e.StartYear, e.EndYear
				c.StartYear, c.EndYear = &s, &en
			}
		}
		for _, m := range loc.Stats["AirTemperature"].Monthly {
			if c := get(m.Month); c != nil {
				c.TempMean, c.TempMax, c.TempMin = num(m.Mean), num(m.Maximum), num(m.Minimum)
				years(loc.Stats["AirTemperature"], c)
			}
		}
		for _, m := range loc.Stats["Precipitation"].Monthly {
			if c := get(m.Month); c != nil {
				c.RainMM, c.RainDays = num(m.Accumulation), num(m.GE01Days)
				if c.StartYear == nil {
					years(loc.Stats["Precipitation"], c)
				}
			}
		}
		for m := 1; m <= 12; m++ {
			if byMonth[m] != nil {
				out = append(out, *byMonth[m])
			}
		}
	}
	return out, nil
}

type DailyRain struct {
	StationID string
	Name      string
	Date      time.Time
	MM        *float64
}

func ParseDailyRain(body []byte) ([]DailyRain, error) {
	var file struct {
		Success string `json:"success"`
		Records struct {
			Location []struct {
				Station struct {
					ID   string `json:"StationID"`
					Name string `json:"StationName"`
				} `json:"station"`
				Times struct {
					List []struct {
						Date     string `json:"Date"`
						Elements struct {
							Precipitation string `json:"Precipitation"`
						} `json:"weatherElements"`
					} `json:"stationObsTime"`
				} `json:"stationObsTimes"`
			} `json:"location"`
		} `json:"records"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &file); err != nil {
		return nil, fmt.Errorf("decode daily rain: %w", err)
	}
	if file.Success != "true" {
		return nil, errors.New("氣象署回應 success 不是 true（授權碼可能錯誤）")
	}
	var out []DailyRain
	for _, loc := range file.Records.Location {
		id := strings.TrimSpace(loc.Station.ID)
		for _, t := range loc.Times.List {
			d, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(t.Date), pipeline.Taipei)
			if id == "" || err != nil {
				continue
			}
			r := DailyRain{StationID: id, Name: strings.TrimSpace(loc.Station.Name), Date: d}
			switch v := strings.TrimSpace(t.Elements.Precipitation); v {
			case "T": // 微量
				zero := 0.0
				r.MM = &zero
			default:
				if f := num(v); f != nil && *f >= 0 {
					r.MM = f
				}
			}
			out = append(out, r)
		}
	}
	return out, nil
}

type ClimatePipeline struct {
	NormalURL string
	RainURL   string
	APIKey    string
}

func (p *ClimatePipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "climate",
		Name:         "氣象站平年值與今年每日雨量",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://opendata.cwa.gov.tw/dataset/climate/C-B0025-001",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   5 * 24 * time.Hour,
	}
}

func (p *ClimatePipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := cwa.Get(ctx, env, p.NormalURL, p.APIKey)
	if err != nil {
		return res, err
	}
	normals, err := ParseNormals(body)
	if err != nil {
		return res, err
	}
	body, err = cwa.Get(ctx, env, p.RainURL, p.APIKey)
	if err != nil {
		return res, err
	}
	rain, err := ParseDailyRain(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(normals) + len(rain)
	if len(normals) == 0 || len(rain) == 0 {
		return res, errors.New("平年值或每日雨量是空的")
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	rows := make([][]any, len(normals))
	for i, n := range normals {
		rows[i] = []any{n.StationID, n.Name, n.Month, n.TempMean, n.TempMax, n.TempMin, n.RainMM, n.RainDays,
			n.StartYear, n.EndYear}
	}
	cols := []string{"station_name", "temp_mean_c", "temp_max_c", "temp_min_c", "rain_mm", "rain_days", "start_year", "end_year"}
	n, err := pipeline.InsertRows(ctx, tx, `INSERT INTO data.climate_normal (station_id, station_name, month,
		temp_mean_c, temp_max_c, temp_min_c, rain_mm, rain_days, start_year, end_year) VALUES`, rows,
		"ON CONFLICT (station_id, month) DO UPDATE SET "+pipeline.SetExcluded(cols)+
			" WHERE ("+prefixed("data.climate_normal.", cols)+") IS DISTINCT FROM ("+prefixed("EXCLUDED.", cols)+")")
	if err != nil {
		return res, fmt.Errorf("upsert climate normals: %w", err)
	}
	res.RowsInserted += n

	var latest time.Time
	rows = rows[:0]
	for _, r := range rain {
		rows = append(rows, []any{r.StationID, r.Name, r.Date.Format(time.DateOnly), r.MM})
		if r.MM != nil && r.Date.After(latest) {
			latest = r.Date
		}
	}
	n, err = pipeline.InsertRows(ctx, tx, `INSERT INTO data.rain_daily (station_id, station_name, date, rain_mm) VALUES`,
		rows, `ON CONFLICT (station_id, date) DO UPDATE SET station_name = EXCLUDED.station_name, rain_mm = EXCLUDED.rain_mm
		WHERE (data.rain_daily.station_name, data.rain_daily.rain_mm) IS DISTINCT FROM (EXCLUDED.station_name, EXCLUDED.rain_mm)`)
	if err != nil {
		return res, fmt.Errorf("upsert daily rain: %w", err)
	}
	res.RowsInserted += n
	if !latest.IsZero() {
		res.ObserveTime(latest)
	}
	return res, tx.Commit()
}

func prefixed(prefix string, cols []string) string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = prefix + c
	}
	return strings.Join(out, ", ")
}
