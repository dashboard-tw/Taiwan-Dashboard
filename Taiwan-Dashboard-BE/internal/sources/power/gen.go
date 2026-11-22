package power

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const DefaultGenURL = "https://service.taipower.com.tw/data/opendata/apply/file/d006001/001.json"

const privatePrefix = "民營電廠-"

var fuelTypes = map[string]string{
	"燃氣":     "gas",
	"燃煤":     "coal",
	"燃料油":    "oil",
	"輕油":     "oil",
	"汽電共生":   "cogen",
	"核能":     "nuclear",
	"太陽能":    "solar",
	"風力":     "wind",
	"水力":     "hydro",
	"儲能":     "storage",
	"儲能負載":   "storage_load",
	"其它再生能源": "other_renewable",
	"其他再生能源": "other_renewable",
}

type UnitReading struct {
	RawType    string // Taipower's type, e.g. 民營電廠-燃氣
	FuelType   string
	IsPrivate  bool
	Name       string
	Plant      string
	CapacityMW *float64
	NetMW      *float64
	Remark     string
}

type Snapshot struct {
	ObservedAt time.Time
	Units      []UnitReading
	Subtotals  map[string]float64
}

var (
	htmlTag     = regexp.MustCompile(`<[^>]*>`)
	noteSuffix  = regexp.MustCompile(`\(註\d+\)`)
	subtotalNum = regexp.MustCompile(`^\s*(-?[\d,.]+)`)
)

func normalizeType(raw string) string {
	s := htmlTag.ReplaceAllString(raw, "")
	if i := strings.IndexAny(s, "(（"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func PlantName(unit string) string {
	name := strings.TrimSpace(noteSuffix.ReplaceAllString(unit, ""))
	if i := strings.Index(name, "#"); i > 0 {
		base := strings.TrimRight(name[:i], "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
		if base != "" {
			return base
		}
	}
	return name
}

func ParseGen(body []byte) (Snapshot, []string, error) {
	var file struct {
		DateTime string              `json:"DateTime"`
		AaData   []map[string]string `json:"aaData"`
	}
	if err := json.NewDecoder(bytes.NewReader(pipeline.StripBOM(body))).Decode(&file); err != nil {
		return Snapshot{}, nil, fmt.Errorf("decode generation file: %w", err)
	}
	observed, err := pipeline.ParseTaipeiTime(file.DateTime, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02 15:04")
	if err != nil {
		return Snapshot{}, nil, fmt.Errorf("generation file time: %w", err)
	}

	snap := Snapshot{ObservedAt: observed, Subtotals: map[string]float64{}}
	var warnings []string
	unknownTypes := map[string]bool{}
	for _, row := range file.AaData {
		rawType := normalizeType(row["機組類型"])
		name := strings.TrimSpace(row["機組名稱"])
		if rawType == "" || name == "" {
			continue
		}

		if strings.HasPrefix(name, "小計") {
			if m := subtotalNum.FindStringSubmatch(row["淨發電量(MW)"]); m != nil {
				if v, err := pipeline.ParseNumber(m[1]); err == nil && v != nil {
					snap.Subtotals[rawType] = *v
				}
			}
			continue
		}

		u := UnitReading{RawType: rawType, Name: name, Plant: PlantName(name), Remark: strings.TrimSpace(row["備註"])}
		base := rawType
		if strings.HasPrefix(base, privatePrefix) {
			u.IsPrivate = true
			base = strings.TrimPrefix(base, privatePrefix)
		}
		fuel, ok := fuelTypes[base]
		if !ok {
			fuel = "other"
			unknownTypes[rawType] = true
		}
		u.FuelType = fuel

		if u.CapacityMW, err = pipeline.ParseNumber(row["裝置容量(MW)"]); err != nil {
			warnings = append(warnings, fmt.Sprintf("機組 %s 裝置容量：%v", name, err))
		}
		if u.NetMW, err = pipeline.ParseNumber(row["淨發電量(MW)"]); err != nil {
			warnings = append(warnings, fmt.Sprintf("機組 %s 淨發電量：%v", name, err))
		}
		snap.Units = append(snap.Units, u)
	}
	for t := range unknownTypes {
		warnings = append(warnings, fmt.Sprintf("未知的機組類型「%s」，暫時歸類為 other", t))
	}
	sort.Strings(warnings)
	return snap, warnings, nil
}

type FuelTotal struct {
	FuelType string
	NetMW    float64
	SharePct *float64
}

func Aggregate(units []UnitReading) []FuelTotal {
	sums := map[string]float64{}
	var total float64
	for _, u := range units {
		if u.NetMW == nil {
			continue
		}
		sums[u.FuelType] += *u.NetMW
		total += *u.NetMW
	}
	out := make([]FuelTotal, 0, len(sums))
	for fuel, mw := range sums {
		ft := FuelTotal{FuelType: fuel, NetMW: round(mw, 1)}
		if total > 0 {
			share := round(mw/total*100, 3)
			ft.SharePct = &share
		}
		out = append(out, ft)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FuelType < out[j].FuelType })
	return out
}

func CheckSubtotals(snap Snapshot) []string {
	sums := map[string]float64{}
	for _, u := range snap.Units {
		if u.NetMW != nil {
			sums[u.RawType] += *u.NetMW
		}
	}
	var warnings []string
	for rawType, want := range snap.Subtotals {
		if got := sums[rawType]; math.Abs(got-want) > 1 {
			warnings = append(warnings, fmt.Sprintf("%s 加總 %.1f MW 與台電小計 %.1f MW 不符", rawType, got, want))
		}
	}
	sort.Strings(warnings)
	return warnings
}

func round(v float64, decimals int) float64 {
	p := math.Pow(10, float64(decimals))
	return math.Round(v*p) / p
}

type GenPipeline struct {
	URL string
}

func (p *GenPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "power_gen",
		Name:         "各機組發電量（每 10 分鐘）",
		SourceAgency: "台灣電力公司",
		SourceURL:    "https://data.gov.tw/dataset/8931",
		UpdateFreq:   10 * time.Minute,
		StaleAfter:   40 * time.Minute,
	}
}

func (p *GenPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	snap, warnings, err := ParseGen(body)
	if err != nil {
		return res, err
	}
	res.Warnings = append(warnings, CheckSubtotals(snap)...)
	res.RowsFetched = len(snap.Units)
	if len(snap.Units) == 0 {
		return res, fmt.Errorf("發電量檔案沒有任何機組資料")
	}
	res.ObserveTime(snap.ObservedAt)

	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	for _, u := range snap.Units {
		var unitID int64
		err := tx.QueryRowContext(ctx, `
			INSERT INTO ref.power_unit (unit_name, plant_name, fuel_type, is_private)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (unit_name, fuel_type) DO UPDATE SET
				plant_name = EXCLUDED.plant_name,
				is_private = EXCLUDED.is_private,
				updated_at = now()
			RETURNING unit_id`,
			u.Name, u.Plant, u.FuelType, u.IsPrivate).Scan(&unitID)
		if err != nil {
			return res, fmt.Errorf("upsert unit %s: %w", u.Name, err)
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO data.power_gen_history (unit_id, observed_at, capacity_mw, net_gen_mw, remark)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (unit_id, observed_at) DO NOTHING`,
			unitID, snap.ObservedAt, u.CapacityMW, u.NetMW, pipeline.NullIfEmpty(u.Remark))
		if err != nil {
			return res, fmt.Errorf("insert generation for %s: %w", u.Name, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}

	totals := Aggregate(snap.Units)
	for _, t := range totals {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO data.power_gen_by_fuel_history (observed_at, fuel_type, net_gen_mw, share_pct)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (observed_at, fuel_type) DO NOTHING`,
			snap.ObservedAt, t.FuelType, t.NetMW, t.SharePct); err != nil {
			return res, fmt.Errorf("insert fuel totals: %w", err)
		}
	}
	if err := replaceCurrentTotals(ctx, tx, snap.ObservedAt, totals); err != nil {
		return res, err
	}
	return res, tx.Commit()
}

func replaceCurrentTotals(ctx context.Context, tx *sql.Tx, observed time.Time, totals []FuelTotal) error {
	var latest sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT max(observed_at) FROM data.power_gen_by_fuel_current`).Scan(&latest); err != nil {
		return err
	}
	if latest.Valid && latest.Time.After(observed) {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM data.power_gen_by_fuel_current`); err != nil {
		return err
	}
	for _, t := range totals {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO data.power_gen_by_fuel_current (observed_at, fuel_type, net_gen_mw, share_pct)
			VALUES ($1, $2, $3, $4)`,
			observed, t.FuelType, t.NetMW, t.SharePct); err != nil {
			return fmt.Errorf("insert current fuel totals: %w", err)
		}
	}
	return nil
}
