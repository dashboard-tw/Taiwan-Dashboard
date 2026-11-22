package power

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const (
	DefaultRegionURL = "https://service.taipower.com.tw/data/opendata/apply/file/d006019/001.csv"
	DefaultLoadURL   = "https://service.taipower.com.tw/data/opendata/apply/file/d006020/001.json"
)

var Regions = []string{"北部", "中部", "南部", "東部"}

type RegionReading struct {
	ObservedAt time.Time
	Region     string
	GenMW      *float64
	LoadMW     *float64
}

func ParseRegionCSV(body []byte) ([]RegionReading, []string, error) {
	r := csv.NewReader(bytes.NewReader(pipeline.StripBOM(body)))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("read CSV header: %w", err)
	}
	index := map[string]int{}
	for i, h := range header {
		index[cleanField(h)] = i
	}
	cols := []string{"時間", "區域", "發電(萬瓩)", "用電(萬瓩)"}
	for _, c := range cols {
		if _, ok := index[c]; !ok {
			return nil, nil, fmt.Errorf("CSV 缺少「%s」欄位", c)
		}
	}
	known := map[string]bool{}
	for _, reg := range Regions {
		known[reg] = true
	}

	var out []RegionReading
	var warnings []string
	for line := 2; ; line++ {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read CSV line %d: %w", line, err)
		}
		field := func(name string) string {
			if i := index[name]; i < len(rec) {
				return cleanField(rec[i])
			}
			return ""
		}
		if field("時間") == "" && field("區域") == "" {
			continue
		}
		at, err := pipeline.ParseTaipeiTime(field("時間"), "20060102T150405", "20060102T1504")
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("第 %d 行時間：%v", line, err))
			continue
		}
		region := field("區域")
		if !known[region] {
			warnings = append(warnings, fmt.Sprintf("第 %d 行未知的區域「%s」", line, region))
			continue
		}
		row := RegionReading{ObservedAt: at, Region: region}
		for _, c := range []struct {
			name string
			dst  **float64
		}{{"發電(萬瓩)", &row.GenMW}, {"用電(萬瓩)", &row.LoadMW}} {
			v, err := wanKW(field(c.name))
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("第 %d 行「%s」：%v", line, c.name, err))
			}
			*c.dst = v
		}
		out = append(out, row)
	}
	return out, warnings, nil
}

func cleanField(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, string(rune(0xFEFF)), ""))
}

func wanKW(s string) (*float64, error) {
	v, err := pipeline.ParseNumber(s)
	if err != nil || v == nil {
		return nil, err
	}
	mw := round(*v*mwPerWanKW, 3)
	return &mw, nil
}

type LoadSnapshot struct {
	ObservedAt     time.Time
	LoadMW         *float64
	UtilPct        *float64
	ForeSupplyMW   *float64
	ForePeakLoadMW *float64
	ForeReserveMW  *float64
	ForeReservePct *float64
	ForeIndicator  string
	ForePeakHours  string
	Yesterday      *PeakDay
}

type PeakDay struct {
	Date       time.Time
	SupplyMW   *float64
	PeakLoadMW *float64
	ReserveMW  *float64
	ReservePct *float64
	Indicator  string
}

var rocDate = regexp.MustCompile(`^(\d{2,3})\.(\d{1,2})\.(\d{1,2})(?:\s*\([^)]*\)\s*(\d{1,2}):(\d{2}))?$`)

func parseROCTime(s string) (time.Time, error) {
	m := rocDate.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return time.Time{}, fmt.Errorf("unrecognised time %q", s)
	}
	n := make([]int, 6)
	for i := 1; i <= 5; i++ {
		if m[i] != "" {
			n[i], _ = strconv.Atoi(m[i])
		}
	}
	t := time.Date(n[1]+1911, time.Month(n[2]), n[3], n[4], n[5], 0, 0, pipeline.Taipei)
	if t.Month() != time.Month(n[2]) || t.Day() != n[3] || n[4] > 23 {
		return time.Time{}, fmt.Errorf("invalid date %q", s)
	}
	return t, nil
}

var indicators = map[string]bool{"G": true, "Y": true, "O": true, "R": true, "B": true}

func ParseLoadJSON(body []byte) (LoadSnapshot, []string, error) {
	var file struct {
		Records []map[string]string `json:"records"`
	}
	if err := json.NewDecoder(bytes.NewReader(pipeline.StripBOM(body))).Decode(&file); err != nil {
		return LoadSnapshot{}, nil, fmt.Errorf("decode load file: %w", err)
	}
	f := map[string]string{}
	for _, rec := range file.Records {
		for k, v := range rec {
			f[k] = strings.TrimSpace(v)
		}
	}
	at, err := parseROCTime(f["publish_time"])
	if err != nil {
		return LoadSnapshot{}, nil, fmt.Errorf("load file publish_time: %w", err)
	}

	var warnings []string
	num := func(key string, scale float64) *float64 {
		v, err := pipeline.ParseNumber(f[key])
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s：%v", key, err))
			return nil
		}
		if v != nil {
			scaled := round(*v*scale, 3)
			v = &scaled
		}
		return v
	}
	light := func(key string) string {
		v := strings.ToUpper(f[key])
		if v != "" && !indicators[v] {
			warnings = append(warnings, fmt.Sprintf("%s 未知的燈號「%s」", key, v))
			return ""
		}
		return v
	}

	snap := LoadSnapshot{
		ObservedAt:     at,
		LoadMW:         num("curr_load", mwPerWanKW),
		UtilPct:        num("curr_util_rate", 1),
		ForeSupplyMW:   num("fore_maxi_sply_capacity", mwPerWanKW),
		ForePeakLoadMW: num("fore_peak_dema_load", mwPerWanKW),
		ForeReserveMW:  num("fore_peak_resv_capacity", mwPerWanKW),
		ForeReservePct: num("fore_peak_resv_rate", 1),
		ForeIndicator:  light("fore_peak_resv_indicator"),
		ForePeakHours:  f["fore_peak_hour_range"],
	}
	if f["yday_date"] != "" {
		if d, err := parseROCTime(f["yday_date"]); err != nil {
			warnings = append(warnings, fmt.Sprintf("yday_date：%v", err))
		} else {
			snap.Yesterday = &PeakDay{
				Date:       d,
				SupplyMW:   num("yday_maxi_sply_capacity", mwPerWanKW),
				PeakLoadMW: num("yday_peak_dema_load", mwPerWanKW),
				ReserveMW:  num("yday_peak_resv_capacity", mwPerWanKW),
				ReservePct: num("yday_peak_resv_rate", 1),
				Indicator:  light("yday_peak_resv_indicator"),
			}
		}
	}
	return snap, warnings, nil
}

type RegionPipeline struct {
	URL string
}

func (p *RegionPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "power_region",
		Name:         "各區發電與用電（每 10 分鐘）",
		SourceAgency: "台灣電力公司",
		SourceURL:    "https://data.gov.tw/dataset/162596",
		UpdateFreq:   10 * time.Minute,
		StaleAfter:   40 * time.Minute,
	}
}

func (p *RegionPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	rows, warnings, err := ParseRegionCSV(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(rows)
	if len(rows) == 0 {
		return res, errors.New("區域別用電檔案沒有任何資料")
	}

	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, row := range rows {
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.power_region_history (observed_at, region, gen_mw, load_mw)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (observed_at, region) DO NOTHING`,
			row.ObservedAt, row.Region, row.GenMW, row.LoadMW)
		if err != nil {
			return res, fmt.Errorf("insert region %s: %w", row.Region, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
		res.ObserveTime(row.ObservedAt)
	}
	return res, tx.Commit()
}

type LoadPipeline struct {
	URL string
}

func (p *LoadPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "power_load",
		Name:         "今日用電量與供電能力（每 10 分鐘）",
		SourceAgency: "台灣電力公司",
		SourceURL:    "https://data.gov.tw/dataset/162595",
		UpdateFreq:   10 * time.Minute,
		StaleAfter:   40 * time.Minute,
	}
}

func (p *LoadPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	s, warnings, err := ParseLoadJSON(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = 1
	if s.LoadMW == nil {
		return res, errors.New("今日供需檔案沒有目前用電量")
	}
	res.ObserveTime(s.ObservedAt)

	r, err := env.DB.ExecContext(ctx, `
		INSERT INTO data.power_load_history
			(observed_at, load_mw, util_pct, fore_supply_mw, fore_peak_load_mw,
			 fore_reserve_mw, fore_reserve_pct, fore_indicator, fore_peak_hours)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (observed_at) DO NOTHING`,
		s.ObservedAt, s.LoadMW, s.UtilPct, s.ForeSupplyMW, s.ForePeakLoadMW,
		s.ForeReserveMW, s.ForeReservePct, pipeline.NullIfEmpty(s.ForeIndicator),
		pipeline.NullIfEmpty(s.ForePeakHours))
	if err != nil {
		return res, fmt.Errorf("insert load: %w", err)
	}
	n, _ := r.RowsAffected()
	res.RowsInserted = int(n)
	return res, nil
}

type PeakPipeline struct {
	URL string
}

func (p *PeakPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "power_peak_daily",
		Name:         "昨日尖峰備轉（每天）",
		SourceAgency: "台灣電力公司",
		SourceURL:    "https://data.gov.tw/dataset/162595",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   3 * 24 * time.Hour,
	}
}

func (p *PeakPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	s, warnings, err := ParseLoadJSON(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	y := s.Yesterday
	if y == nil {
		return res, errors.New("今日供需檔案沒有昨日尖峰")
	}
	res.RowsFetched = 1
	res.ObserveTime(y.Date)

	r, err := env.DB.ExecContext(ctx, `
		INSERT INTO data.power_peak_daily (date, supply_mw, peak_load_mw, reserve_mw, reserve_pct, indicator)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (date) DO UPDATE SET
			supply_mw = EXCLUDED.supply_mw,
			peak_load_mw = EXCLUDED.peak_load_mw,
			reserve_mw = EXCLUDED.reserve_mw,
			reserve_pct = EXCLUDED.reserve_pct,
			indicator = EXCLUDED.indicator,
			ingested_at = now()
		WHERE (data.power_peak_daily.supply_mw, data.power_peak_daily.peak_load_mw,
		       data.power_peak_daily.reserve_mw, data.power_peak_daily.reserve_pct,
		       data.power_peak_daily.indicator)
		      IS DISTINCT FROM
		      (EXCLUDED.supply_mw, EXCLUDED.peak_load_mw, EXCLUDED.reserve_mw,
		       EXCLUDED.reserve_pct, EXCLUDED.indicator)`,
		y.Date, y.SupplyMW, y.PeakLoadMW, y.ReserveMW, y.ReservePct, pipeline.NullIfEmpty(y.Indicator))
	if err != nil {
		return res, fmt.Errorf("upsert yesterday's peak: %w", err)
	}
	n, _ := r.RowsAffected()
	res.RowsInserted = int(n)
	return res, nil
}
