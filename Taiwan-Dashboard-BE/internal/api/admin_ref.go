package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/pipeline"
)

func bindJSON(c *gin.Context, v any) bool {
	dec := json.NewDecoder(io.LimitReader(c.Request.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_body", "內容格式錯誤："+err.Error())
		return false
	}
	return true
}

type reservoirRow struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	CountyCode        *string  `json:"county_code"`
	EffectiveCapacity *float64 `json:"effective_capacity"`
	CapacityLocked    bool     `json:"capacity_locked"`
	CapacityUpdatedOn *string  `json:"capacity_updated_on"`
	IsMajor           bool     `json:"is_major"`
	Lon               *float64 `json:"lon"`
	Lat               *float64 `json:"lat"`
	StoragePct        *float64 `json:"storage_pct"`
	ObservedAt        *string  `json:"observed_at"`
	HasStorage        bool     `json:"has_storage"`
}

func (h adminHandlers) reservoirs(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT r.reservoir_id, r.name, r.county_code, r.effective_capacity, r.capacity_locked,
		       to_char(r.capacity_updated_on, 'YYYY-MM-DD'), r.is_major, ST_X(r.geom), ST_Y(r.geom),
		       o.storage_pct, to_char(o.observed_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM-DD"T"HH24:MI:SS"+08:00"'),
		       coalesce(o.effective_storage IS NOT NULL, false)
		  FROM ref.reservoir r
		  LEFT JOIN data.reservoir_obs_current o USING (reservoir_id)
		 ORDER BY r.reservoir_id`)
	if err != nil {
		adminFail(c, err)
		return
	}
	defer rows.Close()
	out := []reservoirRow{}
	for rows.Next() {
		var r reservoirRow
		if err := rows.Scan(&r.ID, &r.Name, &r.CountyCode, &r.EffectiveCapacity, &r.CapacityLocked,
			&r.CapacityUpdatedOn, &r.IsMajor, &r.Lon, &r.Lat, &r.StoragePct, &r.ObservedAt, &r.HasStorage); err != nil {
			adminFail(c, err)
			return
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		adminFail(c, err)
		return
	}
	writeData(c, http.StatusOK, out, nil)
}

type reservoirInput struct {
	Name              string   `json:"name"`
	CountyCode        *string  `json:"county_code"`
	EffectiveCapacity *float64 `json:"effective_capacity"`
	CapacityLocked    bool     `json:"capacity_locked"`
	IsMajor           bool     `json:"is_major"`
	Lon               *float64 `json:"lon"`
	Lat               *float64 `json:"lat"`
}

func validPoint(lon, lat *float64) error {
	if (lon == nil) != (lat == nil) {
		return errors.New("經度與緯度要一起設定")
	}
	if lon == nil {
		return nil
	}
	if *lon < 114 || *lon > 123 || *lat < 10 || *lat > 27 {
		return errors.New("座標不在臺灣範圍內")
	}
	return nil
}

func (h adminHandlers) updateReservoir(c *gin.Context) {
	u, ok := mustAdmin(c)
	if !ok {
		return
	}
	id := c.Param("id")
	var in reservoirInput
	if !bindJSON(c, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Name == "" || len([]rune(in.Name)) > 50:
		writeError(c, http.StatusBadRequest, "invalid_name", "名稱必須是 1 到 50 個字")
		return
	case in.EffectiveCapacity != nil && (*in.EffectiveCapacity <= 0 || *in.EffectiveCapacity > 1e6):
		writeError(c, http.StatusBadRequest, "invalid_capacity", "有效容量必須大於 0，單位是萬立方公尺")
		return
	case in.CapacityLocked && in.EffectiveCapacity == nil:
		writeError(c, http.StatusBadRequest, "invalid_capacity", "鎖定有效容量時必須填寫數值")
		return
	}
	if err := validPoint(in.Lon, in.Lat); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_point", err.Error())
		return
	}
	ctx := c.Request.Context()
	if in.CountyCode != nil {
		if ok, err := h.countyExists(ctx, *in.CountyCode); err != nil {
			adminFail(c, err)
			return
		} else if !ok {
			writeError(c, http.StatusBadRequest, "invalid_county", "縣市代碼不存在")
			return
		}
	}

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		adminFail(c, err)
		return
	}
	defer tx.Rollback()
	var before json.RawMessage
	err = tx.QueryRowContext(ctx, `
		SELECT json_build_object('name', name, 'county_code', county_code, 'effective_capacity', effective_capacity,
		                         'capacity_locked', capacity_locked, 'is_major', is_major,
		                         'lon', ST_X(geom), 'lat', ST_Y(geom))
		  FROM ref.reservoir WHERE reservoir_id = $1 FOR UPDATE`, id).Scan(&before)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "not_found", "找不到這座水庫")
		return
	}
	if err != nil {
		adminFail(c, err)
		return
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE ref.reservoir
		   SET name = $2, county_code = $3, effective_capacity = $4, capacity_locked = $5, is_major = $6,
		       capacity_updated_on = CASE WHEN effective_capacity IS DISTINCT FROM $4 THEN current_date ELSE capacity_updated_on END,
		       geom = CASE WHEN $7::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_Point($7, $8), 4326) END,
		       updated_at = now()
		 WHERE reservoir_id = $1`,
		id, in.Name, in.CountyCode, in.EffectiveCapacity, in.CapacityLocked, in.IsMajor, in.Lon, in.Lat); err != nil {
		adminFail(c, err)
		return
	}
	if err := h.auditTx(ctx, tx, u, c, "ref.reservoir.update", "reservoir:"+id, before, in); err != nil {
		adminFail(c, err)
		return
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE data.reservoir_obs_current o
		   SET storage_pct = CASE WHEN r.effective_capacity > 0 AND o.effective_storage IS NOT NULL
		                          THEN round(o.effective_storage / r.effective_capacity * 100, 2) END
		  FROM ref.reservoir r
		 WHERE r.reservoir_id = o.reservoir_id AND o.reservoir_id = $1
		   AND o.storage_pct IS DISTINCT FROM CASE WHEN r.effective_capacity > 0 AND o.effective_storage IS NOT NULL
		                          THEN round(o.effective_storage / r.effective_capacity * 100, 2) END`, id); err != nil {
		adminFail(c, err)
		return
	}
	if err := notifyDatasets(ctx, tx, "reservoir_ref", "reservoir_obs"); err != nil {
		adminFail(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		adminFail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type powerUnitRow struct {
	ID         int      `json:"id"`
	UnitName   string   `json:"unit_name"`
	PlantName  *string  `json:"plant_name"`
	FuelType   string   `json:"fuel_type"`
	IsPrivate  bool     `json:"is_private"`
	CountyCode *string  `json:"county_code"`
	NetGenMW   *float64 `json:"net_gen_mw"`
}

func (h adminHandlers) powerUnits(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT u.unit_id, u.unit_name, u.plant_name, u.fuel_type, u.is_private, u.county_code, g.net_gen_mw
		  FROM ref.power_unit u
		  LEFT JOIN LATERAL (
		      SELECT net_gen_mw FROM data.power_gen_history h
		       WHERE h.unit_id = u.unit_id ORDER BY observed_at DESC LIMIT 1) g ON true
		 ORDER BY u.fuel_type, u.plant_name NULLS LAST, u.unit_name`)
	if err != nil {
		adminFail(c, err)
		return
	}
	defer rows.Close()
	out := []powerUnitRow{}
	for rows.Next() {
		var r powerUnitRow
		if err := rows.Scan(&r.ID, &r.UnitName, &r.PlantName, &r.FuelType, &r.IsPrivate, &r.CountyCode, &r.NetGenMW); err != nil {
			adminFail(c, err)
			return
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		adminFail(c, err)
		return
	}
	writeData(c, http.StatusOK, out, nil)
}

func (h adminHandlers) updatePowerUnit(c *gin.Context) {
	u, ok := mustAdmin(c)
	if !ok {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusNotFound, "not_found", "找不到這部機組")
		return
	}
	var in struct {
		CountyCode *string `json:"county_code"`
	}
	if !bindJSON(c, &in) {
		return
	}
	ctx := c.Request.Context()
	if in.CountyCode != nil {
		if ok, err := h.countyExists(ctx, *in.CountyCode); err != nil {
			adminFail(c, err)
			return
		} else if !ok {
			writeError(c, http.StatusBadRequest, "invalid_county", "縣市代碼不存在")
			return
		}
	}
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		adminFail(c, err)
		return
	}
	defer tx.Rollback()
	var before json.RawMessage
	err = tx.QueryRowContext(ctx, `
		SELECT json_build_object('county_code', county_code) FROM ref.power_unit WHERE unit_id = $1 FOR UPDATE`, id).Scan(&before)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "not_found", "找不到這部機組")
		return
	}
	if err != nil {
		adminFail(c, err)
		return
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ref.power_unit SET county_code = $2, updated_at = now() WHERE unit_id = $1`,
		id, in.CountyCode); err != nil {
		adminFail(c, err)
		return
	}
	if err := h.auditTx(ctx, tx, u, c, "ref.power_unit.update", fmt.Sprintf("power_unit:%d", id), before, in); err != nil {
		adminFail(c, err)
		return
	}
	if err := notifyDatasets(ctx, tx, "power_gen"); err != nil {
		adminFail(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		adminFail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h adminHandlers) runDataset(c *gin.Context) {
	u, ok := mustAdmin(c)
	if !ok {
		return
	}
	if h.run == nil {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "這台伺服器連到其他主機的資料庫，不負責抓資料，請在資料庫所在的主機上執行")
		return
	}
	id := c.Param("id")
	res, err := h.run(c.Request.Context(), id)
	switch {
	case errors.Is(err, ErrUnknownDataset):
		writeError(c, http.StatusNotFound, "not_found", "找不到這個資料集")
		return
	case errors.Is(err, pipeline.ErrAlreadyRunning):
		writeError(c, http.StatusConflict, "already_running", "這個資料集正在抓取中，請稍後再試")
		return
	}
	detail := map[string]any{"rows_fetched": res.RowsFetched, "rows_inserted": res.RowsInserted}
	if err != nil {
		detail["error"] = err.Error()
	}
	if aerr := h.store.Audit(context.WithoutCancel(c.Request.Context()), &u.ID, "dataset.run", "dataset:"+id, detail, c.ClientIP()); aerr != nil {
		slog.Error("audit dataset run", "error", aerr)
	}
	if err != nil {
		writeError(c, http.StatusBadGateway, "run_failed", "抓取失敗："+pipeline.Truncate(err.Error(), 300))
		return
	}
	writeData(c, http.StatusOK, gin.H{
		"rows_fetched":     res.RowsFetched,
		"rows_inserted":    res.RowsInserted,
		"warnings":         nonNilStrings(res.Warnings),
		"last_observed_at": res.LastObservedAt,
	}, nil)
}

var ErrUnknownDataset = errors.New("unknown dataset")

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func notifyDatasets(ctx context.Context, tx *sql.Tx, ids ...string) error {
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `SELECT pg_notify($1, $2)`, pipeline.DatasetChannel, id); err != nil {
			return err
		}
	}
	return nil
}

func (h adminHandlers) countyExists(ctx context.Context, code string) (bool, error) {
	var ok bool
	err := h.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM ref.county WHERE code = $1)`, code).Scan(&ok)
	return ok, err
}

func (h adminHandlers) auditTx(ctx context.Context, tx *sql.Tx, u *auth.User, c *gin.Context, action, target string, before json.RawMessage, after any) error {
	a, err := json.Marshal(after)
	if err != nil {
		return err
	}
	return auth.AuditTx(ctx, tx, &u.ID, action, target, map[string]any{
		"before": before, "after": json.RawMessage(a),
	}, c.ClientIP())
}
