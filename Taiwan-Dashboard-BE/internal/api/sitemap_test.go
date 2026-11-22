package api

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRobotsPointsToSitemap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/open", robots(true, "https://example.test"))
	r.GET("/closed", robots(false, "https://example.test"))
	get := func(p string) string {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		return w.Body.String()
	}
	if body := get("/open"); !strings.Contains(body, "Sitemap: https://example.test/sitemap.xml") ||
		!strings.Contains(body, "Disallow: /embed/") {
		t.Errorf("indexable robots.txt %q", body)
	}
	if body := get("/closed"); strings.Contains(body, "Sitemap:") {
		t.Errorf("closed robots.txt should not list a sitemap: %q", body)
	}
}

func TestSitemap(t *testing.T) {
	w := do(seededRouter(t), "/sitemap.xml")
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/xml") {
		t.Fatalf("sitemap: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	var set struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &set); err != nil {
		t.Fatal(err)
	}
	locs := map[string]bool{}
	for _, u := range set.URLs {
		locs[u.Loc] = true
	}
	for _, want := range []string{"/", "/aqi", "/aqi/63000", "/county/63000", "/components/aqi-by-county", "/developers", "/showcase/app"} {
		if !locs[want] {
			t.Errorf("sitemap is missing %s", want)
		}
	}
	for _, skip := range []string{"/home", "/typhoon-event", "/mapdata"} {
		if locs[skip] {
			t.Errorf("sitemap lists %s", skip)
		}
	}
}
