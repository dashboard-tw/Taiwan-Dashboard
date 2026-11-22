package screen

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"dashboard.tw/backend/internal/cache"
	"dashboard.tw/backend/internal/component"
	"dashboard.tw/backend/internal/pipeline"
)

type Querier interface {
	Query(ctx context.Context, q component.Compiled, p component.Params) (component.Rows, error)
}

const configTag = "config"

const (
	catalogTTL = 10 * time.Minute
	geoTTL     = time.Hour
	minDataTTL = time.Minute
	maxDataTTL = 30 * time.Minute
)

type Service struct {
	App    *sql.DB // application pool: app, ref and data schemas
	Reader Querier
	Now    func() time.Time

	catalog  *cache.Cache[*component.Catalog]
	data     *cache.Cache[*Result]
	geo      *cache.Cache[[]byte]
	sources  *cache.Cache[[]component.Source]
	layers   *cache.Cache[[]byte]
	typhoons *cache.Cache[[]Typhoon]
	quakes   *cache.Cache[[]Quake]
	quake    *cache.Cache[*QuakeDetail]
}

func New(app *sql.DB, reader Querier) *Service {
	return &Service{
		App:     app,
		Reader:  reader,
		Now:     time.Now,
		catalog: cache.New[*component.Catalog](1),
		data:    cache.New[*Result](5000),
		geo:     cache.New[[]byte](1),
		sources: cache.New[[]component.Source](500),
		layers:  cache.New[[]byte](500),

		typhoons: cache.New[[]Typhoon](1),
		quakes:   cache.New[[]Quake](1),
		quake:    cache.New[*QuakeDetail](200),
	}
}

func (s *Service) DatasetUpdated(id string) {
	s.data.Invalidate(id)
	s.sources.Invalidate(id)
	s.layers.Invalidate(id)
	s.typhoons.Invalidate(id)
	s.quakes.Invalidate(id)
	s.quake.Invalidate(id)
}

func (s *Service) ConfigUpdated() {
	s.catalog.InvalidateAll()
	s.geo.InvalidateAll()
	s.data.InvalidateAll()
	s.sources.InvalidateAll()
	s.layers.InvalidateAll()
}

func (s *Service) Reset() { s.ConfigUpdated() }

type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

func notFound(code, msg string) *Error   { return &Error{Status: 404, Code: code, Message: msg} }
func badRequest(code, msg string) *Error { return &Error{Status: 400, Code: code, Message: msg} }

func (s *Service) Catalog(ctx context.Context) (*component.Catalog, bool, error) {
	cat, info, err := s.catalog.Get(ctx, "catalog", []string{configTag}, catalogTTL,
		func(ctx context.Context) (*component.Catalog, error) {
			return component.LoadCatalog(ctx, s.App)
		})
	return cat, info.Stale, err
}

func (s *Service) CountiesGeoJSON(ctx context.Context) ([]byte, bool, error) {
	b, info, err := s.geo.Get(ctx, "geo", []string{configTag}, geoTTL,
		func(ctx context.Context) ([]byte, error) {
			return component.CountiesGeoJSON(ctx, s.App)
		})
	return b, info.Stale, err
}

func (s *Service) Layer(ctx context.Context, slug string, id int, county, typhoon string) ([]byte, bool, error) {
	cat, catStale, err := s.Catalog(ctx)
	if err != nil {
		return nil, false, err
	}
	c := cat.Components[slug]
	if c == nil {
		return nil, false, notFound("component_not_found", "找不到這個元件："+slug)
	}
	l := c.Layer(id)
	if l == nil || l.GeoJSON == nil {
		return nil, false, notFound("layer_not_found", "找不到這個地圖圖層")
	}
	var countyParam *string
	if county != "" {
		if !cat.IsCounty(county) {
			return nil, false, badRequest("invalid_county", "縣市代碼不存在："+county)
		}
		countyParam = &county
	}
	q := *l.GeoJSON
	var typhoonParam *string
	if typhoon != "" {
		typhoonParam = &typhoon
	}
	key := fmt.Sprintf("layer|%s|%d|%s|%s", slug, id, county, typhoon)
	b, info, err := s.layers.Get(ctx, key, c.DatasetIDs, dataTTL(cat, c),
		func(ctx context.Context) ([]byte, error) {
			now := s.Now().Truncate(time.Minute)
			from := now.Add(-component.Ranges[c.TimeRange])
			rows, err := s.Reader.Query(ctx, q, component.Params{County: countyParam, From: from, To: now,
				Step: component.StepFor(from, now), Typhoon: typhoonParam})
			if err != nil {
				return nil, err
			}
			return component.BuildGeoJSON(rows)
		})
	return b, info.Stale || catStale, err
}

func (s *Service) Sources(ctx context.Context, c *component.Component) ([]component.Source, bool, error) {
	key := strings.Join(c.DatasetIDs, ",")
	src, info, err := s.sources.Get(ctx, key, c.DatasetIDs, minDataTTL,
		func(ctx context.Context) ([]component.Source, error) {
			return component.LoadSources(ctx, s.App, c.DatasetIDs)
		})
	return src, info.Stale, err
}

type Request struct {
	Slug    string
	County  string // empty for national
	From    *time.Time
	To      *time.Time
	History bool
	Typhoon string
}

type Result struct {
	Chart component.Chart
	Meta  Meta
	Rows  component.Rows // the raw query result, for CSV downloads
}

type Meta struct {
	Component  string             `json:"component"`
	Scope      string             `json:"scope"`
	DataShape  string             `json:"data_shape"`
	County     string             `json:"county,omitempty"`
	From       string             `json:"from,omitempty"`
	To         string             `json:"to,omitempty"`
	Step       string             `json:"step,omitempty"`
	ObservedAt *string            `json:"observed_at"`
	Sources    []component.Source `json:"sources"`
	Categories []string           `json:"categories,omitempty"`
	Stale      bool               `json:"stale,omitempty"`
	AsOf       *string            `json:"as_of,omitempty"`
}

func (s *Service) Data(ctx context.Context, req Request) (*Result, error) {
	cat, catStale, err := s.Catalog(ctx)
	if err != nil {
		return nil, err
	}
	c := cat.Components[req.Slug]
	if c == nil {
		return nil, notFound("component_not_found", "找不到這個元件："+req.Slug)
	}

	scope := "national"
	var county *string
	if req.County != "" {
		if !cat.IsCounty(req.County) {
			return nil, badRequest("invalid_county", "縣市代碼不存在："+req.County)
		}
		scope = "county"
		county = &req.County
	}

	q := c.Queries[scope]
	shape := ""
	var compiled component.Compiled
	switch {
	case req.History:
		h := s.historyQuery(c, scope)
		if h == nil {
			if scope == "county" && c.Queries["national"] != nil && c.Queries["national"].History != nil {
				return nil, badRequest("county_not_supported", "這個元件沒有縣市的歷史資料")
			}
			return nil, notFound("history_not_found", "這個元件沒有歷史資料")
		}
		shape, compiled = component.ShapeTime, *h
	case q == nil:
		return nil, badRequest("county_not_supported", "這個元件只有全國資料")
	default:
		shape, compiled = q.Shape, q.Chart
	}

	now := s.Now()
	var asOf time.Time
	if sources, _, err := s.Sources(ctx, c); err == nil {
		if at, paused := pausedAsOf(sources, now); paused {
			asOf, now = at, at
		}
	}

	from, to, err := s.timeRange(c, req, now)
	if err != nil {
		return nil, err
	}
	params := component.Params{County: county, From: from, To: to, Step: component.StepFor(from, to), AsOf: asOf}
	if req.Typhoon != "" {
		params.Typhoon = &req.Typhoon
	}

	key := fmt.Sprintf("%s|%t|%s|%s|%s|%s|%s", c.Slug, req.History, req.County, timeKey(req.From), timeKey(req.To), req.Typhoon, asOfKey(asOf))
	res, info, err := s.data.Get(ctx, key, c.DatasetIDs, dataTTL(cat, c),
		func(ctx context.Context) (*Result, error) {
			res, err := s.load(ctx, c, scope, shape, compiled, params)
			if err == nil && !asOf.IsZero() {
				at := formatTime(asOf)
				res.Meta.AsOf = &at
			}
			return res, err
		})
	if err != nil {
		return nil, err
	}
	if info.Stale || catStale {
		copied := *res
		copied.Meta.Stale = true
		return &copied, nil
	}
	return res, nil
}

func (s *Service) historyQuery(c *component.Component, scope string) *component.Compiled {
	if q := c.Queries[scope]; q != nil && q.History != nil {
		return q.History
	}
	if n := c.Queries["national"]; scope == "county" && n != nil && n.History != nil && n.History.Uses("county") {
		return n.History
	}
	return nil
}

func (s *Service) timeRange(c *component.Component, req Request, now time.Time) (time.Time, time.Time, error) {
	to := now.Truncate(time.Minute)
	if req.To != nil {
		to = *req.To
	}
	from := to.Add(-component.Ranges[c.TimeRange])
	if req.From != nil {
		from = *req.From
	}
	switch {
	case !from.Before(to):
		return time.Time{}, time.Time{}, badRequest("invalid_time_range", "from 必須早於 to")
	case to.Sub(from) > component.MaxRange:
		return time.Time{}, time.Time{}, badRequest("invalid_time_range", "時間範圍最長 5 年")
	}
	return from, to, nil
}

func (s *Service) load(ctx context.Context, c *component.Component, scope, shape string,
	q component.Compiled, p component.Params) (*Result, error) {
	rows, err := s.Reader.Query(ctx, q, p)
	if err != nil {
		return nil, err
	}
	chart, err := component.Build(shape, rows)
	if err != nil {
		return nil, err
	}
	sources, err := component.LoadSources(ctx, s.App, c.DatasetIDs)
	if err != nil {
		return nil, err
	}
	meta := Meta{
		Component:  c.Slug,
		Scope:      scope,
		DataShape:  shape,
		Sources:    sources,
		Categories: chart.Categories,
		ObservedAt: newest(sources),
	}
	if p.County != nil {
		meta.County = *p.County
	}
	if q.Uses("from") || q.Uses("to") {
		meta.From, meta.To = formatTime(p.From), formatTime(p.To)
	}
	if q.Uses("step") {
		meta.Step = p.Step
	}
	return &Result{Chart: chart, Meta: meta, Rows: rows}, nil
}

func newest(sources []component.Source) *string {
	var fastest int64
	for _, src := range sources {
		if src.LastObservedAt != nil && src.UpdateFreqSeconds > 0 && (fastest == 0 || src.UpdateFreqSeconds < fastest) {
			fastest = src.UpdateFreqSeconds
		}
	}
	var best *string
	for _, src := range sources {
		if src.LastObservedAt == nil || (fastest > 0 && src.UpdateFreqSeconds != fastest) {
			continue
		}
		if best == nil || *src.LastObservedAt > *best {
			best = src.LastObservedAt
		}
	}
	return best
}

func asOfKey(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func timeKey(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func IsClientError(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

func dataTTL(cat *component.Catalog, c *component.Component) time.Duration {
	ttl := maxDataTTL
	for _, id := range c.DatasetIDs {
		if f, ok := cat.UpdateFreq[id]; ok && f/2 < ttl {
			ttl = f / 2
		}
	}
	return max(ttl, minDataTTL)
}

func formatTime(t time.Time) string {
	return t.In(pipeline.Taipei).Format(time.RFC3339)
}
