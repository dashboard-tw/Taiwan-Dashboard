package api

import (
	"bytes"
	"encoding/xml"
	"log/slog"
	"net/http"
	"slices"
	"sort"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/screen"
)

var sitemapPages = []string{
	"/about", "/developers", "/showcase/app", "/showcase/web", "/showcase/innovation",
	"/typhoons", "/components", "/mapdata/map", "/sitemap", "/privacy", "/terms",
}

var sitemapSkipDashboards = []string{"home", "typhoon", "typhoon-event", "mapdata"}

type sitemapURL struct {
	Loc      string `xml:"loc"`
	Priority string `xml:"priority,omitempty"`
}

type sitemapSet struct {
	XMLName xml.Name     `xml:"urlset"`
	NS      string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

func sitemap(svc *screen.Service, base string) gin.HandlerFunc {
	return func(c *gin.Context) {
		cat, _, err := svc.Catalog(c.Request.Context())
		if err != nil || cat == nil {
			slog.Error("sitemap", "error", err)
			writeError(c, http.StatusServiceUnavailable, "unavailable", "暫時無法產生網站地圖")
			return
		}
		set := sitemapSet{NS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
		add := func(path, priority string) {
			set.URLs = append(set.URLs, sitemapURL{Loc: base + path, Priority: priority})
		}
		add("/", "1.0")
		for _, d := range cat.Dashboards {
			if slices.Contains(sitemapSkipDashboards, d.Slug) {
				continue
			}
			add("/"+d.Slug, "0.8")
			for _, county := range cat.Counties {
				add("/"+d.Slug+"/"+county.Code, "0.5")
			}
		}
		for _, county := range cat.Counties {
			add("/county/"+county.Code, "0.6")
		}
		slugs := make([]string, 0, len(cat.Components))
		for slug := range cat.Components {
			slugs = append(slugs, slug)
		}
		sort.Strings(slugs)
		for _, slug := range slugs {
			add("/components/"+slug, "0.4")
		}
		for _, p := range sitemapPages {
			add(p, "0.6")
		}

		var buf bytes.Buffer
		buf.WriteString(xml.Header)
		if err := xml.NewEncoder(&buf).Encode(set); err != nil {
			slog.Error("sitemap", "error", err)
			writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
			return
		}
		c.Header("Cache-Control", "public, max-age=3600")
		c.Data(http.StatusOK, "application/xml; charset=utf-8", buf.Bytes())
	}
}
