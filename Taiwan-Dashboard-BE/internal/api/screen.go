package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/component"
	"dashboard.tw/backend/internal/csvout"
	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/screen"
)

const (
	cachePublic = "public, max-age=60, stale-while-revalidate=300, stale-if-error=86400"
	cacheStale  = "public, max-age=10"
	cacheLong   = "public, max-age=86400"
)

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type screenHandlers struct {
	svc *screen.Service
}

func registerScreen(g *gin.RouterGroup, svc *screen.Service) {
	h := screenHandlers{svc: svc}
	g.GET("/counties", allowParams(), h.counties)
	g.GET("/geo/counties.geojson", allowParams(), h.countiesGeoJSON)
	g.GET("/dashboards", allowParams(), h.dashboards)
	g.GET("/dashboards/:slug", allowParams(), h.dashboard)
	g.GET("/components", allowParams(), h.components)
	g.GET("/components/:slug", allowParams(), h.component)
	g.GET("/components/:slug/data", allowParams("county", "from", "to", "typhoon", "format"), h.data(false))
	g.GET("/components/:slug/history", allowParams("county", "from", "to", "typhoon", "format"), h.data(true))
	g.GET("/components/:slug/layers/:file", allowParams("county", "typhoon"), h.layer)
	g.GET("/typhoons", allowParams(), h.typhoons)
	g.GET("/earthquakes", allowParams(), h.quakes)
	g.GET("/earthquakes/:id", allowParams(), h.quake)
}

func allowParams(names ...string) gin.HandlerFunc {
	allowed := map[string]bool{}
	for _, n := range names {
		allowed[n] = true
	}
	return func(c *gin.Context) {
		for k, v := range c.Request.URL.Query() {
			if !allowed[k] {
				noStore(c)
				writeError(c, http.StatusBadRequest, "unknown_parameter", "不支援的參數："+k)
				return
			}
			if len(v) > 1 {
				noStore(c)
				writeError(c, http.StatusBadRequest, "repeated_parameter", "參數重複："+k)
				return
			}
		}
		c.Next()
	}
}

func (h screenHandlers) counties(c *gin.Context) {
	cat, stale, err := h.svc.Catalog(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	respondJSON(c, gin.H{"data": cat.Counties, "meta": staleMeta(stale)}, stale, cachePublic)
}

func (h screenHandlers) countiesGeoJSON(c *gin.Context) {
	b, stale, err := h.svc.CountiesGeoJSON(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	respond(c, "application/geo+json", b, stale, cacheLong)
}

type dashboardSummary struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	Description string `json:"description"`
}

func (h screenHandlers) dashboards(c *gin.Context) {
	cat, stale, err := h.svc.Catalog(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	out := make([]dashboardSummary, 0, len(cat.Dashboards))
	for _, d := range cat.Dashboards {
		out = append(out, dashboardSummary{Slug: d.Slug, Name: d.Name, Icon: d.Icon, Description: d.Description})
	}
	respondJSON(c, gin.H{"data": out, "meta": staleMeta(stale)}, stale, cachePublic)
}

type componentView struct {
	*component.Component
	Scopes     []string           `json:"scopes"`
	HasHistory bool               `json:"has_history"`
	Size       string             `json:"size,omitempty"`
	Sources    []component.Source `json:"sources,omitempty"`
}

func (h screenHandlers) dashboard(c *gin.Context) {
	slug := c.Param("slug")
	cat, stale, err := h.svc.Catalog(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	d := cat.Dashboard(slug)
	if !slugRe.MatchString(slug) || d == nil {
		h.fail(c, &screen.Error{Status: http.StatusNotFound, Code: "dashboard_not_found", Message: "找不到這個儀表板：" + slug})
		return
	}
	comps := make([]componentView, 0, len(d.Components))
	for _, item := range d.Components {
		comp := cat.Components[item.Component]
		comps = append(comps, componentView{Component: comp, Scopes: comp.Scopes(), HasHistory: comp.HasHistory(), Size: item.Size})
	}
	body := gin.H{"data": gin.H{
		"slug": d.Slug, "name": d.Name, "icon": d.Icon, "description": d.Description, "components": comps,
	}, "meta": staleMeta(stale)}
	respondJSON(c, body, stale, cachePublic)
}

var topicOrder = map[string]int{"weather": 0, "warning": 1, "earthquake": 2, "fire": 3, "aqi": 4, "reservoir": 5,
	"sustain": 6, "power": 7, "outage": 8, "resources": 9, "disease": 10, "agri": 11, "food": 12, "housing": 13,
	"traffic": 14, "rail": 15, "aviation": 16, "bike": 17, "safety": 18, "population": 19, "family": 20, "economy": 21,
	"animal": 22, "tourism": 23}

func (h screenHandlers) components(c *gin.Context) {
	cat, stale, err := h.svc.Catalog(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	comps := make([]componentView, 0, len(cat.Components))
	for _, comp := range cat.Components {
		comps = append(comps, componentView{Component: comp, Scopes: comp.Scopes(), HasHistory: comp.HasHistory()})
	}
	pos := map[string]int{}
	for _, d := range cat.Dashboards {
		for i, item := range d.Components {
			if _, ok := pos[item.Component]; !ok && d.Slug != "home" {
				pos[item.Component] = i
			}
		}
	}
	at := func(slug string) int {
		if p, ok := pos[slug]; ok {
			return p
		}
		return 1 << 20
	}
	sort.Slice(comps, func(i, j int) bool {
		a, b := comps[i].Component, comps[j].Component
		if topicOrder[a.Topic] != topicOrder[b.Topic] {
			return topicOrder[a.Topic] < topicOrder[b.Topic]
		}
		if at(a.Slug) != at(b.Slug) {
			return at(a.Slug) < at(b.Slug)
		}
		return a.Slug < b.Slug
	})
	respondJSON(c, gin.H{"data": comps, "meta": staleMeta(stale)}, stale, cachePublic)
}

func (h screenHandlers) component(c *gin.Context) {
	slug := c.Param("slug")
	cat, stale, err := h.svc.Catalog(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	comp := cat.Components[slug]
	if !slugRe.MatchString(slug) || comp == nil {
		h.fail(c, &screen.Error{Status: http.StatusNotFound, Code: "component_not_found", Message: "找不到這個元件：" + slug})
		return
	}
	sources, srcStale, err := h.svc.Sources(c.Request.Context(), comp)
	if err != nil {
		h.fail(c, err)
		return
	}
	stale = stale || srcStale
	view := componentView{Component: comp, Scopes: comp.Scopes(), HasHistory: comp.HasHistory(), Sources: sources}
	respondJSON(c, gin.H{"data": view, "meta": staleMeta(stale)}, stale, cachePublic)
}

func (h screenHandlers) data(history bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("slug")
		if !slugRe.MatchString(slug) {
			h.fail(c, &screen.Error{Status: http.StatusNotFound, Code: "component_not_found", Message: "找不到這個元件"})
			return
		}
		req := screen.Request{Slug: slug, History: history}
		if v := c.Query("county"); v != "" {
			if len(v) != 5 || strings.Trim(v, "0123456789") != "" {
				h.fail(c, &screen.Error{Status: http.StatusBadRequest, Code: "invalid_county", Message: "縣市代碼格式錯誤：" + v})
				return
			}
			req.County = v
		}
		if v := c.Query("typhoon"); v != "" {
			if !typhoonRe.MatchString(v) {
				h.fail(c, &screen.Error{Status: http.StatusBadRequest, Code: "invalid_typhoon", Message: "颱風代碼格式錯誤：" + v})
				return
			}
			req.Typhoon = v
		}
		var err error
		if req.From, err = parseTime(c.Query("from")); err != nil {
			h.fail(c, &screen.Error{Status: http.StatusBadRequest, Code: "invalid_time", Message: "from 格式錯誤，請使用 ISO 8601 或 YYYY-MM-DD"})
			return
		}
		if req.To, err = parseTime(c.Query("to")); err != nil {
			h.fail(c, &screen.Error{Status: http.StatusBadRequest, Code: "invalid_time", Message: "to 格式錯誤，請使用 ISO 8601 或 YYYY-MM-DD"})
			return
		}
		format := c.DefaultQuery("format", "json")
		if format != "json" && format != "csv" {
			h.fail(c, &screen.Error{Status: http.StatusBadRequest, Code: "invalid_format", Message: "format 只能是 json 或 csv"})
			return
		}

		res, err := h.svc.Data(c.Request.Context(), req)
		if err != nil {
			h.fail(c, err)
			return
		}
		cacheControl := cachePublic
		if res.Meta.Stale {
			cacheControl = cacheStale
		}
		if format == "csv" {
			name := slug
			if history {
				name += "-history"
			}
			c.Header("Content-Disposition", `attachment; filename="`+name+`.csv"`)
			respond(c, "text/csv; charset=utf-8", toCSV(res.Rows), res.Meta.Stale, cacheControl)
			return
		}
		respondJSON(c, gin.H{"data": res.Chart.Data, "meta": res.Meta}, res.Meta.Stale, cacheControl)
	}
}

func (h screenHandlers) layer(c *gin.Context) {
	slug, file := c.Param("slug"), c.Param("file")
	idText, ok := strings.CutSuffix(file, ".geojson")
	id, err := strconv.Atoi(idText)
	if !slugRe.MatchString(slug) || !ok || err != nil || id <= 0 {
		h.fail(c, &screen.Error{Status: http.StatusNotFound, Code: "layer_not_found", Message: "找不到這個地圖圖層"})
		return
	}
	county := c.Query("county")
	if county != "" && (len(county) != 5 || strings.Trim(county, "0123456789") != "") {
		h.fail(c, &screen.Error{Status: http.StatusBadRequest, Code: "invalid_county", Message: "縣市代碼格式錯誤：" + county})
		return
	}
	typhoon := c.Query("typhoon")
	if typhoon != "" && !typhoonRe.MatchString(typhoon) {
		h.fail(c, &screen.Error{Status: http.StatusBadRequest, Code: "invalid_typhoon", Message: "颱風代碼格式錯誤：" + typhoon})
		return
	}
	b, stale, err := h.svc.Layer(c.Request.Context(), slug, id, county, typhoon)
	if err != nil {
		h.fail(c, err)
		return
	}
	cacheControl := cachePublic
	if stale {
		cacheControl = cacheStale
	}
	respond(c, "application/geo+json", b, stale, cacheControl)
}

var typhoonRe = regexp.MustCompile(`^\d{4}-\d{1,3}$`)

func (h screenHandlers) typhoons(c *gin.Context) {
	list, err := h.svc.Typhoons(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	respondJSON(c, gin.H{"data": list}, false, cachePublic)
}

func (h screenHandlers) quakes(c *gin.Context) {
	list, err := h.svc.Quakes(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	respondJSON(c, gin.H{"data": list}, false, cachePublic)
}

var quakeIDRe = regexp.MustCompile(`^[0-9]{4,20}$`)

func (h screenHandlers) quake(c *gin.Context) {
	id := c.Param("id")
	if !quakeIDRe.MatchString(id) {
		noStore(c)
		writeError(c, http.StatusNotFound, "not_found", "找不到這份地震報告")
		return
	}
	d, err := h.svc.Quake(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	respondJSON(c, gin.H{"data": d}, false, cachePublic)
}

func (h screenHandlers) fail(c *gin.Context, err error) {
	noStore(c)
	if e, ok := screen.IsClientError(err); ok {
		writeError(c, e.Status, e.Code, e.Message)
		return
	}
	slog.Error("screen api", "path", c.Request.URL.Path, "error", err)
	if errors.Is(err, component.ErrTimeout) {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "資料載入失敗，請稍後再試")
		return
	}
	writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
}

func respondJSON(c *gin.Context, body any, stale bool, cacheControl string) {
	b, err := json.Marshal(body)
	if err != nil {
		noStore(c)
		writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
		return
	}
	respond(c, "application/json; charset=utf-8", b, stale, cacheControl)
}

func respond(c *gin.Context, contentType string, body []byte, stale bool, cacheControl string) {
	if stale && cacheControl != cacheStale {
		cacheControl = cacheStale
	}
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	c.Header("Cache-Control", cacheControl)
	c.Header("ETag", etag)
	c.Header("Access-Control-Allow-Origin", "*")
	if match := c.GetHeader("If-None-Match"); match != "" && match == etag {
		c.Status(http.StatusNotModified)
		return
	}
	c.Data(http.StatusOK, contentType, body)
}

func noStore(c *gin.Context) { c.Header("Cache-Control", "no-store") }

func staleMeta(stale bool) gin.H {
	if stale {
		return gin.H{"stale": true}
	}
	return gin.H{}
}

func parseTime(v string) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		t, err = time.ParseInLocation(time.DateOnly, v, pipeline.Taipei)
	}
	if err != nil {
		return nil, err
	}
	t = t.Truncate(time.Minute)
	return &t, nil
}

func toCSV(rows component.Rows) []byte {
	return csvout.Bytes(rows.Columns, rows.Values)
}
