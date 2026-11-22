package reservoir

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const (
	DefaultDailyURL    = "https://opendata.wra.gov.tw/api/v2/51023e88-4c76-4dbc-bbb9-470da690d539?sort=_importdate%20asc&format=JSON"
	DefaultBasicURL    = "https://opendata.wra.gov.tw/api/v2/708a43b0-24dc-40b7-9ed2-fca6a291e7ae?sort=_importdate%20asc&format=JSON"
	DefaultRealtimeURL = "https://opendata.wra.gov.tw/api/v2/2be9044c-6e44-4856-aad5-dd108c2e6679?sort=_importdate%20asc&format=JSON"
)

const MajorCapacity = 1000.0

var countyOverrides = map[string]string{
	"10211": "65000", // 青潭堰，新北市新店區
	"10802": "10002", // 羅東攔河堰，宜蘭縣
	"31002": "64000", // 甲仙攔河堰，高雄市甲仙區
}

var countyByIDPrefix = map[string]string{
	"501": "10016",
	"502": "09020",
	"503": "09007",
}

func CountyFor(id, township string, counties *pipeline.Counties) string {
	if code, ok := countyOverrides[id]; ok {
		return code
	}
	if len(id) >= 3 {
		if code, ok := countyByIDPrefix[id[:3]]; ok {
			return code
		}
	}
	if code, ok := counties.CodeFromAddress(township); ok {
		return code
	}
	return ""
}

type RefPipeline struct {
	DailyURL string
	BasicURL string
}

func (p *RefPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "reservoir_ref",
		Name:         "水庫主檔（每日營運狀況、基本資料）",
		SourceAgency: "經濟部水利署",
		SourceURL:    "https://data.gov.tw/dataset/41568",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   4 * 24 * time.Hour,
	}
}

type mergedReservoir struct {
	id, name, county string
	capacity         *float64
	capacityDate     time.Time
}

func (p *RefPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result

	dailyBody, err := env.Fetch.Get(ctx, p.DailyURL, p.DailyURL)
	if err != nil {
		return res, err
	}
	daily, warnings, err := ParseDaily(dailyBody)
	if err != nil {
		return res, err
	}
	res.Warnings = append(res.Warnings, warnings...)

	basicBody, err := env.Fetch.Get(ctx, p.BasicURL, p.BasicURL)
	if err != nil {
		return res, err
	}
	basic, warnings, err := ParseBasic(basicBody)
	if err != nil {
		return res, err
	}
	res.Warnings = append(res.Warnings, warnings...)

	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}

	today := pipeline.TaipeiDate(env.Now())
	merged := map[string]*mergedReservoir{}
	for _, b := range basic {
		merged[b.ReservoirID] = &mergedReservoir{
			id: b.ReservoirID, name: b.Name, capacity: b.Capacity, capacityDate: today,
			county: CountyFor(b.ReservoirID, b.Township, counties),
		}
	}
	for _, d := range daily {
		m, ok := merged[d.ReservoirID]
		if !ok {
			m = &mergedReservoir{id: d.ReservoirID, county: CountyFor(d.ReservoirID, "", counties)}
			merged[d.ReservoirID] = m
		}
		if d.Name != "" {
			m.name = d.Name
		}
		if d.Capacity != nil {
			m.capacity = d.Capacity
			m.capacityDate = today
			if !d.Date.IsZero() {
				m.capacityDate = pipeline.TaipeiDate(d.Date)
			}
		}
		res.ObserveTime(d.Date)
	}

	ids := make([]string, 0, len(merged))
	for id := range merged {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	res.RowsFetched = len(ids)

	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	for _, id := range ids {
		m := merged[id]
		if m.name == "" {
			continue // the observation pipeline creates a placeholder if needed
		}
		isMajor := m.capacity != nil && *m.capacity >= MajorCapacity
		r, err := tx.ExecContext(ctx, `
			INSERT INTO ref.reservoir (reservoir_id, name, county_code, effective_capacity, capacity_updated_on, is_major, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, now())
			ON CONFLICT (reservoir_id) DO UPDATE SET
				name = EXCLUDED.name,
				-- an administrator's county stays; only fill it when missing
				county_code = COALESCE(ref.reservoir.county_code, EXCLUDED.county_code),
				-- a capacity an administrator set (capacity_locked) is kept
				capacity_updated_on = CASE
					WHEN NOT ref.reservoir.capacity_locked
					 AND EXCLUDED.effective_capacity IS NOT NULL
					 AND EXCLUDED.effective_capacity IS DISTINCT FROM ref.reservoir.effective_capacity
					THEN EXCLUDED.capacity_updated_on
					ELSE ref.reservoir.capacity_updated_on END,
				effective_capacity = CASE WHEN ref.reservoir.capacity_locked
				                          THEN ref.reservoir.effective_capacity
				                          ELSE COALESCE(EXCLUDED.effective_capacity, ref.reservoir.effective_capacity) END,
				-- decide is_major once, when the capacity first becomes known
				is_major = CASE WHEN ref.reservoir.effective_capacity IS NULL
				                THEN EXCLUDED.is_major ELSE ref.reservoir.is_major END,
				updated_at = now()
			-- skip no-op updates so rows_inserted counts real changes
			WHERE (ref.reservoir.name, ref.reservoir.county_code, ref.reservoir.effective_capacity)
			      IS DISTINCT FROM
			      (EXCLUDED.name,
			       COALESCE(ref.reservoir.county_code, EXCLUDED.county_code),
			       CASE WHEN ref.reservoir.capacity_locked THEN ref.reservoir.effective_capacity
			            ELSE COALESCE(EXCLUDED.effective_capacity, ref.reservoir.effective_capacity) END)`,
			m.id, m.name, pipeline.NullIfEmpty(m.county), m.capacity, m.capacityDate, isMajor)
		if err != nil {
			return res, fmt.Errorf("upsert reservoir %s: %w", m.id, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT reservoir_id || ' ' || name FROM ref.reservoir WHERE county_code IS NULL ORDER BY reservoir_id`)
	if err != nil {
		return res, err
	}
	var noCounty []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return res, err
		}
		noCounty = append(noCounty, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}
	if len(noCounty) > 0 {
		res.Warnf("%d 座水庫沒有縣市，請在後台補上：%s", len(noCounty), strings.Join(noCounty, "、"))
	}
	return res, tx.Commit()
}
