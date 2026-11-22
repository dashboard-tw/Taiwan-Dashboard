package reservoir

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

type ObsPipeline struct {
	URL string
}

func (p *ObsPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "reservoir_obs",
		Name:         "水庫水情（每小時）",
		SourceAgency: "經濟部水利署",
		SourceURL:    "https://data.gov.tw/dataset/45501",
		UpdateFreq:   time.Hour,
		StaleAfter:   3 * time.Hour,
	}
}

var obsColumns = []string{
	"reservoir_id", "observed_at", "water_level_m", "effective_storage", "storage_pct",
	"inflow_cms", "total_outflow_cms", "rainfall_mm", "status_type",
}

var (
	insertHistorySQL = fmt.Sprintf(
		`INSERT INTO data.reservoir_obs_history (%s) VALUES (%s) ON CONFLICT (reservoir_id, observed_at) DO NOTHING`,
		strings.Join(obsColumns, ", "), pipeline.Placeholders(len(obsColumns)))
	upsertCurrentSQL = fmt.Sprintf(
		`INSERT INTO data.reservoir_obs_current (%s) VALUES (%s)
		 ON CONFLICT (reservoir_id) DO UPDATE SET %s, ingested_at = now()
		 WHERE data.reservoir_obs_current.observed_at <= EXCLUDED.observed_at`,
		strings.Join(obsColumns, ", "), pipeline.Placeholders(len(obsColumns)), pipeline.SetExcluded(obsColumns[1:]))
)

func StoragePct(storage, capacity *float64) *float64 {
	if storage == nil || capacity == nil || *capacity <= 0 {
		return nil
	}
	v := math.Round(*storage / *capacity * 10000) / 100
	return &v
}

func FixStorageUnit(storage, capacity *float64) (*float64, bool) {
	if storage == nil || capacity == nil || *capacity <= 0 || *storage <= *capacity*10 {
		return storage, false
	}
	v := *storage / 10000
	if v > *capacity*1.5 {
		return storage, false
	}
	return &v, true
}

const futureTolerance = 10 * time.Minute

func (p *ObsPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result

	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	obs, warnings, err := ParseRealtime(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(obs)
	if len(obs) == 0 {
		return res, fmt.Errorf("水庫水情回應沒有任何資料")
	}

	capacities, err := loadCapacities(ctx, env.DB)
	if err != nil {
		return res, err
	}

	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	unknown := map[string]bool{}
	overfull := map[string]float64{}
	converted := map[string]bool{}
	cutoff := env.Now().Add(futureTolerance)
	future := 0
	for _, o := range obs {
		if !o.HasData() {
			continue
		}
		if o.ObservedAt.After(cutoff) {
			future++
			continue
		}
		capacity, known := capacities[o.ReservoirID]
		if !known && !unknown[o.ReservoirID] {
			unknown[o.ReservoirID] = true
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO ref.reservoir (reservoir_id, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				o.ReservoirID, "水庫 "+o.ReservoirID); err != nil {
				return res, fmt.Errorf("insert placeholder reservoir %s: %w", o.ReservoirID, err)
			}
		}

		if fixed, ok := FixStorageUnit(o.Storage, capacity); ok {
			o.Storage = fixed
			converted[o.ReservoirID] = true
		}
		pct := StoragePct(o.Storage, capacity)
		if pct != nil && *pct > 110 {
			overfull[o.ReservoirID] = *pct
		}
		args := []any{o.ReservoirID, o.ObservedAt, o.WaterLevel, o.Storage, pct,
			o.Inflow, o.TotalOutflow, o.Rainfall, o.StatusType}

		r, err := tx.ExecContext(ctx, insertHistorySQL, args...)
		if err != nil {
			return res, fmt.Errorf("insert reservoir history %s: %w", o.ReservoirID, err)
		}
		if _, err := tx.ExecContext(ctx, upsertCurrentSQL, args...); err != nil {
			return res, fmt.Errorf("upsert reservoir current %s: %w", o.ReservoirID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
		res.ObserveTime(o.ObservedAt)
	}

	if future > 0 {
		env.Logger.Info("skipped readings timed in the future", "count", future)
	}
	if len(unknown) > 0 {
		res.Warnf("%d 座水庫不在主檔中，已建立暫時名稱：%s", len(unknown), strings.Join(sortedKeys(unknown), "、"))
	}
	for _, id := range sortedKeys(converted) {
		res.Warnf("水庫 %s 的蓄水量看起來是立方公尺，已換算成萬立方公尺", id)
	}
	for _, id := range sortedKeys(overfull) {
		res.Warnf("水庫 %s 蓄水率 %.1f%%，有效容量可能需要更新", id, overfull[id])
	}
	return res, tx.Commit()
}

func loadCapacities(ctx context.Context, db *sql.DB) (map[string]*float64, error) {
	rows, err := db.QueryContext(ctx, `SELECT reservoir_id, effective_capacity FROM ref.reservoir`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*float64{}
	for rows.Next() {
		var id string
		var capacity sql.NullFloat64
		if err := rows.Scan(&id, &capacity); err != nil {
			return nil, err
		}
		if capacity.Valid {
			v := capacity.Float64
			out[id] = &v
		} else {
			out[id] = nil
		}
	}
	return out, rows.Err()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
