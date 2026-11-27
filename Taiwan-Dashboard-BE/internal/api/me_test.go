package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/component"
	"dashboard.tw/backend/internal/screen"
	"dashboard.tw/backend/internal/testdb"
	"dashboard.tw/backend/seed"
)

func newMeEnv(t *testing.T) authEnv {
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
	app := testdb.App(t, db)
	store := &auth.Store{DB: app}
	svc := screen.New(app, &component.Executor{DB: testdb.Reader(t, db)})
	r := NewRouter(Deps{DB: fakePinger{}, Screen: svc, RatePerMinute: 6000, RateBurst: 1000, Auth: AuthDeps{
		Store:     store,
		Providers: auth.Providers(true, false),
		Origins:   []string{testOrigin},
	}})
	return authEnv{db: db, store: store, router: r}
}

func TestMeEndpointsNeedSignIn(t *testing.T) {
	e := newMeEnv(t)
	for _, ep := range []struct{ method, path string }{
		{"GET", "/api/v1/me"},
		{"GET", "/api/v1/me/account"},
		{"DELETE", "/api/v1/me"},
		{"GET", "/api/v1/me/api-clients"},
		{"POST", "/api/v1/me/api-clients"},
		{"PATCH", "/api/v1/me/api-clients/1"},
		{"DELETE", "/api/v1/me/api-clients/1"},
		{"PUT", "/api/v1/me/api-survey"},
	} {
		w := e.send(ep.method, ep.path, "", "{}")
		if w.Code != http.StatusUnauthorized {
			t.Errorf("anonymous %s %s: %d, want 401", ep.method, ep.path, w.Code)
		}
		if cc := w.Header().Get("Cache-Control"); cc != "private, no-store" {
			t.Errorf("%s %s: Cache-Control %q", ep.method, ep.path, cc)
		}
	}
	user := e.signInAs(t, auth.RoleUser, "active")
	if w := e.call("POST", "/api/v1/me/api-clients", user, "https://evil.example"); w.Code != http.StatusForbidden {
		t.Errorf("cross-site key: %d, want 403", w.Code)
	}
	for _, p := range []string{"/api/v1/me/favorites", "/api/v1/me/dashboards"} {
		if w := e.send("GET", p, user, nil); w.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d, want 404", p, w.Code)
		}
	}
}

func TestDeleteAccount(t *testing.T) {
	e := newMeEnv(t)
	a := e.signInAs(t, auth.RoleUser, "active")
	b := e.signInAs(t, auth.RoleUser, "active")
	admin := e.signInAs(t, auth.RoleAdmin, "active")
	for _, tok := range []string{a, b} {
		e.newClient(t, tok, "測試")
	}
	var aID int64
	if err := e.db.QueryRow(`SELECT user_id FROM app.session ORDER BY created_at LIMIT 1`).Scan(&aID); err != nil {
		t.Fatal(err)
	}

	if w := e.send("DELETE", "/api/v1/me", a, map[string]any{}); w.Code != http.StatusBadRequest {
		t.Errorf("without confirm: %d, want 400", w.Code)
	}
	if w := e.send("DELETE", "/api/v1/me", admin, map[string]any{"confirm": true}); w.Code != http.StatusForbidden {
		t.Errorf("admin deleting themselves: %d, want 403", w.Code)
	}
	w := e.send("DELETE", "/api/v1/me", a, map[string]any{"confirm": true})
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	cleared := false
	for _, ck := range w.Result().Cookies() {
		cleared = cleared || (ck.Name == "dtw_session" && ck.MaxAge < 0)
	}
	if !cleared {
		t.Error("the session cookie was not cleared")
	}
	if w := e.send("GET", "/api/v1/me", a, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("deleted user's session: %d, want 401", w.Code)
	}
	for _, q := range []string{
		`SELECT count(*) FROM app.users WHERE id = $1`,
		`SELECT count(*) FROM app.session WHERE user_id = $1`,
		`SELECT count(*) FROM app.api_key WHERE user_id = $1`,
		`SELECT count(*) FROM app.api_survey WHERE user_id = $1`,
	} {
		if n := e.count(t, q, aID); n != 0 {
			t.Errorf("%s: %d rows left", q, n)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM app.api_key WHERE user_id <> $1`, aID); n != 1 {
		t.Errorf("other users' keys: %d, want B's one", n)
	}
	if n := e.count(t, `SELECT count(*) FROM app.audit_log WHERE action = 'account.delete' AND target = $1`,
		fmt.Sprintf("user:%d", aID)); n != 1 {
		t.Errorf("audit rows = %d", n)
	}
	if w := e.send("GET", "/api/v1/me", b, nil); w.Code != http.StatusOK {
		t.Errorf("user B was signed out too: %d", w.Code)
	}
}

func TestDevUserSignIn(t *testing.T) {
	e := newMeEnv(t)
	providers := decode[[]auth.Provider](t, e.send("GET", "/api/v1/auth/providers", "", nil))
	if len(providers) != 2 || providers[1].ID != "dev-user" {
		t.Fatalf("providers = %+v", providers)
	}
	var first int64
	for i := 0; i < 2; i++ {
		w := e.send("POST", "/api/v1/auth/dev-user/start", "", nil)
		if w.Code != http.StatusOK {
			t.Fatalf("sign in: %d %s", w.Code, w.Body)
		}
		u := decode[auth.User](t, w)
		if u.Role != auth.RoleUser {
			t.Errorf("test user role = %q, want user", u.Role)
		}
		if i == 0 {
			first = u.ID
		} else if u.ID != first {
			t.Errorf("second sign-in made a new user %d, want %d", u.ID, first)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM app.users WHERE role = 'admin'`); n != 0 {
		t.Errorf("signing in as the test user created %d administrators", n)
	}
}

func TestPublicComponentList(t *testing.T) {
	r := seededRouter(t)
	w := do(r, "/api/v1/components")
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	if cc := w.Header().Get("Cache-Control"); cc != cachePublic {
		t.Errorf("Cache-Control = %q", cc)
	}
	body := w.Body.String()
	if strings.Contains(body, "SELECT") || strings.Count(body, `"slug":`) != publishedSeed(t) {
		t.Errorf("unexpected list %s", body)
	}
	if !strings.HasPrefix(strings.SplitN(body, `"slug":"`, 2)[1], "radar-image") ||
		!strings.Contains(strings.SplitN(body, `"topic":"`, 2)[1][:10], "weather") {
		t.Errorf("list should start with the weather dashboard's first card: %.200s", body)
	}
}

func publishedSeed(t *testing.T) int {
	t.Helper()
	bundle, err := component.ParseBundle(seed.MVP)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range bundle.Components {
		if c.Status == "" || c.Status == "published" {
			n++
		}
	}
	return n
}
