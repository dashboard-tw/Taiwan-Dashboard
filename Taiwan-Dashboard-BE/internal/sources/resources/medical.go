package resources

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

func readCSV(body []byte) ([]map[string]string, error) {
	r := csv.NewReader(bytes.NewReader(pipeline.StripBOM(body)))
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}
	for i := range head {
		head[i] = strings.TrimSpace(head[i])
	}
	var out []map[string]string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read CSV: %w", err)
		}
		m := make(map[string]string, len(head))
		for i, h := range head {
			if i < len(rec) {
				m[h] = strings.TrimSpace(rec[i])
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func parseYMD(s string) *time.Time {
	s = strings.TrimSpace(s)
	if len(s) != 8 {
		return nil
	}
	t, err := time.ParseInLocation("20060102", s, pipeline.Taipei)
	if err != nil {
		return nil
	}
	return &t
}

var MedicalFiles = []struct {
	Level string
	URL   string
}{
	{"medical_center", "https://info.nhi.gov.tw/api/iode0000s01/Dataset?rId=A21030000I-D21001-003"},
	{"regional", "https://info.nhi.gov.tw/api/iode0000s01/Dataset?rId=A21030000I-D21002-005"},
	{"district", "https://info.nhi.gov.tw/api/iode0000s01/Dataset?rId=A21030000I-D21003-003"},
	{"clinic", "https://info.nhi.gov.tw/api/iode0000s01/Dataset?rId=A21030000I-D21004-009"},
	{"pharmacy", "https://info.nhi.gov.tw/api/iode0000s01/Dataset?rId=A21030000I-D21005-001"},
}

type MedicalFacility struct {
	ID, Name, Level, Kind, County, Address, Phone, Departments, Services string
	ContractEnd                                                          *time.Time
}

func ParseMedical(body []byte, level string) ([]MedicalFacility, error) {
	rows, err := readCSV(body)
	if err != nil {
		return nil, err
	}
	var out []MedicalFacility
	for _, r := range rows {
		f := MedicalFacility{ID: r["醫事機構代碼"], Name: r["醫事機構名稱"], Level: level, Kind: r["醫事機構種類"],
			County: r["縣市別代碼"], Address: r["地址"], Phone: r["電話"], Departments: r["診療科別"],
			Services: r["服務項目"], ContractEnd: parseYMD(r["終止合約或歇業日期"])}
		if f.ID == "" || f.Name == "" || len(f.ID) > 20 {
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

type MedicalPipeline struct {
	URLs map[string]string
}

func (p *MedicalPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "medical_facility",
		Name:         "健保特約醫事機構（醫院、診所、藥局）",
		SourceAgency: "衛生福利部中央健康保險署",
		SourceURL:    "https://data.gov.tw/dataset/39283",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   72 * time.Hour,
	}
}

func (p *MedicalPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	var all []MedicalFacility
	for _, f := range MedicalFiles {
		url := f.URL
		if u := p.URLs[f.Level]; u != "" {
			url = u
		}
		body, err := env.Fetch.Get(ctx, url, url)
		if err != nil {
			return res, err
		}
		list, err := ParseMedical(body, f.Level)
		if err != nil {
			return res, fmt.Errorf("%s: %w", f.Level, err)
		}
		if len(list) == 0 {
			return res, fmt.Errorf("%s 的名單是空的", f.Level)
		}
		all = append(all, list...)
	}
	res.RowsFetched = len(all)
	known, err := countyCodes(ctx, env)
	if err != nil {
		return res, err
	}
	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}
	unknown := 0
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, f := range all {
		var county any
		if known[f.County] {
			county = f.County
		} else if c, ok := counties.CodeFromAddress(f.Address); ok {
			county = c
		} else {
			unknown++
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO ref.medical_facility (facility_id, name, level, kind, county_code, address, phone, departments,
			                                  services, contract_end)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (facility_id) DO UPDATE SET
				name = EXCLUDED.name, level = EXCLUDED.level, kind = EXCLUDED.kind, county_code = EXCLUDED.county_code,
				address = EXCLUDED.address, phone = EXCLUDED.phone, departments = EXCLUDED.departments,
				services = EXCLUDED.services, contract_end = EXCLUDED.contract_end, last_seen_at = now()`,
			f.ID, f.Name, f.Level, pipeline.NullIfEmpty(f.Kind), county, pipeline.NullIfEmpty(f.Address),
			pipeline.NullIfEmpty(f.Phone), pipeline.NullIfEmpty(f.Departments), pipeline.NullIfEmpty(f.Services), f.ContractEnd)
		if err != nil {
			return res, fmt.Errorf("upsert medical facility %s: %w", f.ID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	if unknown > 0 {
		res.Warnf("%d 個醫事機構對不到縣市", unknown)
	}
	res.ObserveTime(env.Now())
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
