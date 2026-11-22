package component

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalid  = errors.New("invalid component")
	ErrNotFound = errors.New("component not found")
	ErrExists   = errors.New("component already exists")
	ErrConflict = errors.New("component changed since it was loaded")
)

const PreviewRowLimit = 50

type Check struct {
	Label     string   `json:"label"` // national chart_sql、county history_sql、map layer 1…
	Scope     string   `json:"scope,omitempty"`
	Kind      string   `json:"kind"` // chart、history、layer
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	RowCount  int      `json:"row_count"`
	ElapsedMS int64    `json:"elapsed_ms"`
	Data      any      `json:"data,omitempty"`
	Error     string   `json:"error,omitempty"`
}

func Preview(ctx context.Context, db *sql.DB, run QueryFunc, c ComponentSpec, county string) ([]Check, error) {
	known, err := knownDatasets(ctx, db)
	if err != nil {
		return nil, err
	}
	if err := validateComponent(c, known); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	now := time.Now()
	from := now.Add(-Ranges[c.TimeRange])
	step := StepFor(from, now)
	exec := func(ck Check, query string, p Params, build func(Rows) (any, error)) Check {
		ck.Columns, ck.Rows = []string{}, [][]any{}
		compiled, err := Compile(query)
		if err != nil {
			ck.Error = err.Error()
			return ck
		}
		if ck.Scope == "county" && !compiled.Uses("county") {
			ck.Error = "縣市查詢必須用 :county 篩選"
			return ck
		}
		started := time.Now()
		rows, err := run(ctx, compiled, p)
		ck.ElapsedMS = time.Since(started).Milliseconds()
		if err != nil {
			ck.Error = err.Error()
			return ck
		}
		ck.Columns, ck.RowCount = rows.Columns, len(rows.Values)
		ck.Rows = rows.Values
		if len(ck.Rows) > PreviewRowLimit {
			ck.Rows = ck.Rows[:PreviewRowLimit]
		}
		if ck.Rows == nil {
			ck.Rows = [][]any{}
		}
		data, err := build(rows)
		if err != nil {
			ck.Error = err.Error()
			return ck
		}
		ck.Data = data
		return ck
	}
	shaped := func(shape string) func(Rows) (any, error) {
		return func(r Rows) (any, error) { return Build(shape, r) }
	}

	var out []Check
	for _, q := range c.Queries {
		p := Params{From: from, To: now, Step: step}
		if q.Scope == "county" {
			p.County = &county
		}
		out = append(out, exec(Check{Label: q.Scope + " chart_sql", Scope: q.Scope, Kind: "chart"},
			q.ChartSQL, p, shaped(q.DataShape)))
		if strings.TrimSpace(q.HistorySQL) != "" {
			out = append(out, exec(Check{Label: q.Scope + " history_sql", Scope: q.Scope, Kind: "history"},
				q.HistorySQL, p, shaped(ShapeTime)))
		}
	}
	for i, l := range c.MapLayers {
		if l.SourceType != "query" {
			continue
		}
		p := Params{County: &county, From: from, To: now, Step: step}
		out = append(out, exec(Check{Label: fmt.Sprintf("map layer %d geojson_sql", i+1), Kind: "layer"},
			l.GeoJSONSQL, p, func(r Rows) (any, error) {
				b, err := BuildGeoJSON(r)
				return json.RawMessage(b), err
			}))
	}
	return out, nil
}

type SaveOptions struct {
	Note        string
	By          *int64
	Create      bool
	BaseVersion *int64
}

func Save(ctx context.Context, db *sql.DB, run QueryFunc, c ComponentSpec, o SaveOptions) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var latest sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		SELECT (SELECT max(v.id) FROM app.component_version v WHERE v.component_id = c.id)
		  FROM app.component c WHERE c.slug = $1 FOR UPDATE`, c.Slug).Scan(&latest)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	switch {
	case o.Create && exists:
		return 0, ErrExists
	case !o.Create && !exists:
		return 0, ErrNotFound
	case o.BaseVersion != nil && latest.Valid && latest.Int64 != *o.BaseVersion:
		return 0, ErrConflict
	}

	known, err := knownDatasets(ctx, tx)
	if err != nil {
		return 0, err
	}
	if err := validateComponent(c, known); err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err := trialRun(ctx, run, c); err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	version, err := writeComponent(ctx, tx, c, o.Note, o.By)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_notify($1, $2)`, ConfigChannel, "component:"+c.Slug); err != nil {
		return 0, err
	}
	return version, tx.Commit()
}

type Summary struct {
	Slug       string    `json:"slug"`
	Name       string    `json:"name"`
	Topic      string    `json:"topic"`
	Status     string    `json:"status"`
	UpdatedAt  time.Time `json:"updated_at"`
	Versions   int       `json:"versions"`
	Dashboards []string  `json:"dashboards"`
}

func List(ctx context.Context, db *sql.DB) ([]Summary, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.slug, c.name, c.topic, c.status, c.updated_at,
		       (SELECT count(*) FROM app.component_version v WHERE v.component_id = c.id),
		       coalesce((SELECT array_agg(d.name ORDER BY d.sort_order)
		                   FROM app.dashboard_component dc JOIN app.dashboard d ON d.id = dc.dashboard_id
		                  WHERE dc.component_id = c.id), '{}')
		  FROM app.component c
		 ORDER BY c.topic, c.slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pgMap := pgtype.NewMap()
	out := []Summary{}
	for rows.Next() {
		var s Summary
		if err := rows.Scan(&s.Slug, &s.Name, &s.Topic, &s.Status, &s.UpdatedAt, &s.Versions,
			pgMap.SQLScanner(&s.Dashboards)); err != nil {
			return nil, err
		}
		s.Dashboards = nonNil(s.Dashboards)
		out = append(out, s)
	}
	return out, rows.Err()
}

func Latest(ctx context.Context, db *sql.DB, slug string) (ComponentSpec, int64, error) {
	var raw []byte
	var id int64
	err := db.QueryRowContext(ctx, `
		SELECT v.id, v.snapshot FROM app.component_version v
		  JOIN app.component c ON c.id = v.component_id
		 WHERE c.slug = $1 ORDER BY v.id DESC LIMIT 1`, slug).Scan(&id, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ComponentSpec{}, 0, ErrNotFound
	}
	if err != nil {
		return ComponentSpec{}, 0, err
	}
	spec, err := decodeSnapshot(raw)
	return spec, id, err
}

type Version struct {
	ID        int64     `json:"id"`
	Note      string    `json:"note"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy *string   `json:"created_by"`
}

func Versions(ctx context.Context, db *sql.DB, slug string) ([]Version, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT v.id, v.note, coalesce(v.snapshot->>'status', ''), v.created_at, u.display_name
		  FROM app.component_version v
		  JOIN app.component c ON c.id = v.component_id
		  LEFT JOIN app.users u ON u.id = v.created_by
		 WHERE c.slug = $1
		 ORDER BY v.id DESC
		 LIMIT 200`, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Version{}
	for rows.Next() {
		var v Version
		if err := rows.Scan(&v.ID, &v.Note, &v.Status, &v.CreatedAt, &v.CreatedBy); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM app.component WHERE slug = $1)`, slug).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrNotFound
		}
	}
	return out, nil
}

func VersionSpec(ctx context.Context, db *sql.DB, slug string, id int64) (ComponentSpec, error) {
	var raw []byte
	err := db.QueryRowContext(ctx, `
		SELECT v.snapshot FROM app.component_version v
		  JOIN app.component c ON c.id = v.component_id
		 WHERE c.slug = $1 AND v.id = $2`, slug, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ComponentSpec{}, ErrNotFound
	}
	if err != nil {
		return ComponentSpec{}, err
	}
	return decodeSnapshot(raw)
}

func decodeSnapshot(raw []byte) (ComponentSpec, error) {
	var spec ComponentSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return ComponentSpec{}, fmt.Errorf("decode snapshot: %w", err)
	}
	spec.DatasetIDs = nonNil(spec.DatasetIDs)
	spec.ChartTypes = nonNil(spec.ChartTypes)
	spec.Colors = nonNil(spec.Colors)
	spec.Links = nonNil(spec.Links)
	if spec.Queries == nil {
		spec.Queries = []QuerySpec{}
	}
	if spec.MapLayers == nil {
		spec.MapLayers = []MapLayerSpec{}
	}
	return spec, nil
}
