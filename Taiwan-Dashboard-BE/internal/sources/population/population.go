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

const DefaultURL = "https://www.ris.gov.tw/rs-opendata/api/v1/datastore/ODRP019/%d?page=%d"

type County struct {
	Code                                 string
	Households, Population, Male, Female int
}

type Page struct {
	TotalPages int
	Rows       []County // per village; Code is the county code
}

var (
	codeKeys = []string{"district_code", "區域別代碼"}
	hhKeys   = [][]string{
		{"household_ordinary_total", "共同生活戶_戶數"},
		{"household_business_total", "共同事業戶_戶數"},
		{"household_single_total", "單獨生活戶_戶數"},
	}
	maleKeys = [][]string{
		{"household_ordinary_m", "共同生活戶_男"},
		{"household_business_m", "共同事業戶_男"},
		{"household_single_m", "單獨生活戶_男"},
	}
	femaleKeys = [][]string{
		{"household_ordinary_f", "共同生活戶_女"},
		{"household_business_f", "共同事業戶_女"},
		{"household_single_f", "單獨生活戶_女"},
	}
)

func ParsePage(body []byte) (Page, error) {
	var doc struct {
		Code      string              `json:"responseCode"`
		Message   string              `json:"responseMessage"`
		TotalPage string              `json:"totalPage"`
		Data      []map[string]string `json:"responseData"`
	}
	if err := json.Unmarshal(pipeline.StripBOM(body), &doc); err != nil {
		return Page{}, fmt.Errorf("decode population: %w", err)
	}
	if !strings.HasSuffix(doc.Code, "-S") {
		return Page{}, fmt.Errorf("戶政司回應 %s %s", doc.Code, doc.Message)
	}
	p := Page{}
	p.TotalPages, _ = strconv.Atoi(doc.TotalPage)
	first := func(r map[string]string, keys []string) string {
		for _, k := range keys {
			if v, ok := r[k]; ok {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}
	sum := func(r map[string]string, groups [][]string) (int, error) {
		n := 0
		for _, keys := range groups {
			s := first(r, keys)
			if s == "" {
				return 0, fmt.Errorf("少了欄位 %s", keys[0])
			}
			v, err := strconv.Atoi(s)
			if err != nil || v < 0 {
				return 0, fmt.Errorf("欄位 %s 不是數字：%q", keys[0], s)
			}
			n += v
		}
		return n, nil
	}
	for _, r := range doc.Data {
		code := first(r, codeKeys)
		if len(code) < 5 {
			continue
		}
		c := County{Code: code[:5]}
		var err error
		if c.Households, err = sum(r, hhKeys); err != nil {
			return Page{}, err
		}
		if c.Male, err = sum(r, maleKeys); err != nil {
			return Page{}, err
		}
		if c.Female, err = sum(r, femaleKeys); err != nil {
			return Page{}, err
		}
		c.Population = c.Male + c.Female
		p.Rows = append(p.Rows, c)
	}
	return p, nil
}

type Pipeline struct {
	URL string
}

func (p *Pipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "population_year",
		Name:         "各縣市年底戶數與人口數",
		SourceAgency: "內政部戶政司",
		SourceURL:    "https://data.gov.tw/dataset/14299",
		UpdateFreq:   365 * 24 * time.Hour,
		StaleAfter:   500 * 24 * time.Hour,
	}
}

const maxPages = 20

func (p *Pipeline) fetchYear(ctx context.Context, env pipeline.Env, roc int) (map[string]*County, int, error) {
	url := p.URL
	if url == "" {
		url = DefaultURL
	}
	out := map[string]*County{}
	villages := 0
	for page, total := 1, 1; page <= total && page <= maxPages; page++ {
		u := fmt.Sprintf(url, roc, page)
		body, err := env.Fetch.Get(ctx, u, u)
		var he *pipeline.HTTPError
		if errors.As(err, &he) && he.Status == 404 && page == 1 {
			return nil, 0, nil
		}
		if err != nil {
			return nil, 0, err
		}
		pg, err := ParsePage(body)
		if err != nil {
			return nil, 0, fmt.Errorf("%d 年第 %d 頁：%w", roc, page, err)
		}
		total = pg.TotalPages
		for _, r := range pg.Rows {
			c := out[r.Code]
			if c == nil {
				c = &County{Code: r.Code}
				out[r.Code] = c
			}
			c.Households += r.Households
			c.Population += r.Population
			c.Male += r.Male
			c.Female += r.Female
			villages++
		}
	}
	if villages == 0 {
		return nil, 0, nil
	}
	return out, villages, nil
}

func (p *Pipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	known, err := countyCodes(ctx, env)
	if err != nil {
		return res, err
	}
	latest := env.Now().In(pipeline.Taipei).Year() - 1
	stored := 0
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for year := latest; year >= latest-3 && stored < 2; year-- {
		counties, villages, err := p.fetchYear(ctx, env, year-1911)
		if err != nil {
			return res, err
		}
		if counties == nil {
			continue
		}
		res.RowsFetched += villages
		for code, c := range counties {
			if !known[code] {
				res.Warnf("%d 年有對不到的縣市代碼 %s", year, code)
				continue
			}
			r, err := tx.ExecContext(ctx, `
				INSERT INTO data.population_year (year, county_code, households, population, male, female)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (year, county_code) DO UPDATE SET
					households = EXCLUDED.households, population = EXCLUDED.population, male = EXCLUDED.male,
					female = EXCLUDED.female, ingested_at = now()`,
				year, code, c.Households, c.Population, c.Male, c.Female)
			if err != nil {
				return res, fmt.Errorf("upsert population %d %s: %w", year, code, err)
			}
			n, _ := r.RowsAffected()
			res.RowsInserted += int(n)
		}
		if stored == 0 {
			res.ObserveTime(time.Date(year, 12, 31, 0, 0, 0, 0, pipeline.Taipei))
		}
		stored++
	}
	if stored == 0 {
		return res, errors.New("找不到最近幾年的人口資料")
	}
	return res, tx.Commit()
}

func countyCodes(ctx context.Context, env pipeline.Env) (map[string]bool, error) {
	rows, err := env.DB.QueryContext(ctx, `SELECT code FROM ref.county`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out[c] = true
	}
	return out, rows.Err()
}
