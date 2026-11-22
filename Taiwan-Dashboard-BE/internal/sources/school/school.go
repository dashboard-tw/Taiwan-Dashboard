package school

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

type File struct {
	Level   string
	URL     string
	PerYear bool
}

var Files = []File{
	{"elementary", "https://stats.moe.gov.tw/files/school/%d/e1_new.csv", true},
	{"junior", "https://stats.moe.gov.tw/files/opendata/j1_new.csv", false},
	{"senior", "https://stats.moe.gov.tw/files/school/%d/high.csv", true},
	{"university", "https://stats.moe.gov.tw/files/opendata/u1_new.csv", false},
}

type School struct {
	Level, Code, Name, County, Address string
	Public                             bool
	Year                               int
}

var bracketRE = regexp.MustCompile(`^\[[0-9A-Za-z]*\]`)

func Parse(body []byte, level string) ([]School, error) {
	r := csv.NewReader(bytes.NewReader(pipeline.StripBOM(body)))
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read school header: %w", err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(h)] = i
	}
	for _, need := range []string{"學年度", "代碼", "學校名稱", "公/私立", "縣市名稱"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("學校名錄少了欄位 %s", need)
		}
	}
	var all []School
	newest := 0
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read school: %w", err)
		}
		get := func(name string) string {
			if i, ok := col[name]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		year, err := strconv.Atoi(get("學年度"))
		if err != nil {
			continue
		}
		s := School{Level: level, Code: get("代碼"), Name: get("學校名稱"), Public: get("公/私立") == "公立", Year: year,
			County:  bracketRE.ReplaceAllString(get("縣市名稱"), ""),
			Address: bracketRE.ReplaceAllString(get("地址"), "")}
		if s.Code == "" || s.Name == "" || len(s.Code) > 20 {
			continue
		}
		all = append(all, s)
		if year > newest {
			newest = year
		}
	}
	var out []School
	for _, s := range all {
		if s.Year == newest {
			out = append(out, s)
		}
	}
	return out, nil
}

type Pipeline struct {
	URLs map[string]string
}

func (p *Pipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "school",
		Name:         "各級學校名錄",
		SourceAgency: "教育部統計處",
		SourceURL:    "https://data.gov.tw/dataset/6087",
		UpdateFreq:   365 * 24 * time.Hour,
		StaleAfter:   500 * 24 * time.Hour,
	}
}

func schoolYear(t time.Time) int {
	t = t.In(pipeline.Taipei)
	y := t.Year() - 1911
	if t.Month() < time.August {
		y--
	}
	return y
}

func (p *Pipeline) fetch(ctx context.Context, env pipeline.Env, f File) ([]byte, error) {
	if u := p.URLs[f.Level]; u != "" {
		f.URL = u
	}
	if !f.PerYear {
		return env.Fetch.Get(ctx, f.URL, f.URL)
	}
	year := schoolYear(env.Now())
	var err error
	for y := year; y >= year-1; y-- {
		u := fmt.Sprintf(f.URL, y)
		var body []byte
		body, err = env.Fetch.Get(ctx, u, u)
		var he *pipeline.HTTPError
		if errors.As(err, &he) && he.Status == 404 {
			continue
		}
		return body, err
	}
	return nil, err
}

func (p *Pipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	var all []School
	for _, f := range Files {
		body, err := p.fetch(ctx, env, f)
		if err != nil {
			return res, fmt.Errorf("%s: %w", f.Level, err)
		}
		list, err := Parse(body, f.Level)
		if err != nil {
			return res, fmt.Errorf("%s: %w", f.Level, err)
		}
		if len(list) == 0 {
			return res, fmt.Errorf("%s 的名錄是空的", f.Level)
		}
		all = append(all, list...)
	}
	res.RowsFetched = len(all)
	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	unknown := 0
	for _, s := range all {
		var county any
		if c, ok := counties.Code(s.County); ok {
			county = c
		} else if c, ok := counties.CodeFromAddress(s.Address); ok {
			county = c
		} else {
			unknown++
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO ref.school (school_id, level, code, name, is_public, county_code, address, school_year)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (school_id) DO UPDATE SET
				name = EXCLUDED.name, is_public = EXCLUDED.is_public, county_code = EXCLUDED.county_code,
				address = EXCLUDED.address, school_year = EXCLUDED.school_year, last_seen_at = now()`,
			s.Level+":"+s.Code, s.Level, s.Code, s.Name, s.Public, county, pipeline.NullIfEmpty(s.Address), s.Year)
		if err != nil {
			return res, fmt.Errorf("upsert school %s: %w", s.Code, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	if unknown > 0 {
		res.Warnf("%d 所學校對不到縣市", unknown)
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}
