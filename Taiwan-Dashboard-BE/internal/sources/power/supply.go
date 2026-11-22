package power

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const (
	DefaultDailyURL    = "https://service.taipower.com.tw/data/opendata/apply/file/d006005/001.csv"
	DefaultForecastURL = "https://service.taipower.com.tw/data/opendata/apply/file/d006014/001.csv"
)

const mwPerWanKW = 10

type SupplyRow struct {
	Date           time.Time
	NetPeakCapMW   *float64
	PeakLoadMW     *float64
	ReserveMW      *float64
	ReservePct     *float64
	IndustrialGWh  *float64 // only in the daily file
	ResidentialGWh *float64 // only in the daily file
}

type column struct {
	header string
	dst    func(*SupplyRow) **float64
	scale  float64
}

var dailyColumns = []column{
	{"淨尖峰供電能力(萬瓩)", func(r *SupplyRow) **float64 { return &r.NetPeakCapMW }, mwPerWanKW},
	{"尖峰負載(萬瓩)", func(r *SupplyRow) **float64 { return &r.PeakLoadMW }, mwPerWanKW},
	{"備轉容量(萬瓩)", func(r *SupplyRow) **float64 { return &r.ReserveMW }, mwPerWanKW},
	{"備轉容量率(%)", func(r *SupplyRow) **float64 { return &r.ReservePct }, 1},
	{"工業用電(百萬度)", func(r *SupplyRow) **float64 { return &r.IndustrialGWh }, 1}, // 百萬度 = GWh
	{"民生用電(百萬度)", func(r *SupplyRow) **float64 { return &r.ResidentialGWh }, 1},
}

var forecastColumns = []column{
	{"預估淨尖峰供電能力(萬瓩)", func(r *SupplyRow) **float64 { return &r.NetPeakCapMW }, mwPerWanKW},
	{"預估瞬時尖峰負載(萬瓩)", func(r *SupplyRow) **float64 { return &r.PeakLoadMW }, mwPerWanKW},
	{"預估尖峰備轉容量(萬瓩)", func(r *SupplyRow) **float64 { return &r.ReserveMW }, mwPerWanKW},
	{"預估尖峰備轉容量率(%)", func(r *SupplyRow) **float64 { return &r.ReservePct }, 1},
}

func ParseDailyCSV(body []byte) ([]SupplyRow, []string, error) {
	return parseSupplyCSV(body, dailyColumns)
}

func ParseForecastCSV(body []byte) ([]SupplyRow, []string, error) {
	return parseSupplyCSV(body, forecastColumns)
}

func parseSupplyCSV(body []byte, cols []column) ([]SupplyRow, []string, error) {
	r := csv.NewReader(bytes.NewReader(pipeline.StripBOM(body)))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("read CSV header: %w", err)
	}
	index := map[string]int{}
	for i, h := range header {
		index[strings.TrimSpace(h)] = i
	}
	dateIdx, ok := index["日期"]
	if !ok {
		return nil, nil, errors.New("CSV 缺少「日期」欄位")
	}
	for _, c := range cols {
		if _, ok := index[c.header]; !ok {
			return nil, nil, fmt.Errorf("CSV 缺少「%s」欄位", c.header)
		}
	}

	var rows []SupplyRow
	var warnings []string
	for line := 2; ; line++ {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read CSV line %d: %w", line, err)
		}
		if dateIdx >= len(rec) || strings.TrimSpace(rec[dateIdx]) == "" {
			continue
		}
		date, err := time.ParseInLocation("20060102", strings.TrimSpace(rec[dateIdx]), pipeline.Taipei)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("第 %d 行日期 %q 無法解析", line, rec[dateIdx]))
			continue
		}
		row := SupplyRow{Date: date}
		for _, c := range cols {
			i := index[c.header]
			if i >= len(rec) {
				continue
			}
			v, err := pipeline.ParseNumber(rec[i])
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("第 %d 行「%s」：%v", line, c.header, err))
				continue
			}
			if v != nil {
				scaled := round(*v*c.scale, 3)
				v = &scaled
			}
			*c.dst(&row) = v
		}
		rows = append(rows, row)
	}
	return rows, warnings, nil
}

type DailyPipeline struct {
	URL string
}

func (p *DailyPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "power_supply_daily",
		Name:         "過去電力供需（每日）",
		SourceAgency: "台灣電力公司",
		SourceURL:    "https://data.gov.tw/dataset/19995",
		UpdateFreq:   30 * 24 * time.Hour,
		StaleAfter:   75 * 24 * time.Hour,
	}
}

func (p *DailyPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	rows, warnings, err := ParseDailyCSV(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(rows)
	if len(rows) == 0 {
		return res, errors.New("每日供需檔案沒有任何資料")
	}

	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	for _, row := range rows {
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.power_supply_daily
				(date, net_peak_capacity_mw, peak_load_mw, reserve_mw, reserve_pct, industrial_gwh, residential_gwh)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (date) DO UPDATE SET
				net_peak_capacity_mw = EXCLUDED.net_peak_capacity_mw,
				peak_load_mw = EXCLUDED.peak_load_mw,
				reserve_mw = EXCLUDED.reserve_mw,
				reserve_pct = EXCLUDED.reserve_pct,
				industrial_gwh = EXCLUDED.industrial_gwh,
				residential_gwh = EXCLUDED.residential_gwh,
				ingested_at = now()
			WHERE (data.power_supply_daily.net_peak_capacity_mw, data.power_supply_daily.peak_load_mw,
			       data.power_supply_daily.reserve_mw, data.power_supply_daily.reserve_pct,
			       data.power_supply_daily.industrial_gwh, data.power_supply_daily.residential_gwh)
			      IS DISTINCT FROM
			      (EXCLUDED.net_peak_capacity_mw, EXCLUDED.peak_load_mw, EXCLUDED.reserve_mw,
			       EXCLUDED.reserve_pct, EXCLUDED.industrial_gwh, EXCLUDED.residential_gwh)`,
			row.Date, row.NetPeakCapMW, row.PeakLoadMW, row.ReserveMW, row.ReservePct,
			row.IndustrialGWh, row.ResidentialGWh)
		if err != nil {
			return res, fmt.Errorf("upsert daily supply %s: %w", row.Date.Format(time.DateOnly), err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
		res.ObserveTime(row.Date)
	}
	return res, tx.Commit()
}

type ForecastPipeline struct {
	URL string
}

func (p *ForecastPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "power_supply_forecast",
		Name:         "未來二個月電力供需預測",
		SourceAgency: "台灣電力公司",
		SourceURL:    "https://data.gov.tw/dataset/103262",
		UpdateFreq:   10 * 24 * time.Hour,
		StaleAfter:   15 * 24 * time.Hour,
	}
}

func (p *ForecastPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	rows, warnings, err := ParseForecastCSV(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(rows)
	if len(rows) == 0 {
		return res, errors.New("預測檔案沒有任何資料")
	}

	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	latestEdition, latestRows, err := loadLatestForecast(ctx, tx)
	if err != nil {
		return res, err
	}
	if !latestEdition.IsZero() && sameForecast(rows, latestRows) {
		res.ObserveTime(latestEdition)
		return res, tx.Commit()
	}

	published := pipeline.TaipeiDate(env.Now())
	for _, row := range rows {
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.power_supply_forecast
				(forecast_date, published_on, net_peak_capacity_mw, peak_load_mw, reserve_mw, reserve_pct)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (forecast_date, published_on) DO UPDATE SET
				net_peak_capacity_mw = EXCLUDED.net_peak_capacity_mw,
				peak_load_mw = EXCLUDED.peak_load_mw,
				reserve_mw = EXCLUDED.reserve_mw,
				reserve_pct = EXCLUDED.reserve_pct,
				ingested_at = now()`,
			row.Date, published, row.NetPeakCapMW, row.PeakLoadMW, row.ReserveMW, row.ReservePct)
		if err != nil {
			return res, fmt.Errorf("insert forecast %s: %w", row.Date.Format(time.DateOnly), err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	res.ObserveTime(published)
	return res, tx.Commit()
}

func loadLatestForecast(ctx context.Context, tx *sql.Tx) (time.Time, []SupplyRow, error) {
	var edition sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT max(published_on) FROM data.power_supply_forecast`).Scan(&edition); err != nil {
		return time.Time{}, nil, err
	}
	if !edition.Valid {
		return time.Time{}, nil, nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT forecast_date, net_peak_capacity_mw, peak_load_mw, reserve_mw, reserve_pct
		  FROM data.power_supply_forecast WHERE published_on = $1 ORDER BY forecast_date`, edition.Time)
	if err != nil {
		return time.Time{}, nil, err
	}
	defer rows.Close()
	var out []SupplyRow
	for rows.Next() {
		var r SupplyRow
		if err := rows.Scan(&r.Date, &r.NetPeakCapMW, &r.PeakLoadMW, &r.ReserveMW, &r.ReservePct); err != nil {
			return time.Time{}, nil, err
		}
		r.Date = pipeline.DateColumn(r.Date)
		out = append(out, r)
	}
	return pipeline.DateColumn(edition.Time), out, rows.Err()
}

func sameForecast(a, b []SupplyRow) bool {
	if len(a) != len(b) {
		return false
	}
	byDate := map[string]SupplyRow{}
	for _, r := range b {
		byDate[r.Date.Format(time.DateOnly)] = r
	}
	for _, r := range a {
		o, ok := byDate[r.Date.Format(time.DateOnly)]
		if !ok || !sameNum(r.NetPeakCapMW, o.NetPeakCapMW) || !sameNum(r.PeakLoadMW, o.PeakLoadMW) ||
			!sameNum(r.ReserveMW, o.ReserveMW) || !sameNum(r.ReservePct, o.ReservePct) {
			return false
		}
	}
	return true
}

func sameNum(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return round(*a, 3) == round(*b, 3)
}
