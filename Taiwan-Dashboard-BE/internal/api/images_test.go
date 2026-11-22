package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestImageProxy(t *testing.T) {
	var asked []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		if r.Header.Get(imagePathHeader) != "" {
			t.Errorf("internal header leaked upstream")
		}
		if r.URL.Path == "/images/radar/20261022T0930Z.jpg" {
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write([]byte("\xff\xd8\xff"))
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL + "/images")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerImageProxy(r.Group("/api/v1"), base)
	site := httptest.NewServer(r)
	defer site.Close()
	get := func(path string, header ...string) *http.Response {
		req, _ := http.NewRequest("GET", site.URL+path, nil)
		if len(header) == 2 {
			req.Header.Set(header[0], header[1])
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}

	res := get("/api/v1/images/radar/20261022T0930Z.jpg", imagePathHeader, "../../etc/passwd")
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("frame: %d %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	if res := get("/api/v1/images/radar/20261022T0940Z.jpg"); res.StatusCode != http.StatusNotFound || res.Header.Get("Cache-Control") != "" {
		t.Errorf("missing frame: %d %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	asked = nil
	for _, p := range []string{"/api/v1/images/other/20261022T0930Z.jpg", "/api/v1/images/radar/..%2Fsecret.txt"} {
		if res := get(p); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d", p, res.StatusCode)
		}
	}
	if len(asked) != 0 {
		t.Errorf("invalid names reached the upstream: %v", asked)
	}
}

func TestImages(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "radar"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "radar", "20261022T0930Z.jpg"), []byte("\xff\xd8\xff"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("no"), 0o600); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerImages(r.Group("/api/v1"), dir)
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}

	if err := os.MkdirAll(filepath.Join(dir, "radar_layer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "radar_layer", "20261022T0930Z.png"), []byte("PNG"), 0o600); err != nil {
		t.Fatal(err)
	}
	if w := get("/api/v1/images/radar_layer/20261022T0930Z.png"); w.Code != http.StatusOK {
		t.Errorf("radar layer frame: %d", w.Code)
	}

	w := get("/api/v1/images/radar/20261022T0930Z.jpg")
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("frame: %d %q", w.Code, w.Header().Get("Cache-Control"))
	}
	for _, p := range []string{
		"/api/v1/images/radar/20261022T0940Z.jpg",
		"/api/v1/images/other/20261022T0930Z.jpg",
		"/api/v1/images/radar/..%2Fsecret.txt",
		"/api/v1/images/radar/secret.txt",
	} {
		if w := get(p); w.Code != http.StatusNotFound || w.Header().Get("Cache-Control") != "" {
			t.Errorf("%s: %d %q", p, w.Code, w.Header().Get("Cache-Control"))
		}
	}
}
