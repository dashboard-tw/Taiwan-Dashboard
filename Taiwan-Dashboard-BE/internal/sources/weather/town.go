package weather

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/sources/cwa"
)

// F-D0047-093 returns the township forecasts of up to five counties in one call.
const DefaultTownForecastURL = cwa.Datastore + "F-D0047-093"

const townBatch = 5

// TownForecastIDs lists the per-county datasets in batches of five: F-D0047-001 (宜蘭縣 3 天),
// -003 (宜蘭縣 1 週), -005 … -087.
func TownForecastIDs(week bool) []string {
	ids := make([]string, 22)
	for i := range ids {
		n := 1 + 4*i
		if week {
			n += 2
		}
		ids[i] = fmt.Sprintf("F-D0047-%03d", n)
	}
	var batches []string
	for start := 0; start < len(ids); start += townBatch {
		batches = append(batches, strings.Join(ids[start:min(start+townBatch, len(ids))], ","))
	}
	return batches
}

type ForecastTown struct {
	Geocode  string
	County   string
	Name     string
	Lat, Lon float64
}

// TownPeriod is one forecast period of one township. Fields not given by the source stay nil.
type TownPeriod struct {
	Geocode     string
	StartsAt    time.Time
	EndsAt      time.Time
	Weather     string
	WeatherCode *int
	RainChance  *int
	Temp        *int // 3-day: temperature at the start of the period
	Apparent    *int
	MaxTemp     *int // week
	MinTemp     *int
	MaxApparent *int
	MinApparent *int
	Humidity    *int
	UVI         *int
	UVLevel     string
	WindMS      *int
	Beaufort    *int
	WindDir     string
	Comfort     string
	Description string
}

type townValue map[string]string

type townElement struct {
	Name string `json:"ElementName"`
	Time []struct {
		DataTime  string      `json:"DataTime"`
		StartTime string      `json:"StartTime"`
		EndTime   string      `json:"EndTime"`
		Value     []townValue `json:"ElementValue"`
	} `json:"Time"`
}

func intValue(s string) *int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return &v
}

func pct(s string) *int {
	if v := intValue(s); v != nil && *v >= 0 && *v <= 100 {
		return v
	}
	return nil
}

// ParseTownForecast reads F-D0047-0xx / -093 JSON. Each period is keyed by the start time of the
// 天氣現象 element; point values (DataTime) are taken at that start time.
func ParseTownForecast(body []byte) ([]ForecastTown, []TownPeriod, []string, error) {
	var file struct {
		Success string `json:"success"`
		Records struct {
			Locations []struct {
				County   string `json:"LocationsName"`
				Location []struct {
					Name     string        `json:"LocationName"`
					Geocode  string        `json:"Geocode"`
					Lat      string        `json:"Latitude"`
					Lon      string        `json:"Longitude"`
					Elements []townElement `json:"WeatherElement"`
				} `json:"Location"`
			} `json:"Locations"`
		} `json:"records"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &file); err != nil {
		return nil, nil, nil, fmt.Errorf("decode township forecast: %w", err)
	}
	if file.Success != "true" {
		return nil, nil, nil, errors.New("氣象署回應 success 不是 true（授權碼可能錯誤）")
	}
	var towns []ForecastTown
	var periods []TownPeriod
	var warnings []string
	for _, group := range file.Records.Locations {
		for _, loc := range group.Location {
			lat, err1 := strconv.ParseFloat(strings.TrimSpace(loc.Lat), 64)
			lon, err2 := strconv.ParseFloat(strings.TrimSpace(loc.Lon), 64)
			geocode := strings.TrimSpace(loc.Geocode)
			if geocode == "" || err1 != nil || err2 != nil || lat < 21 || lat > 27 || lon < 117 || lon > 123 {
				warnings = append(warnings, fmt.Sprintf("%s%s 的代碼或座標不正確", group.County, loc.Name))
				continue
			}
			towns = append(towns, ForecastTown{Geocode: geocode, County: strings.TrimSpace(group.County),
				Name: strings.TrimSpace(loc.Name), Lat: lat, Lon: lon})

			// 時段以天氣現象為準
			byStart := map[time.Time]*TownPeriod{}
			var order []time.Time
			points := map[string]map[time.Time]townValue{}
			for _, e := range loc.Elements {
				for _, t := range e.Time {
					if len(t.Value) == 0 {
						continue
					}
					if t.DataTime != "" {
						at, err := time.Parse(time.RFC3339, t.DataTime)
						if err != nil {
							continue
						}
						if points[e.Name] == nil {
							points[e.Name] = map[time.Time]townValue{}
						}
						points[e.Name][at] = t.Value[0]
						continue
					}
					start, err1 := time.Parse(time.RFC3339, t.StartTime)
					end, err2 := time.Parse(time.RFC3339, t.EndTime)
					if err1 != nil || err2 != nil || !end.After(start) {
						continue
					}
					if e.Name == "天氣現象" {
						if _, ok := byStart[start]; !ok {
							byStart[start] = &TownPeriod{Geocode: geocode, StartsAt: start, EndsAt: end}
							order = append(order, start)
						}
					}
					if points[e.Name] == nil {
						points[e.Name] = map[time.Time]townValue{}
					}
					points[e.Name][start] = t.Value[0]
				}
			}
			for _, start := range order {
				p := byStart[start]
				get := func(name string) townValue { return points[name][start] }
				w := get("天氣現象")
				p.Weather = strings.TrimSpace(w["Weather"])
				p.WeatherCode = intValue(w["WeatherCode"])
				if v := get("3小時降雨機率"); v != nil {
					p.RainChance = pct(v["ProbabilityOfPrecipitation"])
				}
				if v := get("12小時降雨機率"); v != nil {
					p.RainChance = pct(v["ProbabilityOfPrecipitation"])
				}
				p.Temp = intValue(get("溫度")["Temperature"])
				p.Apparent = intValue(get("體感溫度")["ApparentTemperature"])
				p.MaxTemp = intValue(get("最高溫度")["MaxTemperature"])
				p.MinTemp = intValue(get("最低溫度")["MinTemperature"])
				p.MaxApparent = intValue(get("最高體感溫度")["MaxApparentTemperature"])
				p.MinApparent = intValue(get("最低體感溫度")["MinApparentTemperature"])
				p.Humidity = pct(get("相對濕度")["RelativeHumidity"])
				if p.Humidity == nil {
					p.Humidity = pct(get("平均相對濕度")["RelativeHumidity"])
				}
				if uv := get("紫外線指數"); uv != nil {
					p.UVI = intValue(uv["UVIndex"])
					p.UVLevel = strings.TrimSpace(uv["UVExposureLevel"])
				}
				ws := get("風速")
				p.WindMS = intValue(ws["WindSpeed"])
				p.Beaufort = intValue(ws["BeaufortScale"])
				p.WindDir = strings.TrimSpace(get("風向")["WindDirection"])
				p.Comfort = strings.TrimSpace(get("舒適度指數")["ComfortIndexDescription"])
				p.Description = strings.TrimSpace(get("天氣預報綜合描述")["WeatherDescription"])
				if p.Weather != "" {
					periods = append(periods, *p)
				}
			}
		}
	}
	return towns, periods, warnings, nil
}

// minTowns guards against a partial response; Taiwan has 368 townships.
var minTowns = 300

type TownForecastPipeline struct {
	Week   bool
	URL    string
	APIKey string
}

func (p *TownForecastPipeline) Dataset() pipeline.Dataset {
	if p.Week {
		return pipeline.Dataset{
			ID:           "town_forecast_week",
			Name:         "鄉鎮一週天氣預報",
			SourceAgency: "交通部中央氣象署",
			SourceURL:    "https://opendata.cwa.gov.tw/dataset/forecast/F-D0047-091",
			UpdateFreq:   6 * time.Hour,
			StaleAfter:   18 * time.Hour,
		}
	}
	return pipeline.Dataset{
		ID:           "town_forecast",
		Name:         "鄉鎮 3 天天氣預報",
		SourceAgency: "交通部中央氣象署",
		SourceURL:    "https://opendata.cwa.gov.tw/dataset/forecast/F-D0047-089",
		UpdateFreq:   3 * time.Hour,
		StaleAfter:   9 * time.Hour,
	}
}

func (p *TownForecastPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	var towns []ForecastTown
	var periods []TownPeriod
	for _, ids := range TownForecastIDs(p.Week) {
		body, err := cwa.GetWith(ctx, env, p.URL, p.APIKey, url.Values{"locationId": {ids}})
		if err != nil {
			return res, err
		}
		t, f, warnings, err := ParseTownForecast(body)
		if err != nil {
			return res, err
		}
		towns, periods = append(towns, t...), append(periods, f...)
		res.Warnings = append(res.Warnings, warnings...)
	}
	// 同一個時段只寫一次，否則 ON CONFLICT 會出錯
	type key struct {
		geocode string
		start   int64
	}
	seenPeriod := map[key]bool{}
	periods = slices.DeleteFunc(periods, func(f TownPeriod) bool {
		k := key{f.Geocode, f.StartsAt.Unix()}
		dup := seenPeriod[k]
		seenPeriod[k] = true
		return dup
	})
	res.RowsFetched = len(periods)
	if len(towns) < minTowns || len(periods) == 0 {
		return res, fmt.Errorf("鄉鎮預報只有 %d 個鄉鎮、%d 個時段", len(towns), len(periods))
	}

	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	known, err := upsertTowns(ctx, env, tx, towns, &res)
	if err != nil {
		return res, err
	}

	var rows [][]any
	for _, f := range periods {
		if !known[f.Geocode] {
			continue
		}
		if p.Week {
			rows = append(rows, []any{f.Geocode, f.StartsAt, f.EndsAt, pipeline.NullIfEmpty(f.Weather), f.WeatherCode,
				f.RainChance, f.MaxTemp, f.MinTemp, f.MaxApparent, f.MinApparent, f.Humidity, f.UVI,
				pipeline.NullIfEmpty(f.UVLevel), f.WindMS, f.Beaufort, pipeline.NullIfEmpty(f.WindDir),
				pipeline.NullIfEmpty(f.Description)})
		} else {
			rows = append(rows, []any{f.Geocode, f.StartsAt, f.EndsAt, pipeline.NullIfEmpty(f.Weather), f.WeatherCode,
				f.RainChance, f.Temp, f.Apparent, f.Humidity, f.WindMS, f.Beaufort, pipeline.NullIfEmpty(f.WindDir),
				pipeline.NullIfEmpty(f.Comfort)})
		}
	}
	table, cols := "data.town_forecast", []string{"geocode", "starts_at", "ends_at", "weather", "weather_code",
		"rain_chance_pct", "temp_c", "apparent_c", "humidity_pct", "wind_ms", "beaufort", "wind_dir", "comfort"}
	if p.Week {
		table, cols = "data.town_forecast_week", []string{"geocode", "starts_at", "ends_at", "weather", "weather_code",
			"rain_chance_pct", "max_temp_c", "min_temp_c", "max_apparent_c", "min_apparent_c", "humidity_pct", "uvi",
			"uv_level", "wind_ms", "beaufort", "wind_dir", "description"}
	}
	n, err := pipeline.InsertRows(ctx, tx, "INSERT INTO "+table+" ("+strings.Join(cols, ", ")+") VALUES", rows,
		upsertTail(table, cols[2:]))
	if err != nil {
		return res, fmt.Errorf("upsert %s: %w", table, err)
	}
	res.RowsInserted = n

	// 預報只留到結束後兩天
	if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE ends_at < $1`, env.Now().Add(-48*time.Hour)); err != nil {
		return res, err
	}
	t := periods[0].StartsAt
	if now := env.Now(); t.After(now) {
		t = now
	}
	res.ObserveTime(t)
	return res, tx.Commit()
}

// upsertTail updates the given columns only when one of them changed.
func upsertTail(table string, cols []string) string {
	old := make([]string, len(cols))
	ex := make([]string, len(cols))
	for i, c := range cols {
		old[i] = table + "." + c
		ex[i] = "EXCLUDED." + c
	}
	return "ON CONFLICT (geocode, starts_at) DO UPDATE SET " + pipeline.SetExcluded(cols) + ", ingested_at = now()" +
		" WHERE (" + strings.Join(old, ", ") + ") IS DISTINCT FROM (" + strings.Join(ex, ", ") + ")"
}

// upsertTowns saves the township list and returns the geocodes that were saved.
func upsertTowns(ctx context.Context, env pipeline.Env, tx *sql.Tx, towns []ForecastTown, res *pipeline.Result) (map[string]bool, error) {
	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	unknown := map[string]bool{}
	var rows [][]any
	for _, t := range towns {
		if known[t.Geocode] {
			continue
		}
		code, ok := counties.Code(t.County)
		if !ok {
			if !unknown[t.County] {
				unknown[t.County] = true
				res.Warnf("縣市「%s」對不到縣市代碼", t.County)
			}
			continue
		}
		known[t.Geocode] = true
		rows = append(rows, []any{t.Geocode, code, t.Name, fmt.Sprint(t.Lon), fmt.Sprint(t.Lat)})
	}
	if _, err := pipeline.InsertRows(ctx, tx, `
		INSERT INTO ref.forecast_town (geocode, county_code, name, geom)
		SELECT v.geocode, v.county_code, v.name, ST_SetSRID(ST_MakePoint(v.lon::float8, v.lat::float8), 4326)
		  FROM (VALUES`, rows, `) AS v(geocode, county_code, name, lon, lat)
		ON CONFLICT (geocode) DO UPDATE SET county_code = EXCLUDED.county_code, name = EXCLUDED.name,
			geom = EXCLUDED.geom, updated_at = now()
		WHERE (ref.forecast_town.county_code, ref.forecast_town.name, ref.forecast_town.geom)
		      IS DISTINCT FROM (EXCLUDED.county_code, EXCLUDED.name, EXCLUDED.geom)`); err != nil {
		return nil, fmt.Errorf("upsert townships: %w", err)
	}
	return known, nil
}
