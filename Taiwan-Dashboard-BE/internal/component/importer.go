package component

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Bundle struct {
	Components []ComponentSpec `json:"components"`
	Dashboards []DashboardSpec `json:"dashboards"`
}

type ComponentSpec struct {
	Slug         string          `json:"slug"`
	Name         string          `json:"name"`
	Topic        string          `json:"topic"`
	ShortDesc    string          `json:"short_desc"`
	LongDesc     string          `json:"long_desc"`
	UseCase      string          `json:"use_case"`
	DatasetIDs   []string        `json:"dataset_ids"`
	TimeRange    string          `json:"time_range"`
	ChartTypes   []string        `json:"chart_types"`
	Colors       []string        `json:"colors"`
	Unit         string          `json:"unit"`
	Decimals     int             `json:"decimals"`
	ChartMapLink json.RawMessage `json:"chart_map_link,omitempty"`
	Links        []string        `json:"links"`
	Status       string          `json:"status"`
	Queries      []QuerySpec     `json:"queries"`
	MapLayers    []MapLayerSpec  `json:"map_layers"`
}

type QuerySpec struct {
	Scope      string `json:"scope"`
	DataShape  string `json:"data_shape"`
	ChartSQL   string `json:"chart_sql"`
	HistorySQL string `json:"history_sql,omitempty"`
}

type MapLayerSpec struct {
	Title       string          `json:"title"`
	LayerType   string          `json:"layer_type"`
	SourceType  string          `json:"source_type"`
	GeoJSONSQL  string          `json:"geojson_sql,omitempty"`
	Style       json.RawMessage `json:"style,omitempty"`
	PopupFields json.RawMessage `json:"popup_fields,omitempty"`
}

type DashboardSpec struct {
	Slug        string          `json:"slug"`
	Name        string          `json:"name"`
	Icon        string          `json:"icon"`
	Description string          `json:"description"`
	Components  []DashboardItem `json:"components"`
}

func ParseBundle(b []byte) (Bundle, error) {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	var out Bundle
	if err := dec.Decode(&out); err != nil {
		return Bundle{}, fmt.Errorf("parse bundle: %w", err)
	}
	return out, nil
}

type ImportResult struct {
	Components int
	Dashboards int
}

const SampleCounty = "63000"

const ConfigChannel = "config_updated"

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func Import(ctx context.Context, db *sql.DB, b Bundle, note string, dryRun bool) (ImportResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ImportResult{}, err
	}
	defer tx.Rollback()

	known, err := knownDatasets(ctx, tx)
	if err != nil {
		return ImportResult{}, err
	}
	slugs := map[string]bool{}
	for _, c := range b.Components {
		if err := validateComponent(c, known); err != nil {
			return ImportResult{}, fmt.Errorf("component %q: %w", c.Slug, err)
		}
		if slugs[c.Slug] {
			return ImportResult{}, fmt.Errorf("component %q appears twice", c.Slug)
		}
		slugs[c.Slug] = true
		if err := trialRun(ctx, txReader(tx), c); err != nil {
			return ImportResult{}, fmt.Errorf("component %q: %w", c.Slug, err)
		}
		if _, err := writeComponent(ctx, tx, c, note, nil); err != nil {
			return ImportResult{}, fmt.Errorf("component %q: %w", c.Slug, err)
		}
	}
	for i, d := range b.Dashboards {
		if err := writeDashboard(ctx, tx, d, i); err != nil {
			return ImportResult{}, fmt.Errorf("dashboard %q: %w", d.Slug, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_notify($1, 'import')`, ConfigChannel); err != nil {
		return ImportResult{}, err
	}
	res := ImportResult{Components: len(b.Components), Dashboards: len(b.Dashboards)}
	if dryRun {
		return res, nil
	}
	return res, tx.Commit()
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func knownDatasets(ctx context.Context, tx queryer) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM app.dataset`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		known[id] = true
	}
	return known, rows.Err()
}

var topics = map[string]bool{"aqi": true, "reservoir": true, "power": true, "outage": true, "earthquake": true, "weather": true, "warning": true, "traffic": true, "resources": true, "population": true, "tourism": true,
	"rail": true, "bike": true, "safety": true,
	"family": true, "sustain": true,
	"agri": true, "food": true, "fire": true,
	"housing": true,
	"disease": true, "aviation": true, "economy": true, "animal": true}

func validateComponent(c ComponentSpec, known map[string]bool) error {
	var errs []error
	if !slugPattern.MatchString(c.Slug) {
		errs = append(errs, errors.New("slug must be lowercase letters, digits and single hyphens"))
	}
	if strings.TrimSpace(c.Name) == "" {
		errs = append(errs, errors.New("name is required"))
	}
	if !topics[c.Topic] {
		errs = append(errs, fmt.Errorf("unknown topic %q", c.Topic))
	}
	if _, ok := Ranges[c.TimeRange]; !ok {
		errs = append(errs, fmt.Errorf("unknown time_range %q", c.TimeRange))
	}
	if len(c.ChartTypes) == 0 {
		errs = append(errs, errors.New("chart_types needs at least one chart type"))
	}
	if c.Status != "draft" && c.Status != "published" {
		errs = append(errs, fmt.Errorf("status must be draft or published, got %q", c.Status))
	}
	if len(c.DatasetIDs) == 0 {
		errs = append(errs, errors.New("dataset_ids is required: it drives attribution and cache invalidation"))
	}
	for _, id := range c.DatasetIDs {
		if !known[id] {
			errs = append(errs, fmt.Errorf("unknown dataset %q", id))
		}
	}
	scopes := map[string]bool{}
	for _, q := range c.Queries {
		if q.Scope != "national" && q.Scope != "county" {
			errs = append(errs, fmt.Errorf("unknown scope %q", q.Scope))
		}
		if scopes[q.Scope] {
			errs = append(errs, fmt.Errorf("scope %q appears twice", q.Scope))
		}
		scopes[q.Scope] = true
		if !ValidShape(q.DataShape) {
			errs = append(errs, fmt.Errorf("unknown data_shape %q", q.DataShape))
		}
	}
	if !scopes["national"] {
		errs = append(errs, errors.New("a national query is required"))
	}
	return errors.Join(errs...)
}

type QueryFunc func(ctx context.Context, q Compiled, p Params) (Rows, error)

func txReader(tx *sql.Tx) QueryFunc {
	return func(ctx context.Context, q Compiled, p Params) (Rows, error) {
		return runAsReader(ctx, tx, q, p)
	}
}

func trialRun(ctx context.Context, run QueryFunc, c ComponentSpec) error {
	now := time.Now()
	from := now.Add(-Ranges[c.TimeRange])
	for _, q := range c.Queries {
		params := Params{From: from, To: now, Step: StepFor(from, now)}
		if q.Scope == "county" {
			county := SampleCounty
			params.County = &county
		}
		check := func(label, query, shape string) error {
			compiled, err := Compile(query)
			if err != nil {
				return fmt.Errorf("%s %s: %w", q.Scope, label, err)
			}
			if q.Scope == "county" && !compiled.Uses("county") {
				return fmt.Errorf("%s %s must filter by :county", q.Scope, label)
			}
			rows, err := run(ctx, compiled, params)
			if err != nil {
				return fmt.Errorf("%s %s: %w", q.Scope, label, err)
			}
			if _, err := Build(shape, rows); err != nil {
				return fmt.Errorf("%s %s: %w", q.Scope, label, err)
			}
			return nil
		}
		if err := check("chart_sql", q.ChartSQL, q.DataShape); err != nil {
			return err
		}
		if strings.TrimSpace(q.HistorySQL) != "" {
			if err := check("history_sql", q.HistorySQL, ShapeTime); err != nil {
				return err
			}
		}
	}
	for i, l := range c.MapLayers {
		if l.SourceType != "query" {
			continue
		}
		compiled, err := Compile(l.GeoJSONSQL)
		if err != nil {
			return fmt.Errorf("map layer %d geojson_sql: %w", i+1, err)
		}
		county := SampleCounty
		rows, err := run(ctx, compiled, Params{County: &county, From: from, To: now, Step: StepFor(from, now)})
		if err != nil {
			return fmt.Errorf("map layer %d geojson_sql: %w", i+1, err)
		}
		if _, err := BuildGeoJSON(rows); err != nil {
			return fmt.Errorf("map layer %d geojson_sql: %w", i+1, err)
		}
	}
	return nil
}

func runAsReader(ctx context.Context, tx *sql.Tx, q Compiled, p Params) (Rows, error) {
	if _, err := tx.ExecContext(ctx, `SAVEPOINT trial`); err != nil {
		return Rows{}, err
	}
	defer tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT trial`) //nolint:errcheck // undoes the role switch
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE dashboard_reader`); err != nil {
		return Rows{}, err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", DefaultTimeout.Milliseconds())); err != nil {
		return Rows{}, err
	}
	return queryAll(ctx, tx, q, p)
}

func writeComponent(ctx context.Context, tx *sql.Tx, c ComponentSpec, note string, by *int64) (int64, error) {
	var id int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO app.component (slug, name, topic, short_desc, long_desc, use_case, dataset_ids,
		                           time_range, chart_types, colors, unit, decimals, chart_map_link, links, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (slug) DO UPDATE SET
			name = EXCLUDED.name, topic = EXCLUDED.topic, short_desc = EXCLUDED.short_desc,
			long_desc = EXCLUDED.long_desc, use_case = EXCLUDED.use_case, dataset_ids = EXCLUDED.dataset_ids,
			time_range = EXCLUDED.time_range, chart_types = EXCLUDED.chart_types, colors = EXCLUDED.colors,
			unit = EXCLUDED.unit, decimals = EXCLUDED.decimals, chart_map_link = EXCLUDED.chart_map_link,
			links = EXCLUDED.links, status = EXCLUDED.status, updated_at = now()
		RETURNING id`,
		c.Slug, c.Name, c.Topic, c.ShortDesc, c.LongDesc, c.UseCase, nonNil(c.DatasetIDs),
		c.TimeRange, nonNil(c.ChartTypes), nonNil(c.Colors), c.Unit, c.Decimals, rawOrNil(c.ChartMapLink),
		nonNil(c.Links), c.Status).Scan(&id)
	if err != nil {
		return 0, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM app.component_query WHERE component_id = $1`, id); err != nil {
		return 0, err
	}
	for _, q := range c.Queries {
		var history any
		if strings.TrimSpace(q.HistorySQL) != "" {
			history = q.HistorySQL
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app.component_query (component_id, scope, data_shape, chart_sql, history_sql)
			VALUES ($1, $2, $3, $4, $5)`, id, q.Scope, q.DataShape, q.ChartSQL, history); err != nil {
			return 0, err
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM app.component_map_layer WHERE component_id = $1`, id); err != nil {
		return 0, err
	}
	for i, l := range c.MapLayers {
		var geo any
		if l.GeoJSONSQL != "" {
			geo = l.GeoJSONSQL
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app.component_map_layer (component_id, sort_order, title, layer_type, source_type,
			                                     geojson_sql, style, popup_fields)
			VALUES ($1, $2, $3, $4, $5, $6, coalesce($7::jsonb, '{}'), coalesce($8::jsonb, '[]'))`,
			id, i, l.Title, l.LayerType, l.SourceType, geo, rawOrNil(l.Style), rawOrNil(l.PopupFields)); err != nil {
			return 0, err
		}
	}

	snapshot, err := json.Marshal(c)
	if err != nil {
		return 0, err
	}
	var version int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO app.component_version (component_id, snapshot, note, created_by)
		VALUES ($1, $2::jsonb, $3, $4) RETURNING id`,
		id, string(snapshot), note, by).Scan(&version)
	return version, err
}

func writeDashboard(ctx context.Context, tx *sql.Tx, d DashboardSpec, order int) error {
	if !slugPattern.MatchString(d.Slug) {
		return errors.New("slug must be lowercase letters, digits and single hyphens")
	}
	var id int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO app.dashboard (slug, name, icon, description, sort_order)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (slug) DO UPDATE SET
			name = EXCLUDED.name, icon = EXCLUDED.icon, description = EXCLUDED.description,
			sort_order = EXCLUDED.sort_order
		RETURNING id`, d.Slug, d.Name, d.Icon, d.Description, order).Scan(&id)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app.dashboard_component WHERE dashboard_id = $1`, id); err != nil {
		return err
	}
	for i, item := range d.Components {
		size := item.Size
		if size == "" {
			size = "normal"
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO app.dashboard_component (dashboard_id, component_id, position, size)
			SELECT $1, id, $3, $4 FROM app.component WHERE slug = $2`, id, item.Component, i, size)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("unknown component %q", item.Component)
		}
	}
	return nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func rawOrNil(r json.RawMessage) any {
	if len(r) == 0 || string(r) == "null" {
		return nil
	}
	return string(r)
}
