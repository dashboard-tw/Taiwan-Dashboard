package aqi

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultForecastURL = "https://data.moenv.gov.tw/api/v2/aqf_p_01"

type Forecast struct {
	Area           string
	ForecastDate   time.Time
	PublishedAt    time.Time
	AQI            *int16
	MajorPollutant string
	MinorPollutant string
	MinorAQI       *int16
	Content        string
}

type ForecastPipeline struct {
	URL    string
	APIKey string
}

func (p *ForecastPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "aqi_forecast",
		Name:         "空氣品質預報（每天三次）",
		SourceAgency: "環境部",
		SourceURL:    "https://data.moenv.gov.tw/dataset/detail/AQF_P_01",
		UpdateFreq:   8 * time.Hour,
		StaleAfter:   15 * time.Hour,
	}
}

func ParseForecast(body []byte) ([]Forecast, []string, error) {
	records, err := decodeRecords(body)
	if err != nil {
		return nil, nil, err
	}
	var out []Forecast
	var warnings []string
	for i, rec := range records {
		f, err := parseForecastRecord(rec)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("第 %d 筆（%s）：%v", i+1, rec["area"], err))
			continue
		}
		out = append(out, f)
	}
	return out, warnings, nil
}

func parseForecastRecord(rec map[string]string) (Forecast, error) {
	f := Forecast{
		Area:           strings.TrimSpace(rec["area"]),
		MajorPollutant: strings.TrimSpace(rec["majorpollutant"]),
		MinorPollutant: strings.TrimSpace(rec["minorpollutant"]),
		Content:        strings.TrimSpace(rec["content"]),
	}
	if f.Area == "" {
		return f, fmt.Errorf("缺少空品區")
	}
	var err error
	if f.PublishedAt, err = pipeline.ParseTaipeiTime(rec["publishtime"], timeLayouts...); err != nil {
		return f, fmt.Errorf("發布時間：%w", err)
	}
	if f.ForecastDate, err = pipeline.ParseTaipeiTime(rec["forecastdate"], "2006-01-02", "2006/01/02"); err != nil {
		return f, fmt.Errorf("預報日期：%w", err)
	}
	if f.AQI, err = parseAQI(rec["aqi"]); err != nil {
		return f, fmt.Errorf("AQI：%w", err)
	}
	if f.MinorAQI, err = parseAQI(rec["minorpollutantaqi"]); err != nil {
		return f, fmt.Errorf("次要污染物 AQI：%w", err)
	}
	return f, nil
}

func parseAQI(s string) (*int16, error) {
	v, err := pipeline.ParseNumber(s)
	if err != nil || v == nil {
		return nil, err
	}
	if *v < 0 || *v > 500 {
		return nil, fmt.Errorf("超出範圍：%v", *v)
	}
	n := int16(math.Round(*v))
	return &n, nil
}

func (p *ForecastPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	if p.APIKey == "" {
		return res, fmt.Errorf("%w：未設定 MOENV_API_KEY", pipeline.ErrNotConfigured)
	}
	q := url.Values{"format": {"JSON"}, "limit": {"1000"}, "sort": {"publishtime desc"}, "api_key": {p.APIKey}}
	body, err := env.Fetch.Get(ctx, p.URL+"?"+q.Encode(), p.URL+"?format=JSON&limit=1000&api_key=***")
	if err != nil {
		return res, err
	}
	list, warnings, err := ParseForecast(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, fmt.Errorf("空品預報回應沒有任何資料")
	}

	areas := map[string]bool{}
	rows, err := env.DB.QueryContext(ctx, `SELECT area FROM ref.aqi_area`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			rows.Close()
			return res, err
		}
		areas[a] = true
	}
	if err := rows.Err(); err != nil {
		return res, err
	}
	rows.Close()

	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	unknown := map[string]bool{}
	for _, f := range list {
		if !areas[f.Area] {
			if !unknown[f.Area] {
				unknown[f.Area] = true
				res.Warnf("不認得的空品區「%s」，請更新 ref.aqi_area", f.Area)
			}
			continue
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.aqi_forecast (area, forecast_date, published_at, aqi, major_pollutant, minor_pollutant, minor_aqi)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (area, forecast_date, published_at) DO NOTHING`,
			f.Area, f.ForecastDate.Format(time.DateOnly), f.PublishedAt, f.AQI,
			pipeline.NullIfEmpty(f.MajorPollutant), pipeline.NullIfEmpty(f.MinorPollutant), f.MinorAQI)
		if err != nil {
			return res, fmt.Errorf("insert forecast %s %s: %w", f.Area, f.ForecastDate.Format(time.DateOnly), err)
		}
		if f.Content != "" {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO data.aqi_forecast_text (published_at, content) VALUES ($1, $2)
				ON CONFLICT (published_at) DO NOTHING`, f.PublishedAt, f.Content); err != nil {
				return res, fmt.Errorf("insert forecast text: %w", err)
			}
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
		res.ObserveTime(f.PublishedAt)
	}
	return res, tx.Commit()
}
