package river

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"dashboard.tw/backend/internal/pipeline"
)

const (
	DefaultStationURL = "https://opendata.wra.gov.tw/api/v2/c4acc691-7416-40ca-9464-292c0c00da92?sort=_importdate%20asc&format=JSON"
	DefaultLevelURL   = "https://opendata.wra.gov.tw/api/v2/73c4c3de-4045-4765-abeb-89f9f9cd5ff0?sort=_importdate%20asc&format=JSON"
)

const minX, maxX, minY, maxY = 50000, 400000, 2400000, 2850000

const maxOffCountyM = 5000

type Station struct {
	ID       string
	Name     string
	River    string
	Basin    string
	AreaCode string // 行政區代碼, the first five digits are the county
	Address  string
	X, Y     *float64 // TWD97 TM2
	Alerts   [3]*float64
	Active   bool
}

type Reading struct {
	StationID  string
	ObservedAt time.Time
	LevelM     *float64
}

func cleanName(s string) string {
	s = strings.TrimSpace(s)
	for _, suffix := range []string{"(即時)", "（即時）"} {
		s = strings.TrimSpace(strings.TrimSuffix(s, suffix))
	}
	return s
}

func ParseStations(body []byte) ([]Station, []string, error) {
	recs, err := pipeline.DecodeWRAList(body)
	if err != nil {
		return nil, nil, err
	}
	var out []Station
	var warnings []string
	for _, r := range recs {
		s := Station{
			ID:       strings.TrimSpace(r["basinidentifier"]),
			Name:     cleanName(r["observatoryname"]),
			River:    strings.TrimSpace(r["rivername"]),
			Basin:    strings.TrimSpace(r["affiliatedbasin"]),
			AreaCode: strings.TrimSpace(r["areacode"]),
			Address:  strings.TrimSpace(r["locationaddress"]),
			Active:   strings.TrimSpace(r["observationstatus"]) == "現存",
		}
		if s.ID == "" || len(s.ID) > 10 {
			warnings = append(warnings, fmt.Sprintf("站號「%s」無法使用", s.ID))
			continue
		}
		if s.Name == "" {
			s.Name = "水位站 " + s.ID
		}
		if xy := strings.Fields(r["locationbytwd97_xy"]); len(xy) == 2 {
			x, errX := pipeline.ParseNumber(xy[0])
			y, errY := pipeline.ParseNumber(xy[1])
			if errX == nil && errY == nil && x != nil && y != nil &&
				*x >= minX && *x <= maxX && *y >= minY && *y <= maxY {
				s.X, s.Y = x, y
			} else {
				warnings = append(warnings, fmt.Sprintf("水位站 %s 的座標「%s」超出範圍", s.ID, r["locationbytwd97_xy"]))
			}
		}
		for i, key := range []string{"alertlevel1", "alertlevel2", "alertlevel3"} {
			v, err := pipeline.ParseNumber(r[key])
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("水位站 %s 的 %s：%v", s.ID, key, err))
				continue
			}
			s.Alerts[i] = v
		}
		var prev *float64
		for _, a := range s.Alerts {
			if a == nil {
				continue
			}
			if prev != nil && *a >= *prev {
				warnings = append(warnings, fmt.Sprintf("水位站 %s 的警戒水位順序不合理，不使用", s.ID))
				s.Alerts = [3]*float64{}
				break
			}
			prev = a
		}
		out = append(out, s)
	}
	return out, warnings, nil
}

func ParseLevels(body []byte) ([]Reading, []string, error) {
	recs, err := pipeline.DecodeWRAList(body)
	if err != nil {
		return nil, nil, err
	}
	var out []Reading
	var warnings []string
	for i, r := range recs {
		id := strings.TrimSpace(r["stationid"])
		if id == "" || len(id) > 10 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(r["checkresult"]), "false") {
			continue
		}
		at, err := pipeline.ParseTaipeiTime(strings.ToUpper(r["datetime"]), "2006-01-02T15:04:05", "2006-01-02T15:04")
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("第 %d 筆（水位站 %s）時間：%v", i+1, id, err))
			continue
		}
		v, err := pipeline.ParseNumber(r["waterlevel"])
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("第 %d 筆（水位站 %s）水位：%v", i+1, id, err))
			continue
		}
		if v == nil {
			continue
		}
		out = append(out, Reading{StationID: id, ObservedAt: at, LevelM: v})
	}
	return out, warnings, nil
}

type StationPipeline struct {
	URL string
}

func (p *StationPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "river_station",
		Name:         "河川水位站與警戒水位",
		SourceAgency: "經濟部水利署",
		SourceURL:    "https://data.gov.tw/dataset/22227",
		UpdateFreq:   24 * time.Hour,
		StaleAfter:   4 * 24 * time.Hour,
	}
}

const pointSQL = `ST_Transform(ST_SetSRID(ST_MakePoint($1, $2), 3826), 4326)`

func (p *StationPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	stations, warnings, err := ParseStations(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(stations)
	if len(stations) == 0 {
		return res, errors.New("水位站站況沒有任何資料")
	}

	counties, err := pipeline.LoadCounties(ctx, env.DB)
	if err != nil {
		return res, err
	}
	codes := map[string]bool{}
	rows, err := env.DB.QueryContext(ctx, `SELECT code FROM ref.county`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return res, err
		}
		codes[c] = true
	}
	rows.Close()

	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	var misplaced []string
	for _, s := range stations {
		county := ""
		if len(s.AreaCode) >= 5 && codes[s.AreaCode[:5]] {
			county = s.AreaCode[:5]
		} else if c, ok := counties.CodeFromAddress(s.Address); ok {
			county = c
		}

		hasPoint := s.X != nil
		switch {
		case hasPoint && county == "":
			var inside *string
			if err := tx.QueryRowContext(ctx,
				`SELECT code FROM ref.county WHERE ST_Contains(boundary, `+pointSQL+`) LIMIT 1`,
				*s.X, *s.Y).Scan(&inside); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return res, fmt.Errorf("locate station %s: %w", s.ID, err)
			}
			if inside != nil {
				county = *inside
			}
		case hasPoint:
			var near bool
			if err := tx.QueryRowContext(ctx,
				`SELECT ST_DWithin(boundary::geography, (`+pointSQL+`)::geography, $4) FROM ref.county WHERE code = $3`,
				*s.X, *s.Y, county, maxOffCountyM).Scan(&near); err != nil {
				return res, fmt.Errorf("check station %s: %w", s.ID, err)
			}
			if !near {
				hasPoint = false
				misplaced = append(misplaced, s.ID)
			}
		}

		var x, y any
		if hasPoint {
			x, y = *s.X, *s.Y
		}
		r, err := tx.ExecContext(ctx, `
			INSERT INTO ref.river_station
				(station_id, name, river_name, basin_code, county_code, address, geom,
				 alert1_m, alert2_m, alert3_m, is_active)
			VALUES ($3, $4, $5, $6, $7, $8,
			        CASE WHEN $1::float8 IS NULL THEN NULL ELSE `+pointSQL+` END,
			        $9, $10, $11, $12)
			ON CONFLICT (station_id) DO UPDATE SET
				name = EXCLUDED.name, river_name = EXCLUDED.river_name, basin_code = EXCLUDED.basin_code,
				county_code = EXCLUDED.county_code, address = EXCLUDED.address, geom = EXCLUDED.geom,
				alert1_m = EXCLUDED.alert1_m, alert2_m = EXCLUDED.alert2_m, alert3_m = EXCLUDED.alert3_m,
				is_active = EXCLUDED.is_active, updated_at = now()
			WHERE (ref.river_station.name, ref.river_station.river_name, ref.river_station.basin_code,
			       ref.river_station.county_code, ref.river_station.address, ref.river_station.geom,
			       ref.river_station.alert1_m, ref.river_station.alert2_m, ref.river_station.alert3_m,
			       ref.river_station.is_active)
			      IS DISTINCT FROM
			      (EXCLUDED.name, EXCLUDED.river_name, EXCLUDED.basin_code, EXCLUDED.county_code,
			       EXCLUDED.address, EXCLUDED.geom, EXCLUDED.alert1_m, EXCLUDED.alert2_m,
			       EXCLUDED.alert3_m, EXCLUDED.is_active)`,
			x, y, s.ID, s.Name, pipeline.NullIfEmpty(s.River), pipeline.NullIfEmpty(s.Basin),
			pipeline.NullIfEmpty(county), pipeline.NullIfEmpty(s.Address),
			s.Alerts[0], s.Alerts[1], s.Alerts[2], s.Active)
		if err != nil {
			return res, fmt.Errorf("upsert station %s: %w", s.ID, err)
		}
		n, _ := r.RowsAffected()
		res.RowsInserted += int(n)
	}
	if len(misplaced) > 0 {
		sort.Strings(misplaced)
		res.Warnf("%d 個水位站的座標與站址的縣市不符，不存座標：%s", len(misplaced), strings.Join(misplaced, "、"))
	}
	res.ObserveTime(env.Now())
	return res, tx.Commit()
}

type LevelPipeline struct {
	URL string
}

func (p *LevelPipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID:           "river_level",
		Name:         "河川即時水位（每 10 分鐘）",
		SourceAgency: "經濟部水利署",
		SourceURL:    "https://data.gov.tw/dataset/25768",
		UpdateFreq:   10 * time.Minute,
		StaleAfter:   time.Hour,
	}
}

func (p *LevelPipeline) Run(ctx context.Context, env pipeline.Env) (pipeline.Result, error) {
	var res pipeline.Result
	body, err := env.Fetch.Get(ctx, p.URL, p.URL)
	if err != nil {
		return res, err
	}
	readings, warnings, err := ParseLevels(body)
	if err != nil {
		return res, err
	}
	res.Warnings = warnings
	res.RowsFetched = len(readings)
	if len(readings) == 0 {
		return res, errors.New("即時水位沒有任何資料")
	}

	tx, err := env.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	known := map[string]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT station_id FROM ref.river_station`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return res, err
		}
		known[id] = true
	}
	rows.Close()

	cutoff := env.Now().Add(15 * time.Minute)
	var future int
	var unknown []string
	for _, r := range readings {
		if r.ObservedAt.After(cutoff) {
			future++
			continue
		}
		if !known[r.StationID] {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO ref.river_station (station_id, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				r.StationID, "水位站 "+r.StationID); err != nil {
				return res, fmt.Errorf("insert placeholder station %s: %w", r.StationID, err)
			}
			known[r.StationID] = true
			unknown = append(unknown, r.StationID)
		}

		h, err := tx.ExecContext(ctx, `
			INSERT INTO data.river_level_history (station_id, observed_at, water_level_m)
			VALUES ($1, $2, $3)
			ON CONFLICT (station_id, observed_at) DO NOTHING`,
			r.StationID, r.ObservedAt, r.LevelM)
		if err != nil {
			return res, fmt.Errorf("insert level %s: %w", r.StationID, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO data.river_level_current (station_id, observed_at, water_level_m)
			VALUES ($1, $2, $3)
			ON CONFLICT (station_id) DO UPDATE SET
				observed_at = EXCLUDED.observed_at, water_level_m = EXCLUDED.water_level_m, ingested_at = now()
			WHERE data.river_level_current.observed_at < EXCLUDED.observed_at`,
			r.StationID, r.ObservedAt, r.LevelM); err != nil {
			return res, fmt.Errorf("upsert current level %s: %w", r.StationID, err)
		}
		n, _ := h.RowsAffected()
		res.RowsInserted += int(n)
		res.ObserveTime(r.ObservedAt)
	}
	if future > 0 {
		res.Warnf("%d 筆水位的時間在未來，略過", future)
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		res.Warnf("%d 個水位站不在站況資料中：%s", len(unknown), strings.Join(unknown, "、"))
	}
	return res, tx.Commit()
}
