package welfare

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultURL = "https://www.opendata.mohw.gov.tw/dataset/opendata/cc/165355/%E7%A4%BE%E6%9C%83%E7%A6%8F%E5%88%A9%E6%A9%9F%E6%A7%8B%E4%BB%A3%E7%A2%BC.csv"

type Center struct {
	ID, Name, Address string
	Lon, Lat          *float64
}

func ParseChildcare(body []byte) ([]Center, error) {
	r := csv.NewReader(bytes.NewReader(pipeline.StripBOM(body)))
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read welfare header: %w", err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(h)] = i
	}
	for _, need := range []string{"社福機構代碼", "機構名稱", "次類別", "地址"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("社福機構名冊少了欄位 %s", need)
		}
	}
	seen := map[string]bool{}
	var out []Center
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read welfare: %w", err)
		}
		get := func(name string) string {
			if i, ok := col[name]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		c := Center{ID: get("社福機構代碼"), Name: get("機構名稱"), Address: get("地址")}
		if get("次類別") != "T0701" || !strings.Contains(c.Name, "托嬰") || c.ID == "" || len(c.ID) > 20 || seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		lon, err1 := pipeline.ParseNumber(get("經度"))
		lat, err2 := pipeline.ParseNumber(get("緯度"))
		if err1 == nil && err2 == nil && lon != nil && lat != nil && *lat > 21 && *lat < 27 && *lon > 118 && *lon < 123 {
			c.Lon, c.Lat = lon, lat
		}
		out = append(out, c)
	}
	return out, nil
}

type ChildcarePipeline struct {
	URL string
}

func (p *ChildcarePipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "childcare_center",
		Name:         "托嬰中心",
		SourceAgency: "衛生福利部",
		SourceURL:    "https://data.gov.tw/dataset/165355",
		UpdateFreq:   30 * 24 * time.Hour,
		StaleAfter:   75 * 24 * time.Hour,
	}
}

func (p *ChildcarePipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	list, err := ParseChildcare(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(list)
	if len(list) == 0 {
		return res, errors.New("名冊裡沒有托嬰中心")
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
	for _, c := range list {
		var county, lon, lat any
		if code, ok := counties.CodeFromAddress(c.Address); ok {
			county = code
		}
		if c.Lon != nil {
			lon, lat = *c.Lon, *c.Lat
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO ref.childcare_center (facility_id, name, county_code, address, geom)
			VALUES ($1, $2, $3, $4, CASE WHEN $5::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($5, $6), 4326) END)
			ON CONFLICT (facility_id) DO UPDATE SET
				name = EXCLUDED.name, county_code = EXCLUDED.county_code, address = EXCLUDED.address,
				geom = EXCLUDED.geom, last_seen_at = now()`,
			c.ID, c.Name, county, pipeline.NullIfEmpty(c.Address), lon, lat)
		if err != nil {
			return res, fmt.Errorf("upsert childcare centre %s: %w", c.ID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE ref.childcare_center s SET county_code = c.code
		FROM ref.county c
		WHERE s.county_code IS NULL AND s.geom IS NOT NULL AND ST_Contains(c.boundary, s.geom)`); err != nil {
		return res, err
	}
	var unknown int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM ref.childcare_center WHERE county_code IS NULL`).Scan(&unknown); err != nil {
		return res, err
	}
	if unknown > 0 {
		res.Warnf("%d 間托嬰中心對不到縣市", unknown)
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}
