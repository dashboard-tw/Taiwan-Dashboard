package waste

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultURL = "https://data.moenv.gov.tw/api/v2/stat_p_127"

type Row struct {
	Year, Month                    int // Month 0 is the whole year
	County                         string
	Total, Garbage, Recycled, Food float64
	PerCapita                      *float64
}

var rePeriod = regexp.MustCompile(`^(\d{2,3})年(?:\s*(\d{1,2})月)?$`)

func Parse(body []byte) ([]Row, int, error) {
	var recs []map[string]string
	if err := json.Unmarshal(pipeline.StripBOM(body), &recs); err != nil {
		if strings.Contains(string(body), "查無資料") {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("decode waste: %w", err)
	}
	var out []Row
	for _, r := range recs {
		m := rePeriod.FindStringSubmatch(strings.TrimSpace(r["item1"]))
		if m == nil {
			continue
		}
		y, _ := strconv.Atoi(m[1])
		row := Row{Year: y + 1911, County: strings.TrimSpace(r["item2"])}
		if m[2] != "" {
			row.Month, _ = strconv.Atoi(m[2])
		}
		vals := make([]float64, 4)
		ok := true
		for i := range vals {
			v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(r["value"+strconv.Itoa(i+1)]), ",", ""), 64)
			if err != nil || v < 0 {
				ok = false
				break
			}
			vals[i] = v
		}
		if !ok || row.County == "" {
			continue
		}
		row.Total, row.Garbage, row.Recycled, row.Food = vals[0], vals[1], vals[2], vals[3]
		if v, err := strconv.ParseFloat(strings.TrimSpace(r["value5"]), 64); err == nil && v >= 0 && v < 20 {
			row.PerCapita = &v
		}
		out = append(out, row)
	}
	return out, len(recs), nil
}

type Pipeline struct {
	URL    string
	APIKey string // MOENV_API_KEY
}

func (p *Pipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "waste",
		Name:         "各縣市垃圾與資源回收",
		SourceAgency: "環境部",
		SourceURL:    "https://data.gov.tw/dataset/89040",
		UpdateFreq:   30 * 24 * time.Hour,
		StaleAfter:   150 * 24 * time.Hour,
	}
}

const maxPages = 30

func (p *Pipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	if p.APIKey == "" {
		return res, fmt.Errorf("%w：未設定 MOENV_API_KEY", pipeline.ErrNotConfigured)
	}
	var all []Row
	for page := 0; page < maxPages; page++ {
		offset := strconv.Itoa(page * 1000)
		q := url.Values{"format": {"JSON"}, "limit": {"1000"}, "offset": {offset}, "api_key": {p.APIKey}}
		redacted := p.URL + "?format=JSON&limit=1000&offset=" + offset + "&api_key=***"
		body, err := env.Fetch.Get(ctx, p.URL+"?"+q.Encode(), redacted)
		if err != nil {
			return res, err
		}
		rows, n, err := Parse(body)
		if err != nil {
			return res, err
		}
		all = append(all, rows...)
		if n < 1000 {
			break
		}
	}
	res.RowsFetched = len(all)
	if len(all) == 0 {
		return res, errors.New("垃圾與資源回收是空的")
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
	for _, r := range all {
		code, ok := counties.Code(r.County)
		if !ok {
			unknown[r.County] = true
			continue
		}
		kind, month := "year", 1
		if r.Month > 0 {
			kind, month = "month", r.Month
		}
		period := time.Date(r.Year, time.Month(month), 1, 0, 0, 0, 0, pipeline.Taipei)
		res2, err := tx.ExecContext(ctx, `
			INSERT INTO data.waste_county (period_type, period, county_code, total_t, garbage_t, recycled_t, food_t, per_capita_kg)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (period_type, period, county_code) DO UPDATE SET
				total_t = EXCLUDED.total_t, garbage_t = EXCLUDED.garbage_t, recycled_t = EXCLUDED.recycled_t,
				food_t = EXCLUDED.food_t, per_capita_kg = EXCLUDED.per_capita_kg, ingested_at = now()
			WHERE data.waste_county.total_t IS DISTINCT FROM EXCLUDED.total_t
			   OR data.waste_county.per_capita_kg IS DISTINCT FROM EXCLUDED.per_capita_kg`,
			kind, period, code, r.Total, r.Garbage, r.Recycled, r.Food, r.PerCapita)
		if err != nil {
			return res, fmt.Errorf("upsert waste %s %s: %w", period.Format("2006-01"), r.County, err)
		}
		n, _ := res2.RowsAffected()
		res.RowsInserted += int(n)
		if kind == "month" {
			res.ObserveTime(period)
		}
	}
	for c := range unknown {
		if c != "全國" && c != "總計" {
			res.Warnf("對不到的縣市：%s", c)
		}
	}
	return res, tx.Commit()
}
