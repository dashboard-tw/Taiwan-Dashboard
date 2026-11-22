package population

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultVitalURL = "https://www.ris.gov.tw/rs-opendata/api/v1/datastore/ODRP010/%d?page=%d"

type Vital struct {
	Code                                string
	Births, Deaths, Marriages, Divorces int
}

func ParseVitalPage(body []byte) ([]Vital, int, error) {
	var doc struct {
		Code      string              `json:"responseCode"`
		Message   string              `json:"responseMessage"`
		TotalPage string              `json:"totalPage"`
		Data      []map[string]string `json:"responseData"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &doc); err != nil {
		return nil, 0, fmt.Errorf("decode vital statistics: %w", err)
	}
	if !strings.HasSuffix(doc.Code, "-S") {
		return nil, 0, fmt.Errorf("戶政司回應 %s %s", doc.Code, doc.Message)
	}
	pages, _ := strconv.Atoi(doc.TotalPage)
	num := func(r map[string]string, k string) (int, error) {
		v, err := strconv.Atoi(strings.TrimSpace(r[k]))
		if err != nil || v < 0 {
			return 0, fmt.Errorf("欄位 %s 不是數字：%q", k, r[k])
		}
		return v, nil
	}
	var out []Vital
	for _, r := range doc.Data {
		code := strings.TrimSpace(r["district_code"])
		if len(code) < 5 {
			continue
		}
		v := Vital{Code: code[:5]}
		var err error
		for _, f := range []struct {
			key string
			dst *int
		}{{"birth_total", &v.Births}, {"death_total", &v.Deaths}, {"marry_pair", &v.Marriages}, {"divorce_pair", &v.Divorces}} {
			if *f.dst, err = num(r, f.key); err != nil {
				return nil, 0, err
			}
		}
		out = append(out, v)
	}
	return out, pages, nil
}

type VitalPipeline struct {
	URL string
}

func (p *VitalPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "vital_month",
		Name:         "各縣市每月出生、死亡、結婚與離婚",
		SourceAgency: "內政部戶政司",
		SourceURL:    "https://data.gov.tw/dataset/77140",
		UpdateFreq:   30 * 24 * time.Hour,
		StaleAfter:   75 * 24 * time.Hour,
	}
}

const (
	vitalMonths   = 36
	vitalBackfill = 6
)

func rocMonth(t time.Time) int { return (t.Year()-1911)*100 + int(t.Month()) }

func (p *VitalPipeline) fetchMonth(ctx context.Context, env pipeline.Env, month time.Time) (map[string]*Vital, error) {
	url := p.URL
	if url == "" {
		url = DefaultVitalURL
	}
	out := map[string]*Vital{}
	for page, total := 1, 1; page <= total && page <= maxPages; page++ {
		u := fmt.Sprintf(url, rocMonth(month), page)
		body, err := env.Fetch.Get(ctx, u, u)
		var he *pipeline.HTTPError
		if errors.As(err, &he) && he.Status == 404 && page == 1 {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		rows, pages, err := ParseVitalPage(body)
		if err != nil {
			return nil, fmt.Errorf("%d 第 %d 頁：%w", rocMonth(month), page, err)
		}
		total = pages
		for _, r := range rows {
			c := out[r.Code]
			if c == nil {
				c = &Vital{Code: r.Code}
				out[r.Code] = c
			}
			c.Births += r.Births
			c.Deaths += r.Deaths
			c.Marriages += r.Marriages
			c.Divorces += r.Divorces
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func (p *VitalPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	known, err := countyCodes(ctx, env)
	if err != nil {
		return res, err
	}
	have := map[string]bool{}
	rows, err := env.DB.QueryContext(ctx, `SELECT DISTINCT to_char(month, 'YYYY-MM') FROM data.vital_month`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			rows.Close()
			return res, err
		}
		have[m] = true
	}
	rows.Close()

	now := env.Now().In(pipeline.Taipei)
	this := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, pipeline.Taipei)
	var months []time.Time
	backfill := 0
	for i := 1; i <= vitalMonths; i++ {
		m := this.AddDate(0, -i, 0)
		if i <= 3 {
			months = append(months, m)
		} else if !have[m.Format("2006-01")] && backfill < vitalBackfill {
			months = append(months, m)
			backfill++
		}
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	stored := 0
	for _, m := range months {
		counties, err := p.fetchMonth(ctx, env, m)
		if err != nil {
			return res, err
		}
		if counties == nil {
			continue
		}
		stored++
		for code, v := range counties {
			if !known[code] {
				res.Warnf("%s 有對不到的縣市代碼 %s", m.Format("2006-01"), code)
				continue
			}
			res.RowsFetched++
			r, err := tx.ExecContext(ctx, `
				INSERT INTO data.vital_month (month, county_code, births, deaths, marriages, divorces)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (month, county_code) DO UPDATE SET
					births = EXCLUDED.births, deaths = EXCLUDED.deaths, marriages = EXCLUDED.marriages,
					divorces = EXCLUDED.divorces, ingested_at = now()
				WHERE (data.vital_month.births, data.vital_month.deaths, data.vital_month.marriages, data.vital_month.divorces)
				      IS DISTINCT FROM (EXCLUDED.births, EXCLUDED.deaths, EXCLUDED.marriages, EXCLUDED.divorces)`,
				m, code, v.Births, v.Deaths, v.Marriages, v.Divorces)
			if err != nil {
				return res, fmt.Errorf("upsert vital %s %s: %w", m.Format("2006-01"), code, err)
			}
			n, _ := r.RowsAffected()
			res.RowsInserted += int(n)
		}
		res.ObserveTime(m)
	}
	if stored == 0 && len(have) == 0 {
		return res, errors.New("找不到最近幾個月的人口動態資料")
	}
	return res, tx.Commit()
}
