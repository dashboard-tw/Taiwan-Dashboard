package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func init() { gin.SetMode(gin.TestMode) }

func get(t *testing.T, r http.Handler, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	return w, body
}

func TestHealthOK(t *testing.T) {
	r := NewRouter(Deps{DB: fakePinger{}, Version: "test"})
	w, body := get(t, r, "/api/v1/health")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	data := body["data"].(map[string]any)
	if data["status"] != "ok" || data["database"] != "ok" || data["version"] != "test" {
		t.Errorf("data = %v", data)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

func TestHealthDatabaseDown(t *testing.T) {
	r := NewRouter(Deps{DB: fakePinger{err: errors.New("connection refused")}, Version: "test"})
	w, body := get(t, r, "/api/v1/health")

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	data := body["data"].(map[string]any)
	if data["database"] != "unavailable" {
		t.Errorf("database = %v, want unavailable", data["database"])
	}
}

func TestUnknownRouteUsesErrorEnvelope(t *testing.T) {
	r := NewRouter(Deps{DB: fakePinger{}, Version: "test"})
	w, body := get(t, r, "/api/v1/nope")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok || errObj["code"] != "not_found" {
		t.Errorf("body = %v", body)
	}
}
