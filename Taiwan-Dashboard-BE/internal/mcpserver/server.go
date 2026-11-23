package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dashboard.tw/backend/internal/component"
	"dashboard.tw/backend/internal/opendata"
	"dashboard.tw/backend/internal/screen"
)

const (
	defaultRows = 100
	maxRows     = 500
	searchLimit = 10
	fetchRows   = 30
)

var (
	datasetRe = regexp.MustCompile(`^[a-z0-9_]{1,60}$`)
	slugRe    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	siteRe    = regexp.MustCompile(`^[A-Za-z0-9-]{1,40}$`)
	taipei    = time.FixedZone("Asia/Taipei", 8*3600)
)

type Deps struct {
	Open    *opendata.Service
	Screen  *screen.Service
	Site    string
	Version string
	Meter   Meter
	OpenAPI func(ctx context.Context) map[string]any
}

type Meter interface {
	Charge(ctx context.Context, req *mcp.CallToolRequest, bytes int) error
}

type server struct{ Deps }

func New(d Deps) *mcp.Server {
	s := &server{d}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:        "dashboard-tw",
		Title:       "臺灣儀表板 Taiwan Dashboard",
		Description: "Live and historical Taiwan government open data: weather, disasters, air quality, water, power, transport, living and population.",
		Version:     d.Version,
		WebsiteURL:  d.Site,
		Icons:       []mcp.Icon{{Source: d.Site + "/logo.png", MIMEType: "image/png", Sizes: []string{"512x512"}}},
	}, &mcp.ServerOptions{Instructions: instructions})

	ro := func(title string) *mcp.ToolAnnotations {
		f := false
		return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: &f, OpenWorldHint: &f}
	}
	mcp.AddTool(srv, &mcp.Tool{Name: "search", Annotations: ro("Search datasets and charts"),
		Description: "Search Taiwan Dashboard datasets and charts by keyword, in English or Chinese (e.g. air quality, reservoir, typhoon, 停電). Pass a returned id to fetch to read it."}, s.search)
	mcp.AddTool(srv, &mcp.Tool{Name: "fetch", Annotations: ro("Read a dataset or chart"),
		Description: "Read an id returned by search: a dataset's description with its latest rows, or a chart's current values. Counts toward the daily API quota."}, s.fetch)
	mcp.AddTool(srv, &mcp.Tool{Name: "list_datasets", Annotations: ro("List datasets"),
		Description: "List all datasets with update frequency, latest observation time and whether they can be filtered by county. Optional topic: weather, environment, living, transport, society."}, s.listDatasets)
	mcp.AddTool(srv, &mcp.Tool{Name: "describe_dataset", Annotations: ro("Describe a dataset"),
		Description: "Get a dataset's fields, units, source agency, license and history query limits."}, s.describeDataset)
	mcp.AddTool(srv, &mcp.Tool{Name: "get_latest", Annotations: ro("Latest data"),
		Description: "Get the latest rows of a dataset. Filter by county (code like 63000, Chinese name like 臺北市, or English name like Taipei) or by site. For one station, reservoir or site, find its id with list_sites and pass site instead of fetching every row. Use next_cursor to page through large results. Counts toward the daily API quota."}, s.getLatest)
	mcp.AddTool(srv, &mcp.Tool{Name: "get_history", Annotations: ro("Historical data"),
		Description: "Get historical rows of a dataset for a time range of up to 31 days. Times are ISO 8601, e.g. 2026-11-01T00:00:00+08:00 (Taiwan time if no offset). Defaults to the last 24 hours. For one station, reservoir or site, find its id with list_sites and pass site. Counts toward the daily API quota."}, s.getHistory)
	mcp.AddTool(srv, &mcp.Tool{Name: "list_sites", Annotations: ro("List sites"),
		Description: "List the stations, reservoirs or other sites of a dataset with their ids, names and counties. Filter by county or by name (e.g. 石門, 中壢). Use the id as site in get_latest or get_history. Free."}, s.listSites)
	mcp.AddTool(srv, &mcp.Tool{Name: "get_component", Annotations: ro("Chart data"),
		Description: "Get the current values behind a chart on the website, with its page URL and embed code URL. Find slugs with search, e.g. aqi-by-county. Counts toward the daily API quota."}, s.getComponent)
	s.addDocs(srv, ro)
	return srv
}

const instructions = `Taiwan Dashboard (dashboard.tw) serves live and historical Taiwan government open data.
Find a dataset or chart with search or list_datasets, then read values with get_latest, get_history, get_component or fetch.
To build an app on the REST API, start with get_api_overview, then list_api_endpoints, get_api_endpoint and get_openapi_spec.
Data values, place names and station names are in Traditional Chinese; answer in the user's language.
Counties accept a code (63000 Taipei City, 65000 New Taipei City, 64000 Kaohsiung City), a Chinese name (臺北市) or an English name (Taipei).
Times are Taiwan time (UTC+8). search, list_datasets, describe_dataset and the API docs tools are free; tools that return data count toward the user's daily API quota. When quoting numbers, cite the returned source agency and URL.`

func (s *server) catalog(ctx context.Context) (*component.Catalog, error) {
	cat, _, err := s.Screen.Catalog(ctx)
	return cat, err
}

func (s *server) county(ctx context.Context, in string) (string, error) {
	in = strings.TrimSpace(in)
	if in == "" {
		return "", nil
	}
	cat, err := s.catalog(ctx)
	if err != nil {
		return "", err
	}
	norm := strings.ReplaceAll(in, "台", "臺")
	for _, c := range cat.Counties {
		en := strings.ToLower(c.NameEn)
		low := strings.ToLower(in)
		if in == c.Code || norm == c.Name || norm == c.ShortName || low == en ||
			low == strings.TrimSuffix(strings.TrimSuffix(en, " city"), " county") {
			return c.Code, nil
		}
	}
	return "", fmt.Errorf("county %q not found; use a code like 63000, a Chinese name like 臺北市 or an English name like Taipei", in)
}

func rowsLimit(n int) int {
	if n <= 0 {
		return defaultRows
	}
	return min(n, maxRows)
}

func (s *server) datasetURL(id string) string     { return s.Site + "/developers/datasets/" + id }
func (s *server) componentURL(slug string) string { return s.Site + "/components/" + slug }

type source struct {
	Agency  string `json:"agency"`
	License string `json:"license"`
	URL     string `json:"url"`
}

func sourceOf(x *opendata.Summary) source {
	return source{Agency: x.Agency, License: x.License, URL: x.SourceURL}
}

func observed(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.In(taipei).Format(time.RFC3339)
}

func tableRows(t opendata.Table) []map[string]any {
	out := make([]map[string]any, 0, len(t.Rows))
	for _, r := range t.Rows {
		m := make(map[string]any, len(t.Fields))
		for i, f := range t.Fields {
			m[f.Name] = r[i]
		}
		out = append(out, m)
	}
	return out
}

func (s *server) dataset(ctx context.Context, id string) (*opendata.Dataset, *opendata.Summary, error) {
	id = strings.TrimSpace(id)
	if !datasetRe.MatchString(id) {
		return nil, nil, fmt.Errorf("invalid dataset id %q; see list_datasets", id)
	}
	d := opendata.Find(id)
	if d == nil {
		return nil, nil, fmt.Errorf("dataset %s not found; see list_datasets", id)
	}
	sum, err := s.Open.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if sum == nil {
		return nil, nil, fmt.Errorf("dataset %s has no data yet", id)
	}
	return d, sum, nil
}

func (s *server) filters(ctx context.Context, d *opendata.Dataset, countyIn, siteIn string) (county, site *string, err error) {
	if countyIn != "" {
		if !d.HasCounty {
			return nil, nil, fmt.Errorf("dataset %s cannot be filtered by county", d.ID)
		}
		code, err := s.county(ctx, countyIn)
		if err != nil {
			return nil, nil, err
		}
		county = &code
	}
	if siteIn != "" {
		if !d.HasSite {
			return nil, nil, fmt.Errorf("dataset %s has no sites", d.ID)
		}
		if !siteRe.MatchString(siteIn) {
			return nil, nil, errors.New("invalid site id")
		}
		site = &siteIn
	}
	return county, site, nil
}

type searchIn struct {
	Query string `json:"query" jsonschema:"keywords in English or Chinese, e.g. air quality, reservoir, typhoon, 停電"`
}

type searchHit struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Text  string `json:"text,omitempty"`
}

type searchOut struct {
	Results []searchHit `json:"results"`
}

func score(q []string, fields ...string) int {
	n := 0
	for _, w := range q {
		for i, f := range fields {
			if strings.Contains(strings.ToLower(f), w) {
				n += len(fields) - i
			}
		}
	}
	return n
}

func (s *server) search(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, any, error) {
	q := expandQuery(in.Query)
	if len(q) == 0 {
		return nil, nil, errors.New("query is empty")
	}
	type hit struct {
		searchHit
		score int
	}
	var hits []hit
	list, err := s.Open.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, x := range list {
		if n := score(q, x.Name, x.ID, x.Description, x.Agency); n > 0 {
			hits = append(hits, hit{searchHit{ID: "dataset:" + x.ID, Title: x.Name + " (dataset)", URL: s.datasetURL(x.ID), Text: x.Description}, n})
		}
	}
	cat, err := s.catalog(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, c := range cat.Components {
		if n := score(q, c.Name, c.Slug, c.ShortDesc, c.LongDesc); n > 0 {
			hits = append(hits, hit{searchHit{ID: "component:" + c.Slug, Title: c.Name + " (chart)", URL: s.componentURL(c.Slug), Text: c.ShortDesc}, n})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].ID < hits[j].ID
	})
	out := searchOut{Results: []searchHit{}}
	for i := 0; i < len(hits) && i < searchLimit; i++ {
		out.Results = append(out.Results, hits[i].searchHit)
	}
	return nil, out, nil
}

type fetchIn struct {
	ID string `json:"id" jsonschema:"an id returned by search, e.g. dataset:aqi_obs or component:aqi-by-county"`
}

type fetchOut struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Text     string         `json:"text"`
	URL      string         `json:"url"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func textTable(cols []string, rows [][]any, limit int) string {
	var b strings.Builder
	b.WriteString(strings.Join(cols, "\t"))
	for i, r := range rows {
		if i >= limit {
			fmt.Fprintf(&b, "\n… (%d rows in total, showing the first %d)", len(rows), limit)
			break
		}
		b.WriteByte('\n')
		for j, v := range r {
			if j > 0 {
				b.WriteByte('\t')
			}
			if v != nil {
				fmt.Fprint(&b, v)
			}
		}
	}
	return b.String()
}

func (s *server) fetch(ctx context.Context, req *mcp.CallToolRequest, in fetchIn) (*mcp.CallToolResult, any, error) {
	kind, key, ok := strings.Cut(strings.TrimSpace(in.ID), ":")
	if !ok {
		return nil, nil, errors.New("id must look like dataset:aqi_obs or component:aqi-by-county")
	}
	switch kind {
	case "dataset":
		d, sum, err := s.dataset(ctx, key)
		if err != nil {
			return nil, nil, err
		}
		t, _, err := s.Open.LatestPage(ctx, d, nil, nil, "", fetchRows+1)
		if err != nil {
			return nil, nil, err
		}
		cols := make([]string, len(t.Fields))
		for i, f := range t.Fields {
			cols[i] = f.Name
		}
		text := fmt.Sprintf("%s\n%s\nSource: %s (%s)\nLatest observation: %s\n\nLatest rows:\n%s",
			sum.Name, sum.Description, sum.Agency, sum.License, observed(sum.LastObservedAt), textTable(cols, t.Rows, fetchRows))
		return s.bill(ctx, req, fetchOut{ID: in.ID, Title: sum.Name, Text: text, URL: s.datasetURL(d.ID),
			Metadata: map[string]any{"source": sourceOf(sum), "status": sum.Status}})
	case "component":
		res, comp, err := s.componentData(ctx, key, "")
		if err != nil {
			return nil, nil, err
		}
		text := fmt.Sprintf("%s\n%s\n%s\n\nCurrent values (%s):\n%s",
			comp.Name, comp.ShortDesc, comp.LongDesc, strOr(res.Meta.ObservedAt), textTable(res.Rows.Columns, res.Rows.Values, fetchRows))
		return s.bill(ctx, req, fetchOut{ID: in.ID, Title: comp.Name, Text: text, URL: s.componentURL(comp.Slug),
			Metadata: map[string]any{"sources": res.Meta.Sources, "unit": comp.Unit}})
	}
	return nil, nil, errors.New("id must start with dataset: or component:")
}

func strOr(p *string) string {
	if p == nil {
		return "time unknown"
	}
	return *p
}

type listIn struct {
	Topic string `json:"topic,omitempty" jsonschema:"only list this topic: weather, environment, living, transport or society"`
}

type datasetItem struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Topic             string `json:"topic"`
	Description       string `json:"description"`
	Status            string `json:"status"`
	LastObservedAt    string `json:"last_observed_at,omitempty"`
	UpdateFreqMinutes int64  `json:"update_freq_minutes"`
	HasCounty         bool   `json:"has_county"`
	HasSites          bool   `json:"has_sites"`
	HasHistory        bool   `json:"has_history"`
	URL               string `json:"url"`
}

func (s *server) listDatasets(ctx context.Context, _ *mcp.CallToolRequest, in listIn) (*mcp.CallToolResult, any, error) {
	list, err := s.Open.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := []datasetItem{}
	for _, x := range list {
		topic := opendata.TopicOf(x.ID)
		if in.Topic != "" && in.Topic != topic {
			continue
		}
		d := opendata.Find(x.ID)
		out = append(out, datasetItem{ID: x.ID, Name: x.Name, Topic: topic, Description: x.Description, Status: x.Status,
			LastObservedAt: observed(x.LastObservedAt), UpdateFreqMinutes: x.UpdateFreqSeconds / 60,
			HasCounty: x.HasCounty, HasSites: x.HasSites, HasHistory: d.HasHistory(), URL: s.datasetURL(x.ID)})
	}
	return nil, map[string]any{"datasets": out, "topics": opendata.Topics}, nil
}

type describeIn struct {
	Dataset string `json:"dataset" jsonschema:"dataset id, e.g. aqi_obs"`
}

func (s *server) describeDataset(ctx context.Context, _ *mcp.CallToolRequest, in describeIn) (*mcp.CallToolResult, any, error) {
	d, sum, err := s.dataset(ctx, in.Dataset)
	if err != nil {
		return nil, nil, err
	}
	sites := d.SiteFields
	if sites == nil {
		sites = []opendata.Field{}
	}
	return nil, map[string]any{
		"id": d.ID, "name": sum.Name, "description": sum.Description, "source": sourceOf(sum),
		"status": sum.Status, "last_observed_at": observed(sum.LastObservedAt),
		"update_freq_minutes": sum.UpdateFreqSeconds / 60,
		"fields":              d.Fields, "latest_fields": d.LatestRowFields(), "site_fields": sites,
		"has_county": d.HasCounty, "has_sites": d.HasSite, "has_history": d.HasHistory(),
		"history_max_days": int(opendata.MaxRange.Hours() / 24), "max_rows_per_call": maxRows,
		"url": s.datasetURL(d.ID),
	}, nil
}

type latestIn struct {
	Dataset string `json:"dataset" jsonschema:"dataset id, e.g. aqi_obs"`
	County  string `json:"county,omitempty" jsonschema:"county code (63000), Chinese name (臺北市) or English name (Taipei)"`
	Site    string `json:"site,omitempty" jsonschema:"site id"`
	Limit   int    `json:"limit,omitempty" jsonschema:"max rows to return, default 100, up to 500"`
	Cursor  string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous call"`
}

func (s *server) getLatest(ctx context.Context, req *mcp.CallToolRequest, in latestIn) (*mcp.CallToolResult, any, error) {
	d, sum, err := s.dataset(ctx, in.Dataset)
	if err != nil {
		return nil, nil, err
	}
	county, site, err := s.filters(ctx, d, in.County, in.Site)
	if err != nil {
		return nil, nil, err
	}
	limit := rowsLimit(in.Limit)
	t, next, err := s.Open.LatestPage(ctx, d, county, site, in.Cursor, limit)
	if errors.Is(err, opendata.ErrBadCursor) {
		return nil, nil, errors.New("invalid cursor; use next_cursor from the previous call")
	}
	if err != nil {
		return nil, nil, err
	}
	if !d.LatestPaged && len(t.Rows) > limit {
		t.Rows = t.Rows[:limit]
		next = ""
	}
	out := map[string]any{"dataset": d.ID, "name": sum.Name, "source": sourceOf(sum), "status": sum.Status,
		"last_observed_at": observed(sum.LastObservedAt), "rows": tableRows(t), "url": s.datasetURL(d.ID)}
	if next != "" {
		out["next_cursor"] = next
	}
	return s.bill(ctx, req, out)
}

type historyIn struct {
	Dataset string `json:"dataset" jsonschema:"dataset id, e.g. reservoir_obs"`
	From    string `json:"from,omitempty" jsonschema:"start time, ISO 8601, e.g. 2026-11-01T00:00:00+08:00"`
	To      string `json:"to,omitempty" jsonschema:"end time, ISO 8601, default now"`
	County  string `json:"county,omitempty" jsonschema:"county code (63000), Chinese name (臺北市) or English name (Taipei)"`
	Site    string `json:"site,omitempty" jsonschema:"site id"`
	Limit   int    `json:"limit,omitempty" jsonschema:"max rows to return, default 100, up to 500"`
	Cursor  string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous call"`
}

func parseTime(v string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, v, taipei); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q; use ISO 8601, e.g. 2026-11-01T00:00:00+08:00", v)
}

func (s *server) getHistory(ctx context.Context, req *mcp.CallToolRequest, in historyIn) (*mcp.CallToolResult, any, error) {
	d, sum, err := s.dataset(ctx, in.Dataset)
	if err != nil {
		return nil, nil, err
	}
	if !d.HasHistory() {
		return nil, nil, fmt.Errorf("dataset %s has no history; use get_latest", d.ID)
	}
	county, site, err := s.filters(ctx, d, in.County, in.Site)
	if err != nil {
		return nil, nil, err
	}
	to := time.Now()
	if in.To != "" {
		if to, err = parseTime(in.To); err != nil {
			return nil, nil, err
		}
	}
	from := to.Add(-opendata.DefaultRange)
	if in.From != "" {
		if from, err = parseTime(in.From); err != nil {
			return nil, nil, err
		}
	}
	if !from.Before(to) {
		return nil, nil, errors.New("from must be before to")
	}
	if to.Sub(from) > opendata.MaxRange {
		return nil, nil, errors.New("the range can be at most 31 days")
	}
	t, next, err := s.Open.History(ctx, d, opendata.HistoryQuery{County: county, Site: site, From: from, To: to,
		Cursor: in.Cursor, Limit: rowsLimit(in.Limit)})
	if errors.Is(err, opendata.ErrBadCursor) {
		return nil, nil, errors.New("invalid cursor; use next_cursor from the previous call")
	}
	if err != nil {
		return nil, nil, err
	}
	out := map[string]any{"dataset": d.ID, "name": sum.Name, "source": sourceOf(sum),
		"from": from.In(taipei).Format(time.RFC3339), "to": to.In(taipei).Format(time.RFC3339),
		"rows": tableRows(t), "url": s.datasetURL(d.ID)}
	if next != "" {
		out["next_cursor"] = next
	}
	return s.bill(ctx, req, out)
}

type componentIn struct {
	Slug   string `json:"slug" jsonschema:"chart slug, e.g. aqi-by-county"`
	County string `json:"county,omitempty" jsonschema:"county code, Chinese or English name, to show one county only"`
}

func (s *server) componentData(ctx context.Context, slug, countyIn string) (*screen.Result, *component.Component, error) {
	slug = strings.TrimSpace(slug)
	if !slugRe.MatchString(slug) {
		return nil, nil, fmt.Errorf("invalid chart slug %q", slug)
	}
	cat, err := s.catalog(ctx)
	if err != nil {
		return nil, nil, err
	}
	comp := cat.Components[slug]
	if comp == nil {
		return nil, nil, fmt.Errorf("chart %s not found; try search", slug)
	}
	county, err := s.county(ctx, countyIn)
	if err != nil {
		return nil, nil, err
	}
	res, err := s.Screen.Data(ctx, screen.Request{Slug: slug, County: county})
	var se *screen.Error
	if errors.As(err, &se) {
		return nil, nil, errors.New(se.Message)
	}
	if err != nil {
		return nil, nil, err
	}
	return res, comp, nil
}

func (s *server) getComponent(ctx context.Context, req *mcp.CallToolRequest, in componentIn) (*mcp.CallToolResult, any, error) {
	res, comp, err := s.componentData(ctx, in.Slug, in.County)
	if err != nil {
		return nil, nil, err
	}
	rows := res.Rows.Values
	truncated := false
	if len(rows) > maxRows {
		rows, truncated = rows[:maxRows], true
	}
	out := map[string]any{
		"slug": comp.Slug, "name": comp.Name, "description": comp.ShortDesc, "detail": comp.LongDesc,
		"unit": comp.Unit, "scope": res.Meta.Scope, "county": res.Meta.County, "observed_at": res.Meta.ObservedAt,
		"sources": res.Meta.Sources, "columns": res.Rows.Columns, "rows": rows,
		"url": s.componentURL(comp.Slug), "embed_url": s.Site + "/api/v1/components/" + comp.Slug + "/embed",
	}
	if truncated {
		out["truncated"] = true
	}
	return s.bill(ctx, req, out)
}

func (s *server) bill(ctx context.Context, req *mcp.CallToolRequest, out any) (*mcp.CallToolResult, any, error) {
	if s.Meter != nil {
		raw, err := json.Marshal(out)
		if err != nil {
			return nil, nil, err
		}
		if err := s.Meter.Charge(ctx, req, len(raw)); err != nil {
			return nil, nil, err
		}
	}
	return nil, out, nil
}

type sitesIn struct {
	Dataset string `json:"dataset" jsonschema:"dataset id, e.g. reservoir_obs or aqi_obs"`
	County  string `json:"county,omitempty" jsonschema:"county code (63000), Chinese name (臺北市) or English name (Taipei)"`
	Name    string `json:"name,omitempty" jsonschema:"part of the site name, e.g. 石門 or 中壢"`
}

func (s *server) listSites(ctx context.Context, _ *mcp.CallToolRequest, in sitesIn) (*mcp.CallToolResult, any, error) {
	d, sum, err := s.dataset(ctx, in.Dataset)
	if err != nil {
		return nil, nil, err
	}
	if !d.HasSite {
		return nil, nil, fmt.Errorf("dataset %s has no sites", d.ID)
	}
	var county *string
	if in.County != "" {
		code, err := s.county(ctx, in.County)
		if err != nil {
			return nil, nil, err
		}
		county = &code
	}
	t, err := s.Open.Sites(ctx, d, county)
	if err != nil {
		return nil, nil, err
	}
	name := strings.ReplaceAll(strings.TrimSpace(in.Name), "台", "臺")
	if name != "" {
		kept := t.Rows[:0]
		for _, r := range t.Rows {
			for _, v := range r {
				if str, ok := v.(string); ok && strings.Contains(strings.ReplaceAll(str, "台", "臺"), name) {
					kept = append(kept, r)
					break
				}
			}
		}
		t.Rows = kept
	}
	rows := tableRows(t)
	truncated := len(rows) > maxRows
	if truncated {
		rows = rows[:maxRows]
	}
	out := map[string]any{"dataset": d.ID, "name": sum.Name, "id_field": d.SiteFields[0].Name, "count": len(t.Rows), "sites": rows}
	if truncated {
		out["truncated"] = true
	}
	return nil, out, nil
}
