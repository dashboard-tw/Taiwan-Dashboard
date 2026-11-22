package fire

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const base = "https://opdadm.moi.gov.tw/api/v1/no-auth/resource/api/dataset/0621C600-0BF9-44F3-8D7B-529E2BAC12CD/resource/"

var Files = map[int]string{
	2020: base + "553CA892-D539-4D5B-B28F-DC835E071740/download",
	2021: base + "C2A3DCF0-7D51-44D2-AD51-AB9A76F9C2F3/download",
	2022: base + "DABCD323-037C-479B-AE2E-4C7B87D3256C/download",
	2023: base + "4E416C67-E73F-4F11-80D7-F8A2C3093BE1/download",
	2024: base + "F79BD315-0286-4EE6-9B4C-1277B0CEF98A/download",
}

type Incident struct {
	ID, County, Town, Cause string
	Reported                time.Time
	Arrived                 *time.Time
	Deaths, Injuries        int
}

func Cause(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "其他-")
	if i := strings.IndexAny(s, "(（"); i > 0 {
		s = s[:i]
	}
	if s == "人為縱火" {
		s = "縱火"
	}
	if s == "" {
		s = "其他"
	}
	return s
}

func parseTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	var y, mo, d, h, mi int
	if _, err := fmt.Sscanf(s, "%d-%d-%d %d:%d", &y, &mo, &d, &h, &mi); err != nil || y < 90 || mo < 1 || mo > 12 {
		return nil
	}
	t := time.Date(y+1911, time.Month(mo), d, h, mi, 0, 0, pipeline.Taipei)
	return &t
}

func Parse(body []byte) ([]Incident, error) {
	body = pipeline.StripBOM(body)
	first, _, _ := bytes.Cut(body, []byte("\n"))
	r := csv.NewReader(bytes.NewReader(body))
	if bytes.Contains(first, []byte("\t")) {
		r.Comma = '\t'
	}
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read fire header: %w", err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(h)] = i
	}
	for _, need := range []string{"縣市", "火災案件編號", "報案時間", "起火原因"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("火災資料少了欄位 %s", need)
		}
	}
	var out []Incident
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read fires: %w", err)
		}
		get := func(name string) string {
			if i, ok := col[name]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		reported := parseTime(get("報案時間"))
		x := Incident{ID: get("火災案件編號"), County: get("縣市"), Town: get("鄉_鎮_市_區"), Cause: Cause(get("起火原因")),
			Arrived: parseTime(get("到達時間"))}
		if reported == nil || x.ID == "" || len(x.ID) > 30 {
			continue
		}
		x.Reported = *reported
		x.Deaths, _ = strconv.Atoi(get("死亡人數"))
		x.Injuries, _ = strconv.Atoi(get("受傷人數"))
		if x.Deaths < 0 || x.Deaths > 1000 {
			x.Deaths = 0
		}
		if x.Injuries < 0 || x.Injuries > 1000 {
			x.Injuries = 0
		}
		if x.Arrived != nil && (x.Arrived.Before(x.Reported) || x.Arrived.Sub(x.Reported) > 24*time.Hour) {
			x.Arrived = nil
		}
		out = append(out, x)
	}
	return out, nil
}

type Pipeline struct {
	Files map[int]string
}

func (p *Pipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "fire_incident",
		Name:         "火災事件",
		SourceAgency: "內政部消防署",
		SourceURL:    "https://data.gov.tw/dataset/13764",
		UpdateFreq:   365 * 24 * time.Hour,
		StaleAfter:   800 * 24 * time.Hour,
	}
}

func (p *Pipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	files := p.Files
	if files == nil {
		files = Files
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
	for year, u := range files {
		body, err := env.Fetch.Get(ctx, u, u)
		if err != nil {
			return res, fmt.Errorf("%d: %w", year, err)
		}
		list, err := Parse(body)
		if err != nil {
			return res, fmt.Errorf("%d: %w", year, err)
		}
		if len(list) == 0 {
			return res, fmt.Errorf("%d 年的火災資料是空的", year)
		}
		res.RowsFetched += len(list)
		for _, x := range list {
			var county any
			if c, ok := counties.Code(x.County); ok {
				county = c
			}
			r, err := tx.ExecContext(ctx, `
				INSERT INTO data.fire_incident (case_id, reported_at, arrived_at, county_code, town, cause, deaths, injuries)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (case_id) DO NOTHING`,
				x.ID, x.Reported, x.Arrived, county, pipeline.NullIfEmpty(x.Town), x.Cause, x.Deaths, x.Injuries)
			if err != nil {
				return res, fmt.Errorf("insert fire %s: %w", x.ID, err)
			}
			n, _ := r.RowsAffected()
			res.RowsInserted += int(n)
			res.ObserveTime(x.Reported)
		}
	}
	return res, tx.Commit()
}
