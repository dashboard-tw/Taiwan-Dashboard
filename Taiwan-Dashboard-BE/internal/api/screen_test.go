package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dashboard.tw/backend/internal/component"
	"dashboard.tw/backend/internal/screen"
	"dashboard.tw/backend/internal/testdb"
	"dashboard.tw/backend/seed"
)

func do(r http.Handler, path string, header ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestScreenRejectsUnknownParameters(t *testing.T) {
	r := NewRouter(Deps{DB: fakePinger{}, Screen: screen.New(nil, nil)})
	for _, path := range []string{
		"/api/v1/components/aqi-by-county/data?foo=1",
		"/api/v1/components/aqi-by-county/data?county=63000&county=64000",
		"/api/v1/counties?cachebuster=123",
		"/api/v1/dashboards/home?x=",
	} {
		w := do(r, path)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", path, w.Code)
		}
		if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("%s: Cache-Control %q, errors must not be cached", path, cc)
		}
	}
}

func TestLimiter(t *testing.T) {
	l := newLimiter(60, 3) // one per second, burst of three
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }
	for i := range 3 {
		if ok, _ := l.allow("1.2.3.4"); !ok {
			t.Fatalf("request %d within the burst was refused", i+1)
		}
	}
	ok, wait := l.allow("1.2.3.4")
	if ok || wait <= 0 || wait > time.Second {
		t.Errorf("fourth request: ok=%v wait=%v, want refused for up to 1s", ok, wait)
	}
	if ok, _ := l.allow("5.6.7.8"); !ok {
		t.Error("another client has its own bucket")
	}
	now = now.Add(time.Second)
	if ok, _ := l.allow("1.2.3.4"); !ok {
		t.Error("a token should refill after one second")
	}
}

func TestScreenRateLimit(t *testing.T) {
	r := NewRouter(Deps{DB: fakePinger{}, Screen: screen.New(nil, nil), RatePerMinute: 60, RateBurst: 2})
	codes := []int{}
	for range 3 {
		codes = append(codes, do(r, "/api/v1/counties?bad=1").Code)
	}
	if codes[2] != http.StatusTooManyRequests {
		t.Errorf("codes = %v, want the third request limited", codes)
	}
	if w := do(r, "/api/v1/health"); w.Code != http.StatusOK {
		t.Errorf("health is not rate limited, got %d", w.Code)
	}
}

func seededRouter(t *testing.T) http.Handler {
	t.Helper()
	db := testdb.New(t)
	testdb.RegisterDatasets(t, db)
	bundle, err := component.ParseBundle(seed.MVP)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := component.Import(context.Background(), db, bundle, "test", false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO ref.aqi_site (site_id, name, county_code, geom)
		VALUES (1, '=HYPERLINK("x")', '63000', ST_SetSRID(ST_Point(121.5, 25.05), 4326));
		INSERT INTO data.aqi_obs_current (site_id, observed_at, aqi, status) VALUES (1, now(), 40, '良好');
		-- 一個已經解除警報的颱風，給每個颱風的專區
		INSERT INTO data.typhoon (typhoon_id, year, td_no, ty_no, name_en, name_zh) VALUES ('2026-33', 2026, 33, 29, 'CHAMPI', '薔琵');
		INSERT INTO data.typhoon_fix (typhoon_id, kind, issued_at, valid_at, geom, max_wind_ms) VALUES
			('2026-33', 'analysis', '2026-10-22 06:00+00', '2026-10-22 06:00+00', ST_SetSRID(ST_Point(123, 21), 4326), 45),
			('2026-33', 'analysis', '2026-10-22 12:00+00', '2026-10-22 12:00+00', ST_SetSRID(ST_Point(122, 22), 4326), 50);
		INSERT INTO data.typhoon_warning (effective_at, ty_no, warning_no, name_zh, alert, sections) VALUES
			('2026-10-22 11:30+08', 29, 5, '薔琵', '海上陸上颱風警報', '[]'),
			('2026-10-24 08:30+08', 29, 20, '薔琵', '解除颱風警報', '[]')`); err != nil {
		t.Fatal(err)
	}
	svc := screen.New(db, &component.Executor{DB: testdb.Reader(t, db)})
	return NewRouter(Deps{DB: fakePinger{}, Screen: svc, RatePerMinute: 6000, RateBurst: 1000})
}

func TestScreenResponses(t *testing.T) {
	r := seededRouter(t)

	w := do(r, "/api/v1/components/aqi-by-county/data")
	if w.Code != http.StatusOK {
		t.Fatalf("data: %d %s", w.Code, w.Body)
	}
	if cc := w.Header().Get("Cache-Control"); cc != cachePublic {
		t.Errorf("Cache-Control = %q", cc)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("screen API must allow embedding from any site")
	}
	body := w.Body.String()
	if !strings.Contains(body, `"county_code":"63000"`) || !strings.Contains(body, `"sources":[`) {
		t.Errorf("unexpected body %s", body)
	}

	etag := w.Header().Get("ETag")
	if w := do(r, "/api/v1/components/aqi-by-county/data", "If-None-Match", etag); w.Code != http.StatusNotModified {
		t.Errorf("matching ETag: status %d, want 304", w.Code)
	}

	for _, path := range []string{"/api/v1/components/aqi-by-county", "/api/v1/dashboards/aqi"} {
		w := do(r, path)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		if b := w.Body.String(); strings.Contains(b, "SELECT") || strings.Contains(b, "chart_sql") {
			t.Errorf("%s leaks SQL: %s", path, b)
		}
	}

	if w := do(r, "/api/v1/components/NOPE/data"); w.Code != http.StatusNotFound {
		t.Errorf("bad slug: %d", w.Code)
	}
	if w := do(r, "/api/v1/components/aqi-by-county/data?county=abc"); w.Code != http.StatusBadRequest {
		t.Errorf("malformed county: %d", w.Code)
	}
	if w := do(r, "/api/v1/components/aqi-by-county/history?from=yesterday"); w.Code != http.StatusBadRequest {
		t.Errorf("malformed from: %d", w.Code)
	}
}

func TestScreenCSV(t *testing.T) {
	r := seededRouter(t)
	w := do(r, "/api/v1/components/aqi-by-county/data?county=63000&format=csv")
	if w.Code != http.StatusOK {
		t.Fatalf("csv: %d %s", w.Code, w.Body)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q", ct)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "\xef\xbb\xbfx_axis,data\n") {
		t.Errorf("csv should start with a BOM and the header: %q", body)
	}
	if !strings.Contains(body, `"'=HYPERLINK(""x"")"`) {
		t.Errorf("formula not escaped: %q", body)
	}
}

func TestScreenLayerGeoJSON(t *testing.T) {
	r := seededRouter(t)
	w := do(r, "/api/v1/components/aqi-by-county")
	var body struct {
		Data struct {
			HasMapLayers bool `json:"has_map_layers"`
			MapLayers    []struct {
				ID         int    `json:"id"`
				SourceType string `json:"source_type"`
			} `json:"map_layers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	id := 0
	for _, l := range body.Data.MapLayers {
		if l.SourceType == "query" {
			id = l.ID
		}
	}
	if !body.Data.HasMapLayers || id == 0 {
		t.Fatalf("aqi-by-county should list a query layer: %s", w.Body)
	}

	w = do(r, fmt.Sprintf("/api/v1/components/aqi-by-county/layers/%d.geojson?county=63000", id))
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/geo+json") {
		t.Fatalf("layer: %d %s", w.Code, w.Body)
	}
	b := w.Body.String()
	if !strings.Contains(b, `"type":"Point"`) || !strings.Contains(b, `"county_code":"63000"`) || !strings.Contains(b, `"aqi":40`) {
		t.Errorf("unexpected GeoJSON %s", b)
	}

	for path, code := range map[string]int{
		"/api/v1/components/aqi-by-county/layers/999.geojson":                          http.StatusNotFound,
		"/api/v1/components/aqi-by-county/layers/abc.geojson":                          http.StatusNotFound,
		fmt.Sprintf("/api/v1/components/aqi-by-county/layers/%d.geojson?county=x", id): http.StatusBadRequest,
		fmt.Sprintf("/api/v1/components/aqi-by-county/layers/%d.geojson?foo=1", id):    http.StatusBadRequest,
	} {
		if w := do(r, path); w.Code != code {
			t.Errorf("%s: %d, want %d", path, w.Code, code)
		}
	}
}

func TestTyphoonPages(t *testing.T) {
	r := seededRouter(t)
	w := do(r, "/api/v1/typhoons")
	if w.Code != http.StatusOK {
		t.Fatalf("typhoons: %d %s", w.Code, w.Body)
	}
	var list struct {
		Data []screen.Typhoon `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list.Data) != 1 {
		t.Fatalf("typhoons %s, %v", w.Body, err)
	}
	ty := list.Data[0]
	if ty.ID != "2026-33" || ty.Active || ty.EndsAt == nil || ty.Warnings != 2 || *ty.NameZh != "薔琵" {
		t.Errorf("typhoon %+v", ty)
	}
	w = do(r, "/api/v1/components/typhoon-event-summary/data?typhoon=2026-33")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"x":"警報期間（小時）","y":45`) {
		t.Errorf("summary: %d %s", w.Code, w.Body)
	}
	if w := do(r, "/api/v1/components/typhoon-event-summary/data"); w.Code != http.StatusOK || strings.Contains(w.Body.String(), "警報期間") {
		t.Errorf("no typhoon: %d %s", w.Code, w.Body)
	}
	if w := do(r, "/api/v1/components/typhoon-event-summary/data?typhoon=x"); w.Code != http.StatusBadRequest {
		t.Errorf("malformed typhoon: %d", w.Code)
	}
	if w := do(r, "/api/v1/components/typhoon-event-track/layers/1.geojson?typhoon=x"); w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
		t.Errorf("malformed typhoon on a layer: %d", w.Code)
	}
}
