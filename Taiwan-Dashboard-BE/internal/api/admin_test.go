package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/component"
	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/testdb"
	"dashboard.tw/backend/seed"
)

func newAdminEnv(t *testing.T) authEnv {
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
		INSERT INTO ref.reservoir (reservoir_id, name, effective_capacity) VALUES ('10201', '石門水庫', 20000), ('10405', '翡翠水庫', 33000);
		INSERT INTO data.reservoir_obs_current (reservoir_id, observed_at, effective_storage, storage_pct)
		VALUES ('10201', now(), 10000, 50);
		INSERT INTO ref.power_unit (unit_id, unit_name, plant_name, fuel_type) VALUES (1, '大潭CC#1', '大潭', 'gas')`); err != nil {
		t.Fatal(err)
	}
	store := &auth.Store{DB: testdb.App(t, db)}
	exec := &component.Executor{DB: testdb.Reader(t, db)}
	r := NewRouter(Deps{DB: fakePinger{}, Auth: AuthDeps{
		Store:     store,
		Providers: auth.Providers(false, false),
		Origins:   []string{testOrigin},
	}, Admin: AdminDeps{
		Reader:   exec.Query,
		RunOrder: []string{"aqi_obs", "power_gen", "nope"},
		RunDataset: func(_ context.Context, id string) (pipeline.Result, error) {
			switch id {
			case "aqi_obs":
				return pipeline.Result{RowsFetched: 3, RowsInserted: 2}, nil
			case "power_gen":
				return pipeline.Result{}, pipeline.ErrAlreadyRunning
			}
			return pipeline.Result{}, ErrUnknownDataset
		},
	}})
	return authEnv{db: db, store: store, router: r}
}

func (e authEnv) send(method, path, token string, body any) *httptest.ResponseRecorder {
	var b []byte
	switch v := body.(type) {
	case nil:
	case string:
		b = []byte(v)
	default:
		b, _ = json.Marshal(v)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "dtw_session", Value: token})
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var out struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", w.Body, err)
	}
	return out.Data
}

func (e authEnv) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAdminEndpointsAuthorization(t *testing.T) {
	e := newAdminEnv(t)
	user := e.signInAs(t, auth.RoleUser, "active")
	type endpoint struct{ method, path string }
	var endpoints []endpoint
	engine, ok := e.router.(*gin.Engine)
	if !ok {
		t.Fatal("router is not a gin engine")
	}
	for _, r := range engine.Routes() {
		if !strings.HasPrefix(r.Path, "/api/v1/admin") {
			continue
		}
		parts := strings.Split(r.Path, "/")
		for k, p := range parts {
			if strings.HasPrefix(p, ":") || strings.HasPrefix(p, "*") {
				parts[k] = "1"
			}
		}
		endpoints = append(endpoints, endpoint{r.Method, strings.Join(parts, "/")})
	}
	if len(endpoints) < 20 {
		t.Fatalf("only %d admin routes found", len(endpoints))
	}
	for _, ep := range endpoints {
		if w := e.send(ep.method, ep.path, "", "{}"); w.Code != http.StatusUnauthorized {
			t.Errorf("anonymous %s %s: %d, want 401", ep.method, ep.path, w.Code)
		}
		if w := e.send(ep.method, ep.path, user, "{}"); w.Code != http.StatusForbidden {
			t.Errorf("user %s %s: %d, want 403", ep.method, ep.path, w.Code)
		}
	}

	admin := e.signInAs(t, auth.RoleAdmin, "active")
	for _, origin := range []string{"", "https://evil.example"} {
		req := httptest.NewRequest("PUT", "/api/v1/admin/ref/power-units/1", strings.NewReader(`{"county_code":"63000"}`))
		req.AddCookie(&http.Cookie{Name: "dtw_session", Value: admin})
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		e.router.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("origin %q: %d, want 403", origin, w.Code)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM ref.power_unit WHERE county_code IS NOT NULL`); n != 0 {
		t.Errorf("a refused request changed %d power units", n)
	}
}

func TestAdminUpdateReservoir(t *testing.T) {
	e := newAdminEnv(t)
	admin := e.signInAs(t, auth.RoleAdmin, "active")
	path := "/api/v1/admin/ref/reservoirs/10201"
	valid := map[string]any{
		"name": "石門水庫", "county_code": "68000", "effective_capacity": 20500, "capacity_locked": true,
		"is_major": true, "lon": 121.24, "lat": 24.81,
	}
	with := func(k string, v any) map[string]any {
		m := map[string]any{}
		for kk, vv := range valid {
			m[kk] = vv
		}
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
		return m
	}

	for name, body := range map[string]any{
		"unknown county":  with("county_code", "99999"),
		"outside Taiwan":  with("lon", 130.0),
		"lon without lat": with("lat", nil),
		"zero capacity":   with("effective_capacity", 0),
		"empty name":      with("name", " "),
		"unknown field":   with("geom", "POINT(0 0)"),
	} {
		if w := e.send("PUT", path, admin, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", name, w.Code, w.Body)
		}
	}
	if w := e.send("PUT", "/api/v1/admin/ref/reservoirs/00000", admin, valid); w.Code != http.StatusNotFound {
		t.Errorf("unknown reservoir: %d, want 404", w.Code)
	}

	if w := e.send("PUT", path, admin, valid); w.Code != http.StatusNoContent {
		t.Fatalf("update: %d %s", w.Code, w.Body)
	}
	var county string
	var locked, major bool
	var lon, lat, pct float64
	if err := e.db.QueryRow(`
		SELECT r.county_code, r.capacity_locked, r.is_major, ST_X(r.geom), ST_Y(r.geom), o.storage_pct
		  FROM ref.reservoir r JOIN data.reservoir_obs_current o USING (reservoir_id)
		 WHERE reservoir_id = '10201'`).Scan(&county, &locked, &major, &lon, &lat, &pct); err != nil {
		t.Fatal(err)
	}
	if county != "68000" || !locked || !major || lon != 121.24 || lat != 24.81 {
		t.Errorf("saved %s locked=%v major=%v (%v, %v)", county, locked, major, lon, lat)
	}
	if pct != 48.78 { // 10000 / 20500
		t.Errorf("storage_pct = %v, want it recomputed from the new capacity", pct)
	}
	if n := e.count(t, `SELECT count(*) FROM app.audit_log WHERE action = 'ref.reservoir.update' AND target = 'reservoir:10201'
		AND detail->'before'->>'county_code' IS NULL AND detail->'after'->>'county_code' = '68000'`); n != 1 {
		t.Errorf("audit rows = %d, want one with before and after", n)
	}

	w := e.send("GET", "/api/v1/admin/ref/reservoirs", admin, nil)
	rows := decode[[]reservoirRow](t, w)
	if len(rows) != 2 || rows[0].ID != "10201" || rows[0].Lon == nil || rows[1].CountyCode != nil {
		t.Errorf("list = %+v", rows)
	}

	if w := e.send("PUT", path, admin, with("county_code", nil)); w.Code != http.StatusNoContent {
		t.Fatalf("clear county: %d %s", w.Code, w.Body)
	}
	if n := e.count(t, `SELECT count(*) FROM ref.reservoir WHERE reservoir_id = '10201' AND county_code IS NULL`); n != 1 {
		t.Error("county was not cleared")
	}
}

func TestAdminUpdatePowerUnit(t *testing.T) {
	e := newAdminEnv(t)
	admin := e.signInAs(t, auth.RoleAdmin, "active")
	if w := e.send("PUT", "/api/v1/admin/ref/power-units/1", admin, map[string]any{"county_code": "99999"}); w.Code != http.StatusBadRequest {
		t.Errorf("unknown county: %d", w.Code)
	}
	if w := e.send("PUT", "/api/v1/admin/ref/power-units/x", admin, map[string]any{}); w.Code != http.StatusNotFound {
		t.Errorf("bad id: %d", w.Code)
	}
	if w := e.send("PUT", "/api/v1/admin/ref/power-units/1", admin, map[string]any{"county_code": "68000"}); w.Code != http.StatusNoContent {
		t.Fatalf("update: %d %s", w.Code, w.Body)
	}
	units := decode[[]powerUnitRow](t, e.send("GET", "/api/v1/admin/ref/power-units", admin, nil))
	if len(units) != 1 || units[0].CountyCode == nil || *units[0].CountyCode != "68000" {
		t.Errorf("units = %+v", units)
	}
}

func TestAdminRunDataset(t *testing.T) {
	e := newAdminEnv(t)
	admin := e.signInAs(t, auth.RoleAdmin, "active")
	for id, want := range map[string]int{"aqi_obs": http.StatusOK, "power_gen": http.StatusConflict, "nope": http.StatusNotFound} {
		if w := e.send("POST", "/api/v1/admin/datasets/"+id+"/run", admin, nil); w.Code != want {
			t.Errorf("%s: %d %s, want %d", id, w.Code, w.Body, want)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM app.audit_log WHERE action = 'dataset.run' AND target = 'dataset:aqi_obs'`); n != 1 {
		t.Errorf("audit rows = %d", n)
	}
}

func TestAdminRunAll(t *testing.T) {
	e := newAdminEnv(t)
	admin := e.signInAs(t, auth.RoleAdmin, "active")
	if w := e.send("POST", "/api/v1/admin/datasets/run-all", admin, nil); w.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	var st struct {
		Data runAllState `json:"data"`
	}
	for i := 0; i < 100; i++ {
		w := e.send("GET", "/api/v1/admin/datasets/run-all", admin, nil)
		if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
			t.Fatal(err)
		}
		if !st.Data.Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	s := st.Data
	if s.Running || s.Done != 3 || s.Total != 3 || s.FinishedAt == nil {
		t.Fatalf("state %+v", s)
	}
	got := []string{s.Steps[0].Status, s.Steps[1].Status, s.Steps[2].Status}
	if got[0] != "ok" || got[1] != "skipped" || got[2] != "failed" || s.Steps[0].Inserted != 2 {
		t.Errorf("steps %+v", s.Steps)
	}
	if n := e.count(t, `SELECT count(*) FROM app.audit_log WHERE action = 'dataset.run_all'`); n != 1 {
		t.Errorf("audit rows = %d", n)
	}
}

func TestRunAllOneAtATime(t *testing.T) {
	var j runAll
	if !j.start([]string{"a"}, nil, time.Now()) {
		t.Fatal("first start")
	}
	if j.start([]string{"a"}, nil, time.Now()) {
		t.Error("a second job must wait for the first")
	}
	j.finish(time.Now())
	if !j.start([]string{"a"}, nil, time.Now()) {
		t.Error("a finished job can be started again")
	}
}

type specResponse struct {
	Spec    component.ComponentSpec `json:"spec"`
	Version int64                   `json:"version"`
}

func TestComponentEditor(t *testing.T) {
	e := newAdminEnv(t)
	admin := e.signInAs(t, auth.RoleAdmin, "active")
	base := "/api/v1/admin/components/aqi-by-county"

	list := decode[[]component.Summary](t, e.send("GET", "/api/v1/admin/components", admin, nil))
	if seeded := e.count(t, `SELECT count(*) FROM app.component`); len(list) != seeded || seeded < 100 {
		t.Errorf("list has %d components, want the %d seeded", len(list), seeded)
	}

	got := decode[specResponse](t, e.send("GET", base, admin, nil))
	if got.Spec.Slug != "aqi-by-county" || len(got.Spec.Queries) == 0 || got.Version == 0 {
		t.Fatalf("spec = %+v", got)
	}

	w := e.send("POST", "/api/v1/admin/components/preview", admin, map[string]any{"spec": got.Spec, "county": "64000"})
	if w.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", w.Code, w.Body)
	}
	pv := decode[struct {
		County string            `json:"county"`
		Checks []component.Check `json:"checks"`
	}](t, w)
	if pv.County != "64000" || len(pv.Checks) == 0 {
		t.Fatalf("preview = %+v", pv)
	}
	for _, ck := range pv.Checks {
		if ck.Error != "" {
			t.Errorf("%s: %s", ck.Label, ck.Error)
		}
	}

	evil := got.Spec
	evil.Queries = append([]component.QuerySpec(nil), got.Spec.Queries...)
	evil.Queries[0].ChartSQL = `SELECT display_name AS county_code, 1 AS data FROM app.users`
	w = e.send("POST", "/api/v1/admin/components/preview", admin, map[string]any{"spec": evil})
	pv = decode[struct {
		County string            `json:"county"`
		Checks []component.Check `json:"checks"`
	}](t, w)
	if len(pv.Checks) == 0 || !strings.Contains(pv.Checks[0].Error, "permission denied") {
		t.Errorf("reading app.users in preview: %+v", pv.Checks)
	}
	w = e.send("PUT", base, admin, map[string]any{"spec": evil, "base_version": got.Version})
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("saving a query on app.users: %d %s, want 422", w.Code, w.Body)
	}

	edited := got.Spec
	edited.Name = "各縣市 AQI（改）"
	w = e.send("PUT", base, admin, map[string]any{"spec": edited, "note": "改名", "base_version": got.Version})
	if w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	saved := decode[specResponse](t, w).Version
	if w := e.send("PUT", base, admin, map[string]any{"spec": edited, "base_version": got.Version}); w.Code != http.StatusConflict {
		t.Errorf("stale save: %d, want 409", w.Code)
	}
	renamed := edited
	renamed.Slug = "other"
	if w := e.send("PUT", base, admin, map[string]any{"spec": renamed, "base_version": saved}); w.Code != http.StatusBadRequest {
		t.Errorf("slug change: %d, want 400", w.Code)
	}

	w = e.send("POST", base+"/status", admin, map[string]any{"status": "draft", "base_version": saved})
	if w.Code != http.StatusOK {
		t.Fatalf("unpublish: %d %s", w.Code, w.Body)
	}
	if n := e.count(t, `SELECT count(*) FROM app.component WHERE slug = 'aqi-by-county' AND status = 'draft'`); n != 1 {
		t.Error("component is still published")
	}
	versions := decode[[]component.Version](t, e.send("GET", base+"/versions", admin, nil))
	if len(versions) != 3 || versions[0].Note != "取消發布" || versions[1].Note != "改名" ||
		versions[1].CreatedBy == nil || *versions[1].CreatedBy != "測試" {
		t.Fatalf("versions = %+v", versions)
	}
	first := versions[2].ID
	if w := e.send("POST", base+"/versions/"+itoa(first)+"/restore", admin, nil); w.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", w.Code, w.Body)
	}
	if n := e.count(t, `SELECT count(*) FROM app.component WHERE slug = 'aqi-by-county' AND status = 'published' AND name = $1`,
		got.Spec.Name); n != 1 {
		t.Error("restore did not bring back the first version")
	}
	if w := e.send("GET", "/api/v1/admin/components/power-reserve/versions/"+itoa(first), admin, nil); w.Code != http.StatusNotFound {
		t.Errorf("another component's version: %d, want 404", w.Code)
	}

	draft := got.Spec
	draft.Slug, draft.Status = "aqi-copy", "draft"
	if w := e.send("POST", "/api/v1/admin/components", admin, map[string]any{"spec": draft}); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	if w := e.send("POST", "/api/v1/admin/components", admin, map[string]any{"spec": draft}); w.Code != http.StatusConflict {
		t.Errorf("duplicate create: %d, want 409", w.Code)
	}
	if n := e.count(t, `SELECT count(*) FROM app.audit_log WHERE target LIKE 'component:%'`); n != 4 {
		t.Errorf("component audit rows = %d, want 4", n)
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestDatasetStaleWithoutWorker(t *testing.T) {
	e := newAdminEnv(t)
	admin := e.signInAs(t, auth.RoleAdmin, "active")
	if _, err := e.db.Exec(`
		UPDATE app.dataset SET status = 'ok', last_observed_at = now() - stale_after - interval '1 minute' WHERE id = 'aqi_obs';
		UPDATE app.dataset SET status = 'ok', last_observed_at = now(), last_ingested_at = '2026-10-20 08:00:00+00' WHERE id = 'power_gen'`); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, d := range decode[[]datasetHealth](t, e.send("GET", "/api/v1/admin/datasets", admin, nil)) {
		got[d.ID] = d.Status
	}
	if got["aqi_obs"] != "stale" || got["power_gen"] != "ok" {
		t.Errorf("admin statuses = %v, want aqi_obs stale and power_gen ok", got)
	}
	sources, err := component.LoadSources(context.Background(), e.db, []string{"aqi_obs", "power_gen"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[0].Status != "stale" || sources[1].Status != "ok" {
		t.Errorf("sources = %+v", sources)
	}
	if len(sources) == 2 {
		if p := sources[1]; p.LastIngestedAt == nil || *p.LastIngestedAt != "2026-10-20T16:00:00+08:00" || p.StaleAfterSeconds <= 0 {
			t.Errorf("power_gen last_ingested_at = %v, stale_after_seconds = %d", p.LastIngestedAt, p.StaleAfterSeconds)
		}
	}
}

func TestPurgeOldPipelineRuns(t *testing.T) {
	e := newAdminEnv(t)
	if _, err := e.db.Exec(`
		INSERT INTO app.pipeline_run (dataset_id, started_at, status)
		VALUES ('aqi_obs', now() - interval '91 days', 'success'),
		       ('aqi_obs', now() - interval '91 days', 'running'),
		       ('aqi_obs', now() - interval '89 days', 'failed')`); err != nil {
		t.Fatal(err)
	}
	p, err := auth.Purge(context.Background(), e.store.DB, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if p.Runs != 1 {
		t.Errorf("purged %d runs, want only the finished one older than 90 days", p.Runs)
	}
	if n := e.count(t, `SELECT count(*) FROM app.pipeline_run WHERE dataset_id = 'aqi_obs'`); n != 2 {
		t.Errorf("runs left = %d, want the recent one and the one still running", n)
	}
}
