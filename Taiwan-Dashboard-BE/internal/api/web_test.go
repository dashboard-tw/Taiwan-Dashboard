package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func webRouter(t *testing.T, indexable bool) http.Handler {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"index.html":      "<!doctype html><title>dashboard.tw</title>",
		"assets/app-1.js": "console.log(1)",
		"favicon.svg":     "<svg/>",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return NewRouter(Deps{DB: fakePinger{}, WebDir: dir, Indexable: indexable})
}

func webGet(h http.Handler, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestWebServesFrontend(t *testing.T) {
	h := webRouter(t, false)
	cases := []struct {
		path, body, cache string
		status            int
	}{
		{"/", "<!doctype html>", "no-cache", 200},
		{"/weather/63000", "<!doctype html>", "no-cache", 200}, // 前端的路由
		{"/admin", "<!doctype html>", "no-cache", 200},
		{"/assets/app-1.js", "console.log", "immutable", 200},
		{"/favicon.svg", "<svg/>", "max-age=3600", 200},
		{"/assets/old-0.js", "not found", "no-store", 404}, // 舊版的檔案不能回 index.html
	}
	for _, c := range cases {
		w := webGet(h, c.path)
		if w.Code != c.status || !strings.Contains(w.Body.String(), c.body) ||
			!strings.Contains(w.Header().Get("Cache-Control"), c.cache) {
			t.Errorf("%s: %d %q cache %q", c.path, w.Code, w.Body.String(), w.Header().Get("Cache-Control"))
		}
	}
	for _, p := range []string{"/api/v1/nope", "/downloads/x/index.json"} {
		if w := webGet(h, p); w.Code != 404 || !strings.Contains(w.Body.String(), `"not_found"`) {
			t.Errorf("%s: %d %s, want the JSON 404", p, w.Code, w.Body)
		}
	}
}

func TestWebStaysInsideItsFolder(t *testing.T) {
	h := webRouter(t, false)
	for _, p := range []string{"/../go.mod", "/..%2f..%2fgo.mod", "/assets/../../go.mod"} {
		w := webGet(h, p)
		if strings.Contains(w.Body.String(), "module ") {
			t.Errorf("%s leaked a file outside the web folder", p)
		}
	}
}

func TestWebSecurityHeaders(t *testing.T) {
	h := webRouter(t, false)
	w := webGet(h, "/")
	if w.Header().Get("X-Frame-Options") != "SAMEORIGIN" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("page headers %v", w.Header())
	}
	if w := webGet(h, "/embed/aqi-by-county"); w.Header().Get("X-Frame-Options") != "" {
		t.Error("embedded components must be allowed in other sites' iframes")
	}
}

func TestNotIndexableUntilLaunch(t *testing.T) {
	h := webRouter(t, false)
	for _, p := range []string{"/", "/api/v1/health"} {
		if got := webGet(h, p).Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
			t.Errorf("%s: X-Robots-Tag %q", p, got)
		}
	}
	if body := webGet(h, "/robots.txt").Body.String(); !strings.Contains(body, "Disallow: /\n") {
		t.Errorf("robots.txt %q", body)
	}

	open := webRouter(t, true)
	if got := webGet(open, "/").Header().Get("X-Robots-Tag"); got != "" {
		t.Errorf("indexable site still sends X-Robots-Tag %q", got)
	}
	body := webGet(open, "/robots.txt").Body.String()
	if strings.Contains(body, "Disallow: /\n") || !strings.Contains(body, "Disallow: /admin") {
		t.Errorf("indexable robots.txt %q", body)
	}
}
