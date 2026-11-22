package component

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type Component struct {
	Slug         string            `json:"slug"`
	Name         string            `json:"name"`
	Topic        string            `json:"topic"`
	ShortDesc    string            `json:"short_desc"`
	LongDesc     string            `json:"long_desc"`
	UseCase      string            `json:"use_case"`
	DatasetIDs   []string          `json:"dataset_ids"`
	TimeRange    string            `json:"time_range"`
	ChartTypes   []string          `json:"chart_types"`
	Colors       []string          `json:"colors"`
	Unit         string            `json:"unit"`
	Decimals     int               `json:"decimals"`
	ChartMapLink json.RawMessage   `json:"chart_map_link"`
	Links        []string          `json:"links"`
	HasMapLayers bool              `json:"has_map_layers"`
	MapLayers    []*MapLayer       `json:"map_layers"`
	Queries      map[string]*Query `json:"-"` // by scope
}

type MapLayer struct {
	ID          int             `json:"id"`
	Title       string          `json:"title"`
	LayerType   string          `json:"layer_type"`
	SourceType  string          `json:"source_type"`
	Style       json.RawMessage `json:"style"`
	PopupFields json.RawMessage `json:"popup_fields"`
	GeoJSON     *Compiled       `json:"-"`
}

func (c *Component) Layer(id int) *MapLayer {
	for _, l := range c.MapLayers {
		if l.ID == id {
			return l
		}
	}
	return nil
}

type Query struct {
	Shape   string
	Chart   Compiled
	History *Compiled
}

func (c *Component) Scopes() []string {
	var s []string
	for _, scope := range []string{"national", "county"} {
		if c.Queries[scope] != nil {
			s = append(s, scope)
		}
	}
	return s
}

func (c *Component) HasHistory() bool {
	for _, q := range c.Queries {
		if q.History != nil {
			return true
		}
	}
	return false
}

type DashboardItem struct {
	Component string `json:"component"`
	Size      string `json:"size"`
}

type Dashboard struct {
	Slug        string          `json:"slug"`
	Name        string          `json:"name"`
	Icon        string          `json:"icon"`
	Description string          `json:"description"`
	Components  []DashboardItem `json:"components"`
}

type County struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	ShortName string  `json:"short_name"`
	NameEn    string  `json:"name_en"`
	Lon       float64 `json:"lon"`
	Lat       float64 `json:"lat"`
}

type Source struct {
	Dataset           string  `json:"dataset"`
	Name              string  `json:"name"`
	Agency            string  `json:"agency"`
	License           string  `json:"license"`
	URL               string  `json:"url"`
	Status            string  `json:"status"`
	LastObservedAt    *string `json:"last_observed_at"`
	UpdateFreqSeconds int64   `json:"update_freq_seconds"`
	LastIngestedAt    *string `json:"last_ingested_at"`
	StaleAfterSeconds int64   `json:"stale_after_seconds"`
}

type Catalog struct {
	Components map[string]*Component
	Dashboards []*Dashboard
	Counties   []County
	UpdateFreq map[string]time.Duration
	countyCode map[string]bool
}

func (c *Catalog) IsCounty(code string) bool { return c.countyCode[code] }

func (c *Catalog) Dashboard(slug string) *Dashboard {
	for _, d := range c.Dashboards {
		if d.Slug == slug {
			return d
		}
	}
	return nil
}

func LoadCatalog(ctx context.Context, db *sql.DB) (*Catalog, error) {
	cat := &Catalog{Components: map[string]*Component{}, UpdateFreq: map[string]time.Duration{}, countyCode: map[string]bool{}}
	pgMap := pgtype.NewMap() // scans arrays through database/sql; not safe to share

	rows, err := db.QueryContext(ctx, `
		SELECT c.id, c.slug, c.name, c.topic, c.short_desc, c.long_desc, c.use_case, c.dataset_ids,
		       c.time_range, c.chart_types, c.colors, c.unit, c.decimals, c.chart_map_link, c.links,
		       EXISTS (SELECT 1 FROM app.component_map_layer l WHERE l.component_id = c.id)
		  FROM app.component c
		 WHERE c.status = 'published'`)
	if err != nil {
		return nil, fmt.Errorf("load components: %w", err)
	}
	byID := map[int]*Component{}
	for rows.Next() {
		var id int
		var c Component
		var link []byte
		if err := rows.Scan(&id, &c.Slug, &c.Name, &c.Topic, &c.ShortDesc, &c.LongDesc, &c.UseCase,
			pgMap.SQLScanner(&c.DatasetIDs), &c.TimeRange, pgMap.SQLScanner(&c.ChartTypes),
			pgMap.SQLScanner(&c.Colors), &c.Unit, &c.Decimals, &link, pgMap.SQLScanner(&c.Links),
			&c.HasMapLayers); err != nil {
			rows.Close()
			return nil, err
		}
		if link != nil {
			c.ChartMapLink = link
		}
		c.Queries = map[string]*Query{}
		c.MapLayers = []*MapLayer{}
		byID[id] = &c
		cat.Components[c.Slug] = &c
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, `SELECT component_id, scope, data_shape, chart_sql, history_sql FROM app.component_query`)
	if err != nil {
		return nil, fmt.Errorf("load component queries: %w", err)
	}
	for rows.Next() {
		var id int
		var scope, shape, chartSQL string
		var historySQL sql.NullString
		if err := rows.Scan(&id, &scope, &shape, &chartSQL, &historySQL); err != nil {
			rows.Close()
			return nil, err
		}
		c := byID[id]
		if c == nil {
			continue // a draft
		}
		q, err := compileQuery(shape, chartSQL, historySQL)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("component %s (%s): %w", c.Slug, scope, err)
		}
		c.Queries[scope] = q
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, `
		SELECT id, component_id, title, layer_type, source_type, style, popup_fields, geojson_sql
		  FROM app.component_map_layer ORDER BY component_id, sort_order, id`)
	if err != nil {
		return nil, fmt.Errorf("load map layers: %w", err)
	}
	for rows.Next() {
		var l MapLayer
		var componentID int
		var style, popup []byte
		var geoSQL sql.NullString
		if err := rows.Scan(&l.ID, &componentID, &l.Title, &l.LayerType, &l.SourceType, &style, &popup, &geoSQL); err != nil {
			rows.Close()
			return nil, err
		}
		c := byID[componentID]
		if c == nil {
			continue // a draft
		}
		l.Style, l.PopupFields = style, popup
		if geoSQL.Valid {
			q, err := Compile(geoSQL.String)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("component %s layer %d: %w", c.Slug, l.ID, err)
			}
			l.GeoJSON = &q
		}
		c.MapLayers = append(c.MapLayers, &l)
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, `
		SELECT d.slug, d.name, d.icon, d.description, c.slug, dc.size
		  FROM app.dashboard d
		  LEFT JOIN app.dashboard_component dc ON dc.dashboard_id = d.id
		  LEFT JOIN app.component c ON c.id = dc.component_id AND c.status = 'published'
		 ORDER BY d.sort_order, d.id, dc.position`)
	if err != nil {
		return nil, fmt.Errorf("load dashboards: %w", err)
	}
	for rows.Next() {
		var d Dashboard
		var comp, size sql.NullString
		if err := rows.Scan(&d.Slug, &d.Name, &d.Icon, &d.Description, &comp, &size); err != nil {
			rows.Close()
			return nil, err
		}
		last := len(cat.Dashboards) - 1
		if last < 0 || cat.Dashboards[last].Slug != d.Slug {
			d.Components = []DashboardItem{}
			cat.Dashboards = append(cat.Dashboards, &d)
			last++
		}
		if comp.Valid {
			cat.Dashboards[last].Components = append(cat.Dashboards[last].Components,
				DashboardItem{Component: comp.String, Size: size.String})
		}
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, `
		SELECT code, name, short_name, name_en, ST_X(center), ST_Y(center)
		  FROM ref.county ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("load counties: %w", err)
	}
	for rows.Next() {
		var c County
		if err := rows.Scan(&c.Code, &c.Name, &c.ShortName, &c.NameEn, &c.Lon, &c.Lat); err != nil {
			rows.Close()
			return nil, err
		}
		cat.Counties = append(cat.Counties, c)
		cat.countyCode[c.Code] = true
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, `SELECT id, extract(epoch FROM update_freq)::bigint FROM app.dataset`)
	if err != nil {
		return nil, fmt.Errorf("load datasets: %w", err)
	}
	for rows.Next() {
		var id string
		var seconds int64
		if err := rows.Scan(&id, &seconds); err != nil {
			rows.Close()
			return nil, err
		}
		cat.UpdateFreq[id] = time.Duration(seconds) * time.Second
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}
	return cat, nil
}

func DatasetStatusSQL(a string) string {
	return fmt.Sprintf(`CASE WHEN %[1]s.status = 'ok' AND %[1]s.last_observed_at < now() - %[1]s.stale_after
	THEN 'stale' ELSE %[1]s.status END`, a)
}

func LoadSources(ctx context.Context, db *sql.DB, ids []string) ([]Source, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, source_agency, license, source_url, `+DatasetStatusSQL("app.dataset")+`,
		       to_char(last_observed_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM-DD"T"HH24:MI:SS"+08:00"'),
		       extract(epoch FROM update_freq)::bigint,
		       to_char(last_ingested_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM-DD"T"HH24:MI:SS"+08:00"'),
		       extract(epoch FROM stale_after)::bigint
		  FROM app.dataset WHERE id = ANY($1) ORDER BY array_position($1, id)`, ids)
	if err != nil {
		return nil, fmt.Errorf("load dataset sources: %w", err)
	}
	defer rows.Close()
	out := []Source{}
	for rows.Next() {
		var s Source
		if err := rows.Scan(&s.Dataset, &s.Name, &s.Agency, &s.License, &s.URL, &s.Status, &s.LastObservedAt, &s.UpdateFreqSeconds,
			&s.LastIngestedAt, &s.StaleAfterSeconds); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func CountiesGeoJSON(ctx context.Context, db *sql.DB) ([]byte, error) {
	var b []byte
	err := db.QueryRowContext(ctx, `
		SELECT json_build_object(
		         'type', 'FeatureCollection',
		         'features', coalesce(json_agg(json_build_object(
		             'type', 'Feature',
		             'id', code,
		             'geometry', ST_AsGeoJSON(boundary, 5)::json,
		             'properties', json_build_object('code', code, 'name', name, 'short_name', short_name)
		           ) ORDER BY sort_order), '[]'::json))::text
		  FROM ref.county`).Scan(&b)
	return b, err
}

func compileQuery(shape, chartSQL string, historySQL sql.NullString) (*Query, error) {
	if !ValidShape(shape) {
		return nil, fmt.Errorf("unknown data shape %q", shape)
	}
	chart, err := Compile(chartSQL)
	if err != nil {
		return nil, fmt.Errorf("chart_sql: %w", err)
	}
	q := &Query{Shape: shape, Chart: chart}
	if historySQL.Valid && strings.TrimSpace(historySQL.String) != "" {
		h, err := Compile(historySQL.String)
		if err != nil {
			return nil, fmt.Errorf("history_sql: %w", err)
		}
		q.History = &h
	}
	return q, nil
}

func closeRows(rows *sql.Rows) error {
	err := rows.Err()
	if cerr := rows.Close(); err == nil {
		err = cerr
	}
	return err
}
