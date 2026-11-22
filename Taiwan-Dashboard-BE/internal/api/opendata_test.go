package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/opendata"
	"dashboard.tw/backend/internal/testdb"
)

func newOpenEnv(t *testing.T) authEnv {
	t.Helper()
	db := testdb.New(t)
	testdb.RegisterDatasets(t, db)
	if _, err := db.Exec(`
		INSERT INTO ref.aqi_site (site_id, name, county_code, geom) VALUES
		  (1, '中山', '63000', ST_SetSRID(ST_Point(121.52, 25.06), 4326)),
		  (2, '前鎮', '64000', ST_SetSRID(ST_Point(120.31, 22.6), 4326));
		INSERT INTO data.aqi_obs_history (site_id, observed_at, aqi, status, pm25_ug_m3)
		SELECT s, now() - make_interval(mins => m), 40 + m % 10, '良好', 12.5
		  FROM generate_series(1, 2) s, generate_series(1, 600) m;
		INSERT INTO data.aqi_obs_current SELECT DISTINCT ON (site_id) * FROM data.aqi_obs_history ORDER BY site_id, observed_at DESC;
		INSERT INTO ref.reservoir (reservoir_id, name, county_code, effective_capacity) VALUES ('10201', '石門水庫', '68000', 20526);
		INSERT INTO data.reservoir_obs_current (reservoir_id, observed_at, storage_pct) VALUES ('10201', now(), 91.5);
		INSERT INTO ref.power_unit (unit_id, unit_name, plant_name, fuel_type) VALUES (1, '大潭CC#1', '大潭', 'gas');
		INSERT INTO data.power_gen_history (unit_id, observed_at, capacity_mw, net_gen_mw) VALUES (1, now(), 800, 650.5);
		INSERT INTO data.power_supply_daily (date, reserve_pct) VALUES (current_date - 1, 10.5), (current_date - 2, 9.8);
		INSERT INTO data.power_supply_forecast (forecast_date, published_on, reserve_pct) VALUES (current_date + 1, current_date, 11);
		UPDATE app.dataset SET status = 'ok', last_observed_at = now()`); err != nil {
		t.Fatal(err)
	}
	app := testdb.App(t, db)
	store := &auth.Store{DB: app}
	r := NewRouter(Deps{DB: fakePinger{}, OpenData: &opendata.Service{DB: app}, Auth: AuthDeps{
		Store: store, Providers: auth.Providers(false, false), Origins: []string{testOrigin},
	}})
	return authEnv{db: db, store: store, router: r}
}

func withQuota(t *testing.T, perDay, perMinute int) {
	t.Helper()
	old := auth.Tiers[auth.TierFree]
	auth.Tiers[auth.TierFree] = auth.Tier{PerDay: perDay, PerMinute: perMinute, BytesPerDay: old.BytesPerDay}
	t.Cleanup(func() { auth.Tiers[auth.TierFree] = old })
}

var validSurvey = map[string]any{
	"identity": "student", "purpose": "research", "topics": []string{"aqi", "power"},
	"app_name": "論文分析", "app_url": "https://example.org", "suggestion": "希望有雨量資料", "wants_push": "maybe", "agree_terms": true,
}

func (e authEnv) newClient(t *testing.T, session, name string) (clientID, secret string, id int64) {
	t.Helper()
	if w := e.send("PUT", "/api/v1/me/api-survey", session, validSurvey); w.Code != http.StatusNoContent {
		t.Fatalf("survey: %d %s", w.Code, w.Body)
	}
	w := e.send("POST", "/api/v1/me/api-clients", session, map[string]any{"name": name})
	if w.Code != http.StatusCreated {
		t.Fatalf("create client: %d %s", w.Code, w.Body)
	}
	got := decode[struct {
		Client auth.APIClient `json:"client"`
		Secret string         `json:"client_secret"`
	}](t, w)
	return got.Client.ClientID, got.Secret, got.Client.ID
}

var testAddrSeq atomic.Uint32

func testAddr() string {
	n := testAddrSeq.Add(1)
	return fmt.Sprintf("10.%d.%d.%d:40000", n>>16&255, n>>8&255, n&255)
}

func (e authEnv) token(clientID, secret string, basic bool) *httptest.ResponseRecorder {
	form := url.Values{"grant_type": {"client_credentials"}}
	if !basic {
		form.Set("client_id", clientID)
		form.Set("client_secret", secret)
	}
	req := httptest.NewRequest("POST", "/api/v1/auth/token", strings.NewReader(form.Encode()))
	req.RemoteAddr = testAddr()
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if basic {
		req.SetBasicAuth(url.QueryEscape(clientID), url.QueryEscape(secret))
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

type tokenAnswer struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Error       string `json:"error"`
}

func readToken(t *testing.T, w *httptest.ResponseRecorder) tokenAnswer {
	t.Helper()
	var a tokenAnswer
	if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil {
		t.Fatalf("token answer %q: %v", w.Body, err)
	}
	return a
}

func (e authEnv) newKey(t *testing.T, session string) string {
	t.Helper()
	id, secret, _ := e.newClient(t, session, "測試")
	w := e.token(id, secret, false)
	if w.Code != http.StatusOK {
		t.Fatalf("token: %d %s", w.Code, w.Body)
	}
	return readToken(t, w).AccessToken
}

func (e authEnv) withKey(path, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = testAddr()
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

type clientsState struct {
	Clients   []auth.APIClient `json:"clients"`
	UsedToday auth.Usage       `json:"used_today"`
	Usage     []auth.DayUsage  `json:"usage"`
}

func TestClientCredentialsLifecycle(t *testing.T) {
	e := newOpenEnv(t)
	withQuota(t, 1000, 1000)
	a := e.signInAs(t, auth.RoleUser, "active")
	b := e.signInAs(t, auth.RoleUser, "active")
	if w := e.send("PUT", "/api/v1/me/api-survey", b, validSurvey); w.Code != http.StatusNoContent {
		t.Fatalf("B's survey: %d %s", w.Code, w.Body)
	}

	if st := decode[clientsState](t, e.send("GET", "/api/v1/me/api-clients", a, nil)); len(st.Clients) != 0 || len(st.Usage) != 30 {
		t.Fatalf("before creating: %+v", st)
	}
	clientID, secret, id := e.newClient(t, a, "我的程式")
	if !regexp.MustCompile(`^[0-9a-f]{8}-[a-z2-7]{4}-[a-z2-7]{4}-[a-z2-7]{4}$`).MatchString(clientID) || strings.Count(secret, "-") != 5 {
		t.Errorf("client id %q, secret length %d", clientID, len(secret))
	}
	if page := e.send("GET", "/api/v1/me/api-clients", a, nil).Body.String(); strings.Contains(page, secret) {
		t.Error("the credential list contains the secret")
	}
	if n := e.count(t, `SELECT count(*) FROM app.api_key WHERE key_hash = sha256(convert_to($1, 'UTF8'))`, secret); n != 1 {
		t.Error("the database should hold the secret's SHA-256")
	}

	w := e.token(clientID, secret, false)
	tok := readToken(t, w)
	if w.Code != http.StatusOK || tok.TokenType != "Bearer" || tok.ExpiresIn != int(auth.TokenTTL.Seconds()) || tok.AccessToken == "" {
		t.Fatalf("token: %d %+v", w.Code, tok)
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("token Cache-Control = %q", cc)
	}
	if w := e.token(clientID, secret, true); w.Code != http.StatusOK {
		t.Errorf("basic auth token: %d %s", w.Code, w.Body)
	}
	for name, w := range map[string]*httptest.ResponseRecorder{
		"wrong secret":   e.token(clientID, secret+"x", false),
		"unknown client": e.token("00000000-nope-nope-nope", secret, false),
		"empty":          e.token("", "", false),
	} {
		if w.Code != http.StatusUnauthorized || readToken(t, w).Error != "invalid_client" {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	req := httptest.NewRequest("POST", "/api/v1/auth/token", strings.NewReader("grant_type=password&client_id="+clientID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rw := httptest.NewRecorder()
	e.router.ServeHTTP(rw, req)
	if rw.Code != http.StatusBadRequest || readToken(t, rw).Error != "unsupported_grant_type" {
		t.Errorf("password grant: %d %s", rw.Code, rw.Body)
	}
	if n := e.count(t, `SELECT count(*) FROM app.api_token WHERE token_hash = sha256(convert_to($1, 'UTF8'))`, tok.AccessToken); n != 1 {
		t.Error("the database should hold the token's SHA-256")
	}

	for i := 0; i < 3; i++ {
		if w := e.withKey("/api/v1/datasets", tok.AccessToken); w.Code != http.StatusOK {
			t.Fatalf("using the token: %d", w.Code)
		}
	}
	if w := e.withKey("/api/v1/datasets", secret); w.Code != http.StatusUnauthorized {
		t.Errorf("secret used as a token: %d, want 401", w.Code)
	}

	id2, secret2, _ := e.newClient(t, a, "第二個")
	tok2 := readToken(t, e.token(id2, secret2, false)).AccessToken
	if w := e.withKey("/api/v1/datasets", tok2); w.Header().Get("X-RateLimit-Remaining") != "996" {
		t.Errorf("second credential remaining = %q, want 996", w.Header().Get("X-RateLimit-Remaining"))
	}
	if st := decode[clientsState](t, e.send("GET", "/api/v1/me/api-clients", a, nil)); len(st.Clients) != 2 || st.UsedToday.Requests != 4 || st.UsedToday.Bytes <= 0 {
		t.Errorf("after use: %+v", st)
	}
	e.newClient(t, a, "第三個")
	if w := e.send("POST", "/api/v1/me/api-clients", a, map[string]any{"name": "第四個"}); w.Code != http.StatusConflict {
		t.Errorf("fourth credential: %d, want 409", w.Code)
	}

	path := fmt.Sprintf("/api/v1/me/api-clients/%d", id)
	if w := e.send("PATCH", path, a, map[string]any{"enabled": false}); w.Code != http.StatusNoContent {
		t.Fatalf("disable: %d %s", w.Code, w.Body)
	}
	if w := e.withKey("/api/v1/datasets", tok.AccessToken); w.Code != http.StatusUnauthorized {
		t.Errorf("token of a disabled credential: %d", w.Code)
	}
	if w := e.token(clientID, secret, false); w.Code != http.StatusUnauthorized {
		t.Errorf("token for a disabled credential: %d", w.Code)
	}
	e.send("PATCH", path, a, map[string]any{"enabled": true})
	tok = readToken(t, e.token(clientID, secret, false))

	w = e.send("POST", path+"/secret", a, nil)
	newSecret := decode[struct {
		Secret string `json:"client_secret"`
	}](t, w).Secret
	if w.Code != http.StatusOK || newSecret == "" || newSecret == secret {
		t.Fatalf("rotate: %d %s", w.Code, w.Body)
	}
	if w := e.withKey("/api/v1/datasets", tok.AccessToken); w.Code != http.StatusUnauthorized {
		t.Errorf("token after rotation: %d", w.Code)
	}
	if w := e.token(clientID, secret, false); w.Code != http.StatusUnauthorized {
		t.Errorf("old secret after rotation: %d", w.Code)
	}
	if w := e.token(clientID, newSecret, false); w.Code != http.StatusOK {
		t.Errorf("new secret: %d", w.Code)
	}

	if st := decode[clientsState](t, e.send("GET", "/api/v1/me/api-clients", b, nil)); len(st.Clients) != 0 || st.UsedToday.Requests != 0 {
		t.Errorf("user B sees %+v", st)
	}
	for _, r := range []struct{ method, path string }{
		{"PATCH", path}, {"POST", path + "/secret"}, {"DELETE", path},
	} {
		if w := e.send(r.method, r.path, b, map[string]any{"enabled": false}); w.Code != http.StatusNotFound {
			t.Errorf("B %s %s: %d, want 404", r.method, r.path, w.Code)
		}
	}

	if w := e.send("DELETE", path, a, nil); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}
	if w := e.token(clientID, newSecret, false); w.Code != http.StatusUnauthorized {
		t.Errorf("deleted credential: %d", w.Code)
	}
	if w := e.withKey("/api/v1/datasets", tok2); w.Code != http.StatusOK {
		t.Errorf("other credential after delete: %d", w.Code)
	}
	for action, want := range map[string]int{"client.create": 3, "client.disable": 1, "client.rotate": 1, "client.delete": 1} {
		if n := e.count(t, `SELECT count(*) FROM app.audit_log WHERE action = $1`, action); n != want {
			t.Errorf("%s audit rows = %d, want %d", action, n, want)
		}
	}
}

func TestAPIKeyScope(t *testing.T) {
	e := newOpenEnv(t)
	withQuota(t, 1000, 1000)
	a := e.signInAs(t, auth.RoleUser, "active")
	clientID, secret, _ := e.newClient(t, a, "測試")
	tw := e.token(clientID, secret, false)
	if tw.Code != http.StatusOK {
		t.Fatalf("token: %d %s", tw.Code, tw.Body)
	}
	key := readToken(t, tw).AccessToken

	if w := e.withKey("/api/v1/datasets", key); w.Code != http.StatusOK {
		t.Fatalf("valid key: %d %s", w.Code, w.Body)
	}
	changed := []byte(key)
	changed[len(changed)-1] ^= 1
	for name, k := range map[string]string{
		"no key":        "",
		"made up":       "dtw_aaaaaaaa_" + strings.Repeat("x", 43),
		"wrong shape":   "abc",
		"client secret": secret,
		"client id":     clientID,
		"changed token": string(changed),
	} {
		w := e.withKey("/api/v1/datasets", k)
		if w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("%s: %d, want 401 with WWW-Authenticate", name, w.Code)
		}
	}
	if w := e.call("GET", "/api/v1/datasets", a, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("session instead of key: %d, want 401", w.Code)
	}
	for _, p := range []string{"/api/v1/me", "/api/v1/me/api-clients", "/api/v1/admin/datasets"} {
		if w := e.withKey(p, key); w.Code != http.StatusUnauthorized {
			t.Errorf("key on %s: %d, want 401", p, w.Code)
		}
	}
	if _, err := e.db.Exec(`UPDATE app.users SET status = 'disabled'`); err != nil {
		t.Fatal(err)
	}
	if w := e.withKey("/api/v1/datasets", key); w.Code != http.StatusUnauthorized {
		t.Errorf("disabled user's key: %d, want 401", w.Code)
	}
	if _, err := e.db.Exec(`UPDATE app.users SET status = 'active'; UPDATE app.api_key SET revoked_at = now()`); err != nil {
		t.Fatal(err)
	}
	if w := e.withKey("/api/v1/datasets", key); w.Code != http.StatusUnauthorized {
		t.Errorf("revoked key: %d, want 401", w.Code)
	}

	req := httptest.NewRequest("OPTIONS", "/api/v1/datasets/aqi_obs/latest", nil)
	req.Header.Set("Origin", "https://example.org")
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent || !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Errorf("preflight: %d %v", w.Code, w.Header())
	}
}

func TestAPIKeyQuotas(t *testing.T) {
	e := newOpenEnv(t)
	a := e.signInAs(t, auth.RoleUser, "active")

	free := auth.Tiers[auth.TierFree]

	key := e.newKey(t, a)
	for i := 0; i < free.PerMinute; i++ {
		if w := e.withKey("/api/v1/datasets", key); w.Code != http.StatusOK {
			t.Fatalf("request %d: %d", i+1, w.Code)
		}
	}
	w := e.withKey("/api/v1/datasets", key)
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Errorf("request %d in a minute: %d, want 429 with Retry-After", free.PerMinute+1, w.Code)
	}
	if n := e.count(t, `SELECT coalesce(sum(requests), 0)::int FROM app.api_usage_daily`); n != free.PerMinute {
		t.Errorf("counted %d requests, want the %d accepted ones", n, free.PerMinute)
	}

	other := e.newKey(t, e.signInAs(t, auth.RoleUser, "active"))
	if _, err := e.db.Exec(`
		INSERT INTO app.api_usage_daily (api_key_id, day, requests)
		SELECT id, (now() AT TIME ZONE 'Asia/Taipei')::date, $1 FROM app.api_key ORDER BY id DESC LIMIT 1`, free.PerDay-1); err != nil {
		t.Fatal(err)
	}
	w = e.withKey("/api/v1/datasets", other)
	if w.Code != http.StatusOK || w.Header().Get("X-RateLimit-Remaining") != "0" ||
		w.Header().Get("X-RateLimit-Limit") != strconv.Itoa(free.PerDay) {
		t.Errorf("last request of the day: %d remaining=%q limit=%q", w.Code, w.Header().Get("X-RateLimit-Remaining"), w.Header().Get("X-RateLimit-Limit"))
	}
	w = e.withKey("/api/v1/datasets", other)
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "quota_exceeded") {
		t.Errorf("one over the daily quota: %d %s, want 429 quota_exceeded", w.Code, w.Body)
	}
	if n := e.count(t, `SELECT max(requests) FROM app.api_usage_daily`); n != free.PerDay {
		t.Errorf("stored %d requests, want %d: refused requests are not counted", n, free.PerDay)
	}
}

func TestOpenDataResponses(t *testing.T) {
	e := newOpenEnv(t)
	withQuota(t, 1000, 1000)
	key := e.newKey(t, e.signInAs(t, auth.RoleUser, "active"))
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		return e.withKey(path, key)
	}

	list := decode[[]opendata.Summary](t, get("/api/v1/datasets"))
	if len(list) != len(opendata.Datasets) || list[0].ID != "aqi_obs" {
		t.Fatalf("list = %+v", list)
	}
	desc := get("/api/v1/datasets/aqi_obs").Body.String()
	if !strings.Contains(desc, `"name":"pm25"`) || !strings.Contains(desc, `"unit":"μg/m³"`) {
		t.Errorf("field dictionary missing: %.300s", desc)
	}
	if w := get("/api/v1/datasets/nope"); w.Code != http.StatusNotFound {
		t.Errorf("unknown dataset: %d", w.Code)
	}

	w := get("/api/v1/datasets/aqi_obs/latest?county=63000")
	body := w.Body.String()
	if w.Code != http.StatusOK || strings.Count(body, `"site_id":`) != 1 || !strings.HasPrefix(strings.SplitN(body, `[{`, 2)[1], `"site_id":1,"observed_at":"`) {
		t.Errorf("latest: %d %.300s", w.Code, body)
	}
	if !strings.Contains(body, "+08:00") || !strings.Contains(body, `"pm25":12.5`) {
		t.Errorf("times should be in Taiwan time and numbers numeric: %.300s", body)
	}
	sites := get("/api/v1/datasets/reservoir_obs/sites").Body.String()
	if !strings.Contains(sites, `"county_code":"68000"`) || !strings.Contains(sites, `"effective_capacity":20526`) {
		t.Errorf("sites: %.300s", sites)
	}
	if body := get("/api/v1/datasets/power_supply_forecast/latest").Body.String(); !strings.Contains(body, `"reserve_pct":11`) {
		t.Errorf("forecast latest: %.300s", body)
	}

	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		path := "/api/v1/datasets/aqi_obs/history"
		if cursor != "" {
			path += "?cursor=" + cursor
		}
		w := get(path)
		if w.Code != http.StatusOK {
			t.Fatalf("page %d: %d %s", pages+1, w.Code, w.Body)
		}
		var page struct {
			Data []map[string]any `json:"data"`
			Meta struct {
				NextCursor string `json:"next_cursor"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		pages++
		for _, r := range page.Data {
			k := fmt.Sprint(r["site_id"], r["observed_at"])
			if seen[k] {
				t.Fatalf("row %s repeated", k)
			}
			seen[k] = true
		}
		if page.Meta.NextCursor == "" {
			break
		}
		cursor = page.Meta.NextCursor
		if pages > 5 {
			t.Fatal("pagination does not end")
		}
	}
	if pages != 2 || len(seen) != 1200 {
		t.Errorf("%d pages, %d rows; want 2 pages and 1200 rows", pages, len(seen))
	}

	w = get("/api/v1/datasets/aqi_obs/sites?format=csv")
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("csv content type %q", ct)
	}
	if first, _, _ := strings.Cut(w.Body.String(), "\n"); !strings.Contains(first, "site_id,name,county_code,lon,lat") {
		t.Errorf("csv header: %.80q", w.Body.String())
	}

	for path, want := range map[string]int{
		"/api/v1/datasets/power_supply_daily/latest?county=63000":        http.StatusBadRequest,
		"/api/v1/datasets/power_supply_daily/sites":                      http.StatusNotFound,
		"/api/v1/datasets/aqi_obs/latest?county=63":                      http.StatusBadRequest,
		"/api/v1/datasets/aqi_obs/latest?site=1%27%20OR%201=1":           http.StatusBadRequest,
		"/api/v1/datasets/aqi_obs/history?from=2026-01-01&to=2026-03-01": http.StatusBadRequest,
		"/api/v1/datasets/aqi_obs/history?from=2026-03-01&to=2026-01-01": http.StatusBadRequest,
		"/api/v1/datasets/aqi_obs/history?cursor=bm9wZQ":                 http.StatusBadRequest,
		"/api/v1/datasets/aqi_obs/latest?format=xml":                     http.StatusBadRequest,
		"/api/v1/datasets/aqi_obs/latest?foo=1":                          http.StatusBadRequest,
		"/api/v1/datasets/power_supply_daily/history?from=" +
			url.QueryEscape(time.Now().AddDate(0, 0, -10).Format(time.RFC3339)): http.StatusOK,
	} {
		if w := get(path); w.Code != want {
			t.Errorf("%s: %d %s, want %d", path, w.Code, w.Body, want)
		}
	}
}

func TestSecondsToTaipeiMidnight(t *testing.T) {
	now, _ := parseTime("2026-10-22T23:59:00+08:00")
	if s := secondsToTaipeiMidnight(*now); s != 61 {
		t.Errorf("one minute before midnight: %d seconds, want 61", s)
	}
}

func TestAPISurvey(t *testing.T) {
	e := newOpenEnv(t)
	a := e.signInAs(t, auth.RoleUser, "active")
	b := e.signInAs(t, auth.RoleUser, "active")
	admin := e.signInAs(t, auth.RoleAdmin, "active")

	if w := e.send("POST", "/api/v1/me/api-clients", a, map[string]any{"name": "x"}); w.Code != http.StatusConflict ||
		!strings.Contains(w.Body.String(), "survey_required") {
		t.Fatalf("key before the survey: %d %s, want 409 survey_required", w.Code, w.Body)
	}
	with := func(k string, v any) map[string]any {
		m := map[string]any{}
		for kk, vv := range validSurvey {
			m[kk] = vv
		}
		m[k] = v
		return m
	}
	for name, body := range map[string]any{
		"not agreed":      with("agree_terms", false),
		"bad identity":    with("identity", "alien"),
		"bad purpose":     with("purpose", ""),
		"no topics":       with("topics", []string{}),
		"unknown topic":   with("topics", []string{"rain"}),
		"repeated topic":  with("topics", []string{"aqi", "aqi"}),
		"script url":      with("app_url", "javascript:alert(1)"),
		"long suggestion": with("suggestion", strings.Repeat("字", 501)),
		"unknown field":   with("user_id", 2),
		"no push answer":  with("wants_push", ""),
		"bad push answer": with("wants_push", "always"),
	} {
		if w := e.send("PUT", "/api/v1/me/api-survey", a, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, w.Code)
		}
	}
	if w := e.send("PUT", "/api/v1/me/api-survey", a, validSurvey); w.Code != http.StatusNoContent {
		t.Fatalf("survey: %d %s", w.Code, w.Body)
	}
	if w := e.send("POST", "/api/v1/me/api-clients", a, map[string]any{"name": "x"}); w.Code != http.StatusCreated {
		t.Errorf("key after the survey: %d %s", w.Code, w.Body)
	}
	got := decode[struct {
		Survey       *survey `json:"survey"`
		TermsVersion string  `json:"terms_version"`
	}](t, e.send("GET", "/api/v1/me/api-clients", a, nil))
	if got.Survey == nil || got.Survey.Purpose != "research" || len(got.Survey.Topics) != 2 || got.Survey.TermsVersion != got.TermsVersion {
		t.Errorf("A's survey = %+v", got)
	}
	if s := decode[struct {
		Survey *survey `json:"survey"`
	}](t, e.send("GET", "/api/v1/me/api-clients", b, nil)); s.Survey != nil {
		t.Errorf("user B sees a survey: %+v", s.Survey)
	}

	if _, err := e.db.Exec(`UPDATE app.api_survey SET terms_version = '2000-01-01'`); err != nil {
		t.Fatal(err)
	}
	if w := e.send("POST", "/api/v1/me/api-clients", a, map[string]any{"name": "x"}); w.Code != http.StatusConflict {
		t.Errorf("key under outdated rules: %d, want 409", w.Code)
	}

	e.send("PUT", "/api/v1/me/api-survey", b, with("purpose", "journalism"))
	if w := e.send("GET", "/api/v1/admin/api-survey", a, nil); w.Code != http.StatusForbidden {
		t.Errorf("user reading the survey summary: %d, want 403", w.Code)
	}
	w := e.send("GET", "/api/v1/admin/api-survey", admin, nil)
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, `"respondents":2`) ||
		!strings.Contains(body, `{"value":"aqi","count":2,"active":1}`) || !strings.Contains(body, "希望有雨量資料") {
		t.Errorf("summary: %d %s", w.Code, body)
	}
	if strings.Contains(body, "user_id") || strings.Contains(body, "測試") {
		t.Errorf("summary leaks who answered: %s", body)
	}

	e.send("DELETE", "/api/v1/me", a, map[string]any{"confirm": true})
	if n := e.count(t, `SELECT count(*) FROM app.api_survey`); n != 1 {
		t.Errorf("%d surveys left, want only B's", n)
	}
}

func TestAdminAPIUsage(t *testing.T) {
	e := newOpenEnv(t)
	withQuota(t, 3, 1000)
	admin := e.signInAs(t, auth.RoleAdmin, "active")
	a := e.signInAs(t, auth.RoleUser, "active")
	b := e.signInAs(t, auth.RoleUser, "active")
	c := e.signInAs(t, auth.RoleUser, "active")

	keyA := e.newKey(t, a) // used: in use
	e.newKey(t, b)         // never used: idle
	_, _, cID := e.newClient(t, c, "c")
	e.send("DELETE", fmt.Sprintf("/api/v1/me/api-clients/%d", cID), c, nil) // revoked
	for _, p := range []string{"/api/v1/datasets", "/api/v1/datasets/aqi_obs/latest", "/api/v1/datasets/aqi_obs/sites"} {
		if w := e.withKey(p, keyA); w.Code != http.StatusOK {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
	if w := e.withKey("/api/v1/datasets", keyA); w.Code != http.StatusTooManyRequests {
		t.Fatalf("over quota: %d", w.Code)
	}

	if w := e.send("GET", "/api/v1/admin/api-usage?days=5", admin, nil); w.Code != http.StatusBadRequest {
		t.Errorf("days=5: %d, want 400", w.Code)
	}
	type usageResp struct {
		Accounts map[string]int `json:"accounts"`
		Totals   struct {
			Requests  int `json:"requests"`
			Rejected  int `json:"rejected"`
			Accounts  int `json:"accounts"`
			QuotaHits int `json:"quota_hits"`
		} `json:"totals"`
		Daily    []usageDay `json:"daily"`
		Datasets []struct {
			Dataset  string `json:"dataset"`
			Requests int    `json:"requests"`
		} `json:"datasets"`
		Top []usageAccount `json:"top"`
	}
	w := e.send("GET", "/api/v1/admin/api-usage?days=7", admin, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("usage: %d %s", w.Code, w.Body)
	}
	got := decode[usageResp](t, w)
	if got.Accounts["active"] != 1 || got.Accounts["idle"] != 1 || got.Accounts["revoked"] != 1 || got.Accounts["never_used"] != 1 {
		t.Errorf("accounts = %v", got.Accounts)
	}
	if got.Totals.Requests != 3 || got.Totals.Rejected != 1 || got.Totals.Accounts != 1 || got.Totals.QuotaHits != 1 {
		t.Errorf("totals = %+v", got.Totals)
	}
	if len(got.Daily) != 7 || got.Daily[6].Requests != 3 || got.Daily[6].Accounts != 1 {
		t.Errorf("daily = %+v", got.Daily)
	}
	byDataset := map[string]int{}
	for _, d := range got.Datasets {
		byDataset[d.Dataset] = d.Requests
	}
	if byDataset["aqi_obs"] != 2 || byDataset["_list"] != 1 {
		t.Errorf("datasets = %v (refused requests must not count)", byDataset)
	}
	if len(got.Top) != 3 || got.Top[0].Requests != 3 || got.Top[0].Status != "active" || got.Top[0].Prefix == nil {
		t.Errorf("top = %+v", got.Top)
	}
	if body := w.Body.String(); strings.Contains(body, "student") || strings.Contains(body, "research") {
		t.Errorf("the account list carries survey answers: %s", body)
	}
	if strings.Contains(w.Body.String(), keyA[strings.LastIndex(keyA, "_")+1:]) {
		t.Error("the usage page leaks a key")
	}

	s := e.send("GET", "/api/v1/admin/api-survey", admin, nil).Body.String()
	if !strings.Contains(s, `"respondents":3`) || !strings.Contains(s, `"active_respondents":2`) ||
		!strings.Contains(s, `{"value":"student","count":3,"active":2}`) || !strings.Contains(s, `{"value":"aqi","count":3,"active":2}`) {
		t.Errorf("survey summary: %s", s)
	}

	path := fmt.Sprintf("/api/v1/admin/users/%d/api-key", got.Top[0].UserID)
	if w := e.send("DELETE", path, a, nil); w.Code != http.StatusForbidden {
		t.Errorf("user revoking: %d, want 403", w.Code)
	}
	if w := e.send("DELETE", path, admin, nil); w.Code != http.StatusNoContent {
		t.Fatalf("admin revoke: %d %s", w.Code, w.Body)
	}
	if w := e.withKey("/api/v1/datasets/aqi_obs", keyA); w.Code != http.StatusUnauthorized {
		t.Errorf("key after admin revoke: %d, want 401", w.Code)
	}
	if w := e.send("DELETE", path, admin, nil); w.Code != http.StatusNotFound {
		t.Errorf("second revoke: %d, want 404", w.Code)
	}
	if n := e.count(t, `SELECT count(*) FROM app.audit_log WHERE action = 'admin.apikey.revoke'`); n != 1 {
		t.Errorf("audit rows = %d", n)
	}
}

func TestDownloads(t *testing.T) {
	e := newOpenEnv(t)
	withQuota(t, 1000, 1000)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "aqi_obs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"aqi_obs/index.json":     `{"dataset":"aqi_obs","files":[]}`,
		"aqi_obs/2026-09.csv.gz": "gzip bytes",
		"secret.txt":             "outside any dataset",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e.router = NewRouter(Deps{DB: fakePinger{}, DownloadDir: dir, OpenData: &opendata.Service{DB: e.store.DB}, Auth: AuthDeps{
		Store: e.store, Providers: auth.Providers(false, false), Origins: []string{testOrigin},
	}})
	key := e.newKey(t, e.signInAs(t, auth.RoleUser, "active"))

	if w := e.withKey("/downloads/aqi_obs/index.json", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("without a key: %d, want 401", w.Code)
	}
	w := e.withKey("/downloads/aqi_obs/index.json", key)
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") ||
		!strings.Contains(w.Body.String(), `"dataset":"aqi_obs"`) {
		t.Errorf("index: %d %v %s", w.Code, w.Header(), w.Body)
	}
	w = e.withKey("/downloads/aqi_obs/2026-09.csv.gz", key)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/gzip" ||
		!strings.Contains(w.Header().Get("Content-Disposition"), "aqi_obs-2026-09.csv.gz") || w.Body.String() != "gzip bytes" {
		t.Errorf("monthly file: %d %v", w.Code, w.Header())
	}
	for _, p := range []string{
		"/downloads/aqi_obs/2026-10.csv.gz",  // not written
		"/downloads/aqi_obs/2026-13.csv.gz",  // no such month
		"/downloads/aqi_obs/..%2Fsecret.txt", // traversal
		"/downloads/nope/index.json",         // unknown dataset
		"/downloads/power_gen/index.json",    // nothing exported yet
		"/downloads/aqi_obs/secret.txt",
	} {
		if w := e.withKey(p, key); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", p, w.Code)
		}
	}
	if n := e.count(t, `SELECT requests FROM app.api_usage_daily`); n != 7 {
		t.Errorf("counted %d requests, want 7", n)
	}
}

func TestOpenAPIIsPublic(t *testing.T) {
	e := newOpenEnv(t)
	if _, err := e.db.Exec(`UPDATE app.dataset SET name = '空氣品質指標（每小時）' WHERE id = 'aqi_obs'`); err != nil {
		t.Fatal(err)
	}
	w := e.send("GET", "/api/v1/openapi.json", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); !strings.HasPrefix(cc, "public") {
		t.Errorf("Cache-Control = %q", cc)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("the spec should be readable from other sites' tools")
	}
	var doc struct {
		OpenAPI string         `json:"openapi"`
		Paths   map[string]any `json:"paths"`
		Tags    []struct {
			Name string `json:"name"`
		} `json:"tags"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.OpenAPI != "3.1.0" || doc.Paths["/datasets/aqi_obs/history"] == nil {
		t.Errorf("doc = %.300s", w.Body.String())
	}
	found := false
	for _, tag := range doc.Tags {
		found = found || tag.Name == "空氣品質指標（aqi_obs）"
	}
	if !found {
		t.Errorf("no tag 空氣品質指標（aqi_obs） in %+v", doc.Tags)
	}
	for p := range doc.Paths {
		if strings.HasPrefix(p, "/downloads/") {
			t.Errorf("%s documented without DOWNLOAD_DIR", p)
		}
	}
	if w := e.send("GET", "/api/v1/openapi.json?x=1", "", nil); w.Code != http.StatusBadRequest {
		t.Errorf("unknown parameter: %d", w.Code)
	}
}
