package aqi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

type Observation struct {
	SiteID     int
	SiteName   string
	County     string
	Lng, Lat   *float64
	ObservedAt time.Time
	Status     string
	Pollutant  string
	AQI        *int16
	Values     map[string]*float64
}

var numericFields = []struct{ source, column string }{
	{"pm2.5", "pm25_ug_m3"},
	{"pm2.5_avg", "pm25_avg_ug_m3"},
	{"pm10", "pm10_ug_m3"},
	{"pm10_avg", "pm10_avg_ug_m3"},
	{"o3", "o3_ppb"},
	{"o3_8hr", "o3_8hr_ppb"},
	{"co", "co_ppm"},
	{"co_8hr", "co_8hr_ppm"},
	{"so2", "so2_ppb"},
	{"so2_avg", "so2_avg_ppb"},
	{"no2", "no2_ppb"},
	{"nox", "nox_ppb"},
	{"no", "no_ppb"},
	{"wind_speed", "wind_speed_m_s"},
	{"wind_direc", "wind_dir_deg"},
}

var timeLayouts = []string{
	"2006/01/02 15:04:05",
	"2006/01/02 15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04:05",
}

func Parse(body []byte) ([]Observation, []string, error) {
	records, err := decodeRecords(body)
	if err != nil {
		return nil, nil, err
	}

	var obs []Observation
	var warnings []string
	for i, rec := range records {
		o, err := parseRecord(rec)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("第 %d 筆（%s）：%v", i+1, rec["sitename"], err))
			continue
		}
		obs = append(obs, o)
	}
	return obs, warnings, nil
}

func decodeRecords(body []byte) ([]map[string]string, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode AQI response: %w", err)
	}

	var list []any
	switch v := raw.(type) {
	case []any:
		list = v
	case map[string]any:
		if r, ok := v["records"].([]any); ok {
			list = r
		} else if res, ok := v["result"].(map[string]any); ok {
			list, _ = res["records"].([]any)
		}
	}
	if list == nil {
		return nil, fmt.Errorf("AQI response has no records")
	}

	out := make([]map[string]string, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		rec := make(map[string]string, len(m))
		for k, v := range m {
			switch x := v.(type) {
			case string:
				rec[k] = x
			case json.Number:
				rec[k] = x.String()
			case nil:
				rec[k] = ""
			default:
				rec[k] = fmt.Sprint(x)
			}
		}
		out = append(out, rec)
	}
	return out, nil
}

func parseRecord(rec map[string]string) (Observation, error) {
	var o Observation

	id, err := strconv.Atoi(strings.TrimSpace(rec["siteid"]))
	if err != nil {
		return o, fmt.Errorf("測站編號 %q 不是整數", rec["siteid"])
	}
	o.SiteID = id
	o.SiteName = strings.TrimSpace(rec["sitename"])
	if o.SiteName == "" {
		return o, fmt.Errorf("缺少測站名稱")
	}
	o.County = strings.TrimSpace(rec["county"])

	o.ObservedAt, err = pipeline.ParseTaipeiTime(rec["publishtime"], timeLayouts...)
	if err != nil {
		return o, fmt.Errorf("發布時間：%w", err)
	}

	if o.Lng, err = pipeline.ParseNumber(rec["longitude"]); err != nil {
		return o, fmt.Errorf("經度：%w", err)
	}
	if o.Lat, err = pipeline.ParseNumber(rec["latitude"]); err != nil {
		return o, fmt.Errorf("緯度：%w", err)
	}

	aqi, err := pipeline.ParseNumber(rec["aqi"])
	if err != nil {
		return o, fmt.Errorf("AQI：%w", err)
	}
	if aqi != nil {
		if *aqi < 0 || *aqi > math.MaxInt16 {
			return o, fmt.Errorf("AQI 超出範圍：%v", *aqi)
		}
		v := int16(math.Round(*aqi))
		o.AQI = &v
	}

	o.Status = strings.TrimSpace(rec["status"])
	o.Pollutant = strings.TrimSpace(rec["pollutant"])

	o.Values = make(map[string]*float64, len(numericFields))
	for _, f := range numericFields {
		v, err := pipeline.ParseNumber(rec[f.source])
		if err != nil {
			return o, fmt.Errorf("%s：%w", f.source, err)
		}
		o.Values[f.column] = v
	}
	return o, nil
}
