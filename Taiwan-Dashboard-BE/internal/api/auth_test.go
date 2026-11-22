package api

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/testdb"
)

const testOrigin = "http://localhost:5173"

type authEnv struct {
	db     *sql.DB
	store  *auth.Store
	router http.Handler
}

func newAuthEnv(t *testing.T, devLogin bool) authEnv {
	t.Helper()
	db := testdb.New(t)
	store := &auth.Store{DB: testdb.App(t, db)}
	r := NewRouter(Deps{DB: fakePinger{}, Auth: AuthDeps{
		Store:     store,
		Providers: auth.Providers(devLogin, false),
		Origins:   []string{testOrigin},
	}})
	return authEnv{db: db, store: store, router: r}
}

func (e authEnv) call(method, path, cookie, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(nil))
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "dtw_session", Value: cookie})
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

func sessionCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == "dtw_session" && c.Value != "" {
			return c
		}
	}
	t.Fatalf("no session cookie in %v", w.Header())
	return nil
}

func (e authEnv) signInAs(t *testing.T, role, status string) string {
	t.Helper()
	var id int64
	if err := e.db.QueryRow(`INSERT INTO app.users (display_name, role, status) VALUES ('測試', $1, $2) RETURNING id`,
		role, status).Scan(&id); err != nil {
		t.Fatal(err)
	}
	token, _, err := e.store.CreateSession(context.Background(), id, "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestProvidersNeverIncludeDevInProduction(t *testing.T) {
	if p := auth.Providers(true, true); len(p) != 0 {
		t.Errorf("production providers = %v, want none", p)
	}
	if p := auth.Providers(false, false); len(p) != 0 {
		t.Errorf("dev login off: providers = %v, want none", p)
	}
	if p := auth.Providers(true, false); len(p) != 2 || p[0].ID != "dev" || p[1].ID != "dev-user" {
		t.Errorf("development providers = %v", p)
	}
}

func TestDevLoginDisabled(t *testing.T) {
	e := newAuthEnv(t, false)
	if _, err := e.store.CreateAdmin(context.Background(), "管理員"); err != nil {
		t.Fatal(err)
	}
	if w := e.call("GET", "/api/v1/auth/providers", "", ""); !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Errorf("providers = %s, want none", w.Body)
	}
	if w := e.call("POST", "/api/v1/auth/dev/start", "", testOrigin); w.Code != http.StatusNotFound {
		t.Errorf("dev sign-in while disabled: %d, want 404", w.Code)
	}
}

func TestDevLoginNeedsAdministrator(t *testing.T) {
	e := newAuthEnv(t, true)
	if w := e.call("POST", "/api/v1/auth/dev/start", "", testOrigin); w.Code != http.StatusConflict {
		t.Errorf("without an administrator: %d, want 409", w.Code)
	}
}

func TestSignInAndOut(t *testing.T) {
	e := newAuthEnv(t, true)
	ctx := context.Background()
	if _, err := e.store.CreateAdmin(ctx, "管理員"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.CreateAdmin(ctx, "第二個"); err == nil {
		t.Error("a second development administrator must be refused")
	}

	w := e.call("POST", "/api/v1/auth/dev/start", "", testOrigin)
	if w.Code != http.StatusOK {
		t.Fatalf("sign-in: %d %s", w.Code, w.Body)
	}
	c := sessionCookie(t, w)
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
		t.Errorf("cookie must be HttpOnly, SameSite=Lax, Path=/: %+v", c)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}

	var stored int
	if err := e.db.QueryRow(`SELECT count(*) FROM app.session WHERE id_hash = $1`, auth.HashToken(c.Value)).Scan(&stored); err != nil || stored != 1 {
		t.Errorf("session by hash = %d, %v", stored, err)
	}
	var raw int
	if err := e.db.QueryRow(`SELECT count(*) FROM app.session WHERE encode(id_hash, 'escape') LIKE '%' || $1 || '%'`, c.Value).Scan(&raw); err != nil || raw != 0 {
		t.Errorf("the raw token must not be stored: %d, %v", raw, err)
	}
	var logins int
	if err := e.db.QueryRow(`SELECT count(*) FROM app.audit_log WHERE action = 'login'`).Scan(&logins); err != nil || logins != 1 {
		t.Errorf("audit log logins = %d, %v", logins, err)
	}

	if w := e.call("GET", "/api/v1/me", c.Value, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"role":"admin"`) {
		t.Errorf("/me: %d %s", w.Code, w.Body)
	}

	w2 := e.call("POST", "/api/v1/auth/dev/start", c.Value, testOrigin)
	c2 := sessionCookie(t, w2)
	if c2.Value == c.Value {
		t.Error("a new sign-in must issue a new token")
	}
	if w := e.call("GET", "/api/v1/me", c.Value, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("old token after re-sign-in: %d, want 401", w.Code)
	}

	if w := e.call("POST", "/api/v1/auth/logout", c2.Value, testOrigin); w.Code != http.StatusNoContent {
		t.Errorf("logout: %d", w.Code)
	}
	if w := e.call("GET", "/api/v1/me", c2.Value, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("/me after logout: %d, want 401", w.Code)
	}
}

func TestStateChangesNeedOrigin(t *testing.T) {
	e := newAuthEnv(t, true)
	if _, err := e.store.CreateAdmin(context.Background(), "管理員"); err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"", "https://evil.example", "http://localhost:5174"} {
		if w := e.call("POST", "/api/v1/auth/dev/start", "", origin); w.Code != http.StatusForbidden {
			t.Errorf("origin %q: %d, want 403", origin, w.Code)
		}
	}
	token := e.signInAs(t, auth.RoleUser, "active")
	if w := e.call("POST", "/api/v1/auth/logout", token, "https://evil.example"); w.Code != http.StatusForbidden {
		t.Errorf("cross-site logout: %d, want 403", w.Code)
	}
}

func TestAdminAPIAuthorization(t *testing.T) {
	e := newAuthEnv(t, true)
	user := e.signInAs(t, auth.RoleUser, "active")
	admin := e.signInAs(t, auth.RoleAdmin, "active")
	disabled := e.signInAs(t, auth.RoleAdmin, "disabled")

	expired := e.signInAs(t, auth.RoleAdmin, "active")
	if _, err := e.db.Exec(`UPDATE app.session SET expires_at = now() - interval '1 second' WHERE id_hash = $1`, auth.HashToken(expired)); err != nil {
		t.Fatal(err)
	}
	revoked := e.signInAs(t, auth.RoleAdmin, "active")
	if err := e.store.Revoke(context.Background(), revoked); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/api/v1/admin/datasets", "/api/v1/admin/pipeline-runs"} {
		cases := []struct {
			name, token string
			want        int
		}{
			{"anonymous", "", http.StatusUnauthorized},
			{"made-up token", "not-a-real-token", http.StatusUnauthorized},
			{"user", user, http.StatusForbidden},
			{"disabled admin", disabled, http.StatusUnauthorized},
			{"expired admin", expired, http.StatusUnauthorized},
			{"revoked admin", revoked, http.StatusUnauthorized},
			{"admin", admin, http.StatusOK},
		}
		for _, c := range cases {
			w := e.call("GET", path, c.token, "")
			if w.Code != c.want {
				t.Errorf("%s %s: %d, want %d", c.name, path, w.Code, c.want)
			}
			if cc := w.Header().Get("Cache-Control"); cc != "private, no-store" {
				t.Errorf("%s %s: Cache-Control %q", c.name, path, cc)
			}
		}
	}
}

func TestSessionExpiresAfterTTL(t *testing.T) {
	db := testdb.New(t)
	now := time.Now()
	store := &auth.Store{DB: testdb.App(t, db), Now: func() time.Time { return now }}
	var id int64
	if err := db.QueryRow(`INSERT INTO app.users (display_name) VALUES ('測試') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	token, _, err := store.CreateSession(context.Background(), id, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Lookup(context.Background(), token); err != nil {
		t.Fatalf("fresh session: %v", err)
	}
	now = now.Add(auth.SessionTTL + time.Minute)
	if _, err := store.Lookup(context.Background(), token); err != auth.ErrNoSession {
		t.Errorf("after the TTL: err = %v, want ErrNoSession", err)
	}
}
