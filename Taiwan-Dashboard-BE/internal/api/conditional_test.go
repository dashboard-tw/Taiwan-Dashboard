package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestConditionalGet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"data":[1,2,3]}`
	var sizes []int
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Next()
		sizes = append(sizes, c.Writer.Size())
	}, conditionalGet())
	r.GET("/x", func(c *gin.Context) { c.Data(http.StatusOK, "application/json", []byte(body)) })
	r.GET("/missing", func(c *gin.Context) { writeError(c, http.StatusNotFound, "not_found", "找不到") })

	get := func(path, inm string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		if inm != "" {
			req.Header.Set("If-None-Match", inm)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	first := get("/x", "")
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || first.Body.String() != body || !strings.HasPrefix(etag, `W/"`) {
		t.Fatalf("first: %d %q etag %q", first.Code, first.Body, etag)
	}
	again := get("/x", etag)
	if again.Code != http.StatusNotModified || again.Body.Len() != 0 || again.Header().Get("ETag") != etag {
		t.Errorf("revalidate: %d body %q etag %q", again.Code, again.Body, again.Header().Get("ETag"))
	}
	if w := get("/x", `"something-else", `+strings.TrimPrefix(etag, "W/")); w.Code != http.StatusNotModified {
		t.Errorf("strong form in a list: %d", w.Code)
	}
	if w := get("/x", `W/"old"`); w.Code != http.StatusOK || w.Body.String() != body {
		t.Errorf("stale etag: %d %q", w.Code, w.Body)
	}
	if w := get("/missing", "*"); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "not_found") || w.Header().Get("ETag") != "" {
		t.Errorf("error answer: %d %q etag %q", w.Code, w.Body, w.Header().Get("ETag"))
	}
	if len(sizes) != 5 || sizes[0] != len(body) || sizes[1] > 0 || sizes[3] != len(body) {
		t.Errorf("sizes seen by the meter = %v, want [%d ≤0 ≤0 %d …]", sizes, len(body), len(body))
	}
}

func TestETagMatches(t *testing.T) {
	etag := `W/"abc"`
	for header, want := range map[string]bool{
		`W/"abc"`:         true,
		`"abc"`:           true,
		`"x", W/"abc"`:    true,
		`*`:               true,
		`W/"abd"`:         false,
		`"abc-different"`: false,
	} {
		if got := etagMatches(header, etag); got != want {
			t.Errorf("etagMatches(%q) = %v, want %v", header, got, want)
		}
	}
}
