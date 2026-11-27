package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/sources/cwa"
)

// 健康氣象：熱傷害（逐 3 小時）、冷傷害與溫差提醒（逐日），各鄉鎮未來 5 天
var DefaultHealthURLs = map[string]string{
	"heat":      cwa.Datastore + "M-A0085-001",
	"cold":      cwa.Datastore + "F-A0085-002",
	"temp_diff": cwa.Datastore + "F-A0085-004",
}

// index and warning field of each kind
var healthKeys = map[string][2]string{
	"heat":      {"HeatInjuryIndex", "HeatInjuryWarning"},
	"cold":      {"ColdInjuryIndex", "ColdInjuryWarning"},
	"temp_diff": {"", "TemperatureDifferenceWarning"},
}

type HealthRow struct {
	Geocode  string
	Kind     string
	StartsAt time.Time
	EndsAt   time.Time
	Index    *int
	MaxTemp  *int
	MinTemp  *int
	Warning  string
}

func parseCWATime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return pipeline.ParseTaipeiTime(s, time.DateTime)
}

func jsonInt(raw json.RawMessage) *int {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		n := int(v)
		return &n
	}
	return nil
}

// ParseHealth reads M-A0085-001, F-A0085-002 or F-A0085-004.
func ParseHealth(body []byte, kind string) ([]ForecastTown, []HealthRow, []string, error) {
	var file struct {
		Success string `json:"success"`
		Records struct {
			Locations []struct {
				County   string `json:"CountyName"`
				Location []struct {
					Town    string `json:"TownName"`
					Geocode string `json:"Geocode"`
					Lat     string `json:"Latitude"`
					Lon     string `json:"Longitude"`
					Time    []struct {
						IssueTime string                     `json:"IssueTime"`
						StartTime string                     `json:"StartTime"`
						EndTime   string                     `json:"EndTime"`
						Elements  map[string]json.RawMessage `json:"WeatherElements"`
					} `json:"Time"`
				} `json:"Location"`
			} `json:"Locations"`
		} `json:"records"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &file); err != nil {
		return nil, nil, nil, fmt.Errorf("decode health forecast: %w", err)
	}
	if file.Success != "true" {
		return nil, nil, nil, errors.New("氣象署回應 success 不是 true（授權碼可能錯誤）")
	}
	keys, ok := healthKeys[kind]
	if !ok {
		return nil, nil, nil, fmt.Errorf("unknown health kind %q", kind)
	}
	indexKey, warnKey := keys[0], keys[1]
	var towns []ForecastTown
	var rows []HealthRow
	var warnings []string
	for _, c := range file.Records.Locations {
		for _, loc := range c.Location {
			lat, err1 := strconv.ParseFloat(strings.TrimSpace(loc.Lat), 64)
			lon, err2 := strconv.ParseFloat(strings.TrimSpace(loc.Lon), 64)
			geocode := strings.TrimSpace(loc.Geocode)
			if geocode == "" || err1 != nil || err2 != nil || lat < 21 || lat > 27 || lon < 117 || lon > 123 {
				warnings = append(warnings, fmt.Sprintf("%s%s 的代碼或座標不正確", c.County, loc.Town))
				continue
			}
			towns = append(towns, ForecastTown{Geocode: geocode, County: strings.TrimSpace(c.County),
				Name: strings.TrimSpace(loc.Town), Lat: lat, Lon: lon})
			for _, t := range loc.Time {
				r := HealthRow{Geocode: geocode, Kind: kind}
				if t.IssueTime != "" {
					// 熱傷害是逐 3 小時的時間點
					at, err := parseCWATime(t.IssueTime)
					if err != nil {
						continue
					}
					r.StartsAt, r.EndsAt = at, at.Add(3*time.Hour)
				} else {
					start, err1 := parseCWATime(t.StartTime)
					end, err2 := parseCWATime(t.EndTime)
					if err1 != nil || err2 != nil || !end.After(start) {
						continue
					}
					r.StartsAt, r.EndsAt = start, end
				}
				if indexKey != "" {
					r.Index = jsonInt(t.Elements[indexKey])
				}
				r.MaxTemp = jsonInt(t.Elements["MaxTemperature"])
				r.MinTemp = jsonInt(t.Elements["MinTemperature"])
				var w string
				_ = json.Unmarshal(t.Elements[warnKey], &w)
				r.Warning = strings.TrimSpace(w)
				rows = append(rows, r)
			}
		}
	}
	return towns, rows, warnings, nil
}

type HealthPipeline struct {
	URLs   map[string]string // kind → URL
	APIKey string
}

func (p *HealthPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "health_weather",
		Name:         "健康氣象（熱傷害、冷傷害、溫差提醒）",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://opendata.cwa.gov.tw/dataset/forecast/M-A0085-001",
		UpdateFreq:   6 * time.Hour,
		StaleAfter:   24 * time.Hour,
	}
}

func (p *HealthPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	var towns []ForecastTown
	var rows []HealthRow
	kinds := make([]string, 0, len(p.URLs))
	for k := range p.URLs {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	for _, kind := range kinds {
		body, err := cwa.Get(ctx, env, p.URLs[kind], p.APIKey)
		if err != nil {
			return res, err
		}
		t, r, warnings, err := ParseHealth(body, kind)
		if err != nil {
			return res, err
		}
		if len(r) == 0 {
			return res, fmt.Errorf("%s 健康氣象是空的", kind)
		}
		towns, rows = append(towns, t...), append(rows, r...)
		res.Warnings = append(res.Warnings, warnings...)
	}
	res.RowsFetched = len(rows)

	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	known, err := upsertTowns(ctx, env, tx, towns, &res)
	if err != nil {
		return res, err
	}
	seen := map[string]bool{}
	var data [][]any
	for _, r := range rows {
		k := r.Geocode + r.Kind + r.StartsAt.String()
		if !known[r.Geocode] || seen[k] {
			continue
		}
		seen[k] = true
		data = append(data, []any{r.Geocode, r.Kind, r.StartsAt, r.EndsAt, r.Index, r.MaxTemp, r.MinTemp,
			pipeline.NullIfEmpty(r.Warning)})
	}
	n, err := pipeline.InsertRows(ctx, tx, `INSERT INTO data.town_health (geocode, kind, starts_at, ends_at, index_value,
		max_temp_c, min_temp_c, warning) VALUES`, data, `
		ON CONFLICT (geocode, kind, starts_at) DO UPDATE SET ends_at = EXCLUDED.ends_at,
			index_value = EXCLUDED.index_value, max_temp_c = EXCLUDED.max_temp_c, min_temp_c = EXCLUDED.min_temp_c,
			warning = EXCLUDED.warning, ingested_at = now()
		WHERE (data.town_health.ends_at, data.town_health.index_value, data.town_health.max_temp_c,
		       data.town_health.min_temp_c, data.town_health.warning)
		      IS DISTINCT FROM (EXCLUDED.ends_at, EXCLUDED.index_value, EXCLUDED.max_temp_c, EXCLUDED.min_temp_c,
		                        EXCLUDED.warning)`)
	if err != nil {
		return res, fmt.Errorf("upsert health: %w", err)
	}
	res.RowsInserted = n
	if _, err := tx.ExecContext(ctx, `DELETE FROM data.town_health WHERE ends_at < $1`,
		env.Now().Add(-48*time.Hour)); err != nil {
		return res, err
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}
