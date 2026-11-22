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

const DefaultForecastURL = cwa.Datastore + "F-C0032-001"

type CountyForecast struct {
	County      string // name as written by the CWA, e.g. 臺北市
	StartsAt    time.Time
	EndsAt      time.Time
	Weather     string
	WeatherCode *int
	RainChance  *int
	MinTemp     *int
	MaxTemp     *int
	Comfort     string
}

func ParseForecast(body []byte) ([]CountyForecast, error) {
	var file struct {
		Success string `json:"success"`
		Records struct {
			Location []struct {
				Name     string `json:"locationName"`
				Elements []struct {
					Name string `json:"elementName"`
					Time []struct {
						Start     string `json:"startTime"`
						End       string `json:"endTime"`
						Parameter struct {
							Name  string `json:"parameterName"`
							Value string `json:"parameterValue"`
						} `json:"parameter"`
					} `json:"time"`
				} `json:"weatherElement"`
			} `json:"location"`
		} `json:"records"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &file); err != nil {
		return nil, fmt.Errorf("decode county forecast: %w", err)
	}
	if file.Success != "true" {
		return nil, errors.New("氣象署回應 success 不是 true（授權碼可能錯誤）")
	}
	layout := "2006-01-02 15:04:05"
	integer := func(s string) *int {
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return nil
		}
		return &v
	}
	var out []CountyForecast
	for _, loc := range file.Records.Location {
		byStart := map[string]*CountyForecast{}
		var order []string
		for _, e := range loc.Elements {
			for _, t := range e.Time {
				f, ok := byStart[t.Start]
				if !ok {
					start, err1 := pipeline.ParseTaipeiTime(t.Start, layout)
					end, err2 := pipeline.ParseTaipeiTime(t.End, layout)
					if err1 != nil || err2 != nil || !end.After(start) {
						return nil, fmt.Errorf("%s 預報的時間「%s–%s」無法解析", loc.Name, t.Start, t.End)
					}
					f = &CountyForecast{County: strings.TrimSpace(loc.Name), StartsAt: start, EndsAt: end}
					byStart[t.Start] = f
					order = append(order, t.Start)
				}
				p := t.Parameter
				switch e.Name {
				case "Wx":
					f.Weather = strings.TrimSpace(p.Name)
					f.WeatherCode = integer(p.Value)
				case "PoP":
					if v := integer(p.Name); v != nil && *v >= 0 && *v <= 100 {
						f.RainChance = v
					}
				case "MinT":
					f.MinTemp = integer(p.Name)
				case "MaxT":
					f.MaxTemp = integer(p.Name)
				case "CI":
					f.Comfort = strings.TrimSpace(p.Name)
				}
			}
		}
		for _, k := range order {
			if byStart[k].Weather != "" {
				out = append(out, *byStart[k])
			}
		}
	}
	return out, nil
}

type ForecastPipeline struct {
	URL    string
	APIKey string
}

func (p *ForecastPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "county_forecast",
		Name:         "縣市 36 小時天氣預報",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://opendata.cwa.gov.tw/dataset/forecast/F-C0032-001",
		UpdateFreq:   6 * time.Hour,
		StaleAfter:   15 * time.Hour,
	}
}

func (p *ForecastPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := cwa.Get(ctx, env, p.URL, p.APIKey)
	if err != nil {
		return res, err
	}
	list, err := ParseForecast(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, errors.New("預報沒有任何縣市")
	}
	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	unknown := map[string]bool{}
	for _, f := range list {
		code, ok := counties.Code(f.County)
		if !ok {
			if !unknown[f.County] {
				unknown[f.County] = true
				res.Warnf("縣市「%s」對不到縣市代碼", f.County)
			}
			continue
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.county_forecast (county_code, starts_at, ends_at, weather, weather_code, rain_chance_pct,
			                                  min_temp_c, max_temp_c, comfort)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (county_code, starts_at) DO UPDATE SET
				ends_at = EXCLUDED.ends_at, weather = EXCLUDED.weather, weather_code = EXCLUDED.weather_code,
				rain_chance_pct = EXCLUDED.rain_chance_pct, min_temp_c = EXCLUDED.min_temp_c,
				max_temp_c = EXCLUDED.max_temp_c, comfort = EXCLUDED.comfort, ingested_at = now()
			WHERE (data.county_forecast.ends_at, data.county_forecast.weather, data.county_forecast.weather_code,
			       data.county_forecast.rain_chance_pct, data.county_forecast.min_temp_c,
			       data.county_forecast.max_temp_c, data.county_forecast.comfort)
			      IS DISTINCT FROM (EXCLUDED.ends_at, EXCLUDED.weather, EXCLUDED.weather_code, EXCLUDED.rain_chance_pct,
			                        EXCLUDED.min_temp_c, EXCLUDED.max_temp_c, EXCLUDED.comfort)`,
			code, f.StartsAt, f.EndsAt, f.Weather, f.WeatherCode, f.RainChance, f.MinTemp, f.MaxTemp,
			pipeline.NullIfEmpty(f.Comfort))
		if err != nil {
			return res, fmt.Errorf("upsert forecast %s %v: %w", f.County, f.StartsAt, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	t := list[0].StartsAt
	if now := env.Now(); t.After(now) {
		t = now
	}
	res.ObserveTime(t)
	return res, tx.Commit()
}
