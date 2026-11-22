package power

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

const DefaultRenewableURL = "https://www.moeaea.gov.tw/ECW/populace/opendata/wHandOpenData_File.ashx?set_id=48"

var renewableColumns = map[string]string{
	"慣常水力(數值)":   "hydro",
	"地熱(數值)":     "geothermal",
	"太陽光電(數值)":   "solar",
	"風力_陸域(數值)":  "wind_onshore",
	"風力_離岸(數值)":  "wind_offshore",
	"生質能_固態(數值)": "biomass_solid",
	"生質能_氣態(數值)": "biomass_gas",
	"廢棄物(數值)":    "waste",
}

type RenewableRow struct {
	Month  time.Time
	Source string
	MWh    float64
}

func ParseRenewable(body []byte) ([]RenewableRow, error) {
	r := csv.NewReader(bytes.NewReader(pipeline.StripBOM(body)))
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read renewable header: %w", err)
	}
	cols := map[int]string{}
	monthCol, unitCol := -1, -1
	for i, h := range head {
		h = strings.TrimSpace(h)
		if src, ok := renewableColumns[h]; ok {
			cols[i] = src
		}
		switch h {
		case "日期(年/月)":
			monthCol = i
		case "單位":
			unitCol = i
		}
	}
	if monthCol < 0 || len(cols) != len(renewableColumns) {
		return nil, fmt.Errorf("再生能源發電量的欄位不符：%v", head)
	}
	var out []RenewableRow
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read renewable: %w", err)
		}
		if monthCol >= len(rec) {
			continue
		}
		month, err := time.ParseInLocation("200601", strings.TrimSpace(rec[monthCol]), pipeline.Taipei)
		if err != nil {
			continue
		}
		if unitCol >= 0 && unitCol < len(rec) && strings.TrimSpace(rec[unitCol]) != "千度" {
			return nil, fmt.Errorf("再生能源發電量的單位不是千度：%q", rec[unitCol])
		}
		for i, src := range cols {
			if i >= len(rec) {
				continue
			}
			v, err := pipeline.ParseNumber(rec[i])
			if err != nil || v == nil || *v < 0 {
				continue
			}
			out = append(out, RenewableRow{Month: month, Source: src, MWh: *v})
		}
	}
	return out, nil
}

type RenewablePipeline struct {
	URL string
}

func (p *RenewablePipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "renewable_month",
		Name:         "再生能源每月發電量",
		SourceAgency: "經濟部能源署",
		SourceURL:    DefaultRenewableURL,
		UpdateFreq:   30 * 24 * time.Hour,
		StaleAfter:   100 * 24 * time.Hour,
	}
}

func (p *RenewablePipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	rows, err := ParseRenewable(body)
	if err != nil {
		return res, err
	}
	res.RowsFetched = len(rows)
	if len(rows) == 0 {
		return res, errors.New("再生能源發電量是空的")
	}
	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, row := range rows {
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.renewable_month (month, source, gen_mwh) VALUES ($1, $2, $3)
			ON CONFLICT (month, source) DO UPDATE SET gen_mwh = EXCLUDED.gen_mwh, ingested_at = now()
			WHERE data.renewable_month.gen_mwh IS DISTINCT FROM EXCLUDED.gen_mwh`,
			pipeline.DateColumn(row.Month), row.Source, row.MWh)
		if err != nil {
			return res, fmt.Errorf("upsert renewable %s %s: %w", row.Month.Format("2006-01"), row.Source, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
		res.ObserveTime(row.Month)
	}
	return res, tx.Commit()
}
