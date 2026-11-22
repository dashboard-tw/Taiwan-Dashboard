package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/auth"
)

const oauthOrigin = "http://127.0.0.1:5173"

type fakeOAuthStore struct {
	mu        sync.Mutex
	logins    map[string]auth.OAuthLogin
	users     map[string]*auth.User
	disabled  map[string]bool
	idents    []auth.Identity
	sessions  int
	audits    []string
	nextID    int64
	expireNow bool
	emails    map[string]int64
	pending   map[string]auth.PendingLink
	tokens    map[string]int64
}

func newFakeStore() *fakeOAuthStore {
	return &fakeOAuthStore{logins: map[string]auth.OAuthLogin{}, users: map[string]*auth.User{}, disabled: map[string]bool{},
		emails: map[string]int64{}, pending: map[string]auth.PendingLink{}}
}

func (s *fakeOAuthStore) SavePendingLink(_ context.Context, l auth.PendingLink) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[string(l.TokenHash)] = l
	return nil
}

func (s *fakeOAuthStore) PendingLinkInfo(_ context.Context, h []byte) (*auth.PendingLink, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.pending[string(h)]
	if !ok || !l.ExpiresAt.After(time.Now()) {
		return nil, auth.ErrNoSession
	}
	l.Existing = []string{}
	for key, u := range s.users {
		if u.ID == l.UserID {
			l.Existing = append(l.Existing, strings.SplitN(key, "|", 2)[0])
		}
	}
	return &l, nil
}

func (s *fakeOAuthStore) TakePendingLink(_ context.Context, h []byte) (*auth.PendingLink, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.pending[string(h)]
	delete(s.pending, string(h))
	if !ok || !l.ExpiresAt.After(time.Now()) {
		return nil, auth.ErrNoSession
	}
	return &l, nil
}

func (s *fakeOAuthStore) LinkIdentity(_ context.Context, userID int64, provider, subject, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := provider + "|" + subject
	if u, ok := s.users[key]; ok {
		if u.ID == userID {
			return nil
		}
		return auth.ErrIdentityTaken
	}
	for k, u := range s.users {
		if u.ID == userID && strings.HasPrefix(k, provider+"|") {
			return auth.ErrProviderLinked
		}
	}
	for _, u := range s.users {
		if u.ID == userID {
			s.users[key] = u
			s.audits = append(s.audits, "identity.link")
			return nil
		}
	}
	return auth.ErrNoSession
}

func (s *fakeOAuthStore) SaveOAuthLogin(_ context.Context, l auth.OAuthLogin) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.expireNow {
		l.ExpiresAt = time.Now().Add(-time.Second)
	}
	s.logins[string(l.StateHash)] = l
	return nil
}

func (s *fakeOAuthStore) TakeOAuthLogin(_ context.Context, stateHash []byte, provider string) (*auth.OAuthLogin, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.logins[string(stateHash)]
	if !ok || l.Provider != provider {
		return nil, auth.ErrNoSession
	}
	delete(s.logins, string(stateHash))
	if !l.ExpiresAt.After(time.Now()) {
		return nil, auth.ErrNoSession
	}
	return &l, nil
}

func (s *fakeOAuthStore) SignInExternal(_ context.Context, provider string, id auth.Identity) (*auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idents = append(s.idents, id)
	key := provider + "|" + id.Subject
	if s.disabled[key] {
		return nil, false, auth.ErrDisabled
	}
	if u, ok := s.users[key]; ok {
		return u, false, nil
	}
	if owner, ok := s.emails[id.Email]; ok && id.Email != "" {
		return nil, false, &auth.LinkRequired{UserID: owner}
	}
	s.nextID++
	u := &auth.User{ID: s.nextID, DisplayName: id.Name, Role: auth.RoleUser}
	s.users[key] = u
	if id.Email != "" {
		s.emails[id.Email] = u.ID
	}
	return u, true, nil
}

func (s *fakeOAuthStore) CreateSession(_ context.Context, userID int64, _, _ string) (string, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions++
	token := fmt.Sprintf("session-token-%d", s.sessions)
	if s.tokens == nil {
		s.tokens = map[string]int64{}
	}
	s.tokens[token] = userID
	return token, time.Now().Add(time.Hour), nil
}

func (s *fakeOAuthStore) Lookup(_ context.Context, token string) (*auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.tokens[token]
	if !ok {
		return nil, auth.ErrNoSession
	}
	for _, u := range s.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, auth.ErrNoSession
}

func (s *fakeOAuthStore) Revoke(context.Context, string) error { return nil }

func (s *fakeOAuthStore) Audit(_ context.Context, _ *int64, action, _ string, _ map[string]any, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audits = append(s.audits, action)
	return nil
}

type fakeProvider struct {
	srv         *httptest.Server
	mu          sync.Mutex
	challenges  map[string]string
	createdAt   time.Time
	emailVerify bool
	tokenFails  bool
	tokenSeen   url.Values
	accessToken string
	googleEmail string
	githubID    int
}

func newFakeProvider(t *testing.T) *fakeProvider {
	f := &fakeProvider{challenges: map[string]string{}, createdAt: time.Now().AddDate(-3, 0, 0), emailVerify: true,
		accessToken: "gho_SECRET_PROVIDER_TOKEN", googleEmail: "ming@example.com", githubID: 4242}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		defer f.mu.Unlock()
		f.tokenSeen = r.PostForm
		redirect, ok := f.challenges[auth.PKCEChallenge(r.PostForm.Get("code_verifier"))]
		switch {
		case f.tokenFails, r.PostForm.Get("code") != "good-code", !ok, redirect != r.PostForm.Get("redirect_uri"),
			r.PostForm.Get("client_secret") != "secret":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"bad_verification_code"}`))
		default:
			_, _ = w.Write([]byte(`{"access_token":"` + f.accessToken + `","token_type":"bearer"}`))
		}
	})
	authed := func(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer "+f.accessToken }
	mux.HandleFunc("/github/user", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": f.githubID, "login": "octo", "name": "Octo Cat", "created_at": f.createdAt})
	})
	mux.HandleFunc("/github/emails", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"email": "other@example.com", "primary": false, "verified": true},
			{"email": "octo@example.com", "primary": true, "verified": f.emailVerify},
		})
	})
	mux.HandleFunc("/google/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": "google-sub-1", "name": "陳小明", "email": f.googleEmail, "email_verified": f.emailVerify})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeProvider) github() *auth.OAuthProvider {
	p := auth.GitHub("client", "secret")
	p.AuthURL, p.TokenURL = f.srv.URL+"/authorize", f.srv.URL+"/token"
	p.UserURL, p.EmailsURL = f.srv.URL+"/github/user", f.srv.URL+"/github/emails"
	return p
}

func (f *fakeProvider) google() *auth.OAuthProvider {
	p := auth.Google("client", "secret")
	p.AuthURL, p.TokenURL, p.UserURL = f.srv.URL+"/authorize", f.srv.URL+"/token", f.srv.URL+"/google/userinfo"
	return p
}

type oauthEnv struct {
	t     *testing.T
	r     *gin.Engine
	store *fakeOAuthStore
	prov  *fakeProvider
}

func newOAuthEnv(t *testing.T) *oauthEnv {
	gin.SetMode(gin.TestMode)
	prov := newFakeProvider(t)
	store := newFakeStore()
	providers := []*auth.OAuthProvider{prov.github(), prov.google()}
	deps := AuthDeps{
		Providers:  auth.Providers(false, true, providers...),
		OAuth:      providers,
		Origins:    []string{oauthOrigin},
		oauthStore: store,
	}
	r := gin.New()
	registerAuth(r.Group("/api/v1"), deps)
	return &oauthEnv{t: t, r: r, store: store, prov: prov}
}

type started struct {
	state, challenge, redirectURI, scope string
	cookie                               *http.Cookie
	authURL                              *url.URL
}

func (e *oauthEnv) start(provider, origin, body string) (*httptest.ResponseRecorder, *started) {
	e.t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/"+provider+"/start", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	e.r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return w, nil
	}
	var resp struct {
		Data struct {
			RedirectURL string `json:"redirect_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		e.t.Fatal(err)
	}
	u, err := url.Parse(resp.Data.RedirectURL)
	if err != nil {
		e.t.Fatal(err)
	}
	q := u.Query()
	s := &started{state: q.Get("state"), challenge: q.Get("code_challenge"), redirectURI: q.Get("redirect_uri"),
		scope: q.Get("scope"), authURL: u}
	for _, c := range w.Result().Cookies() {
		if c.Name == "dtw_oauth" {
			s.cookie = c
		}
	}
	e.prov.mu.Lock()
	e.prov.challenges[s.challenge] = s.redirectURI
	e.prov.mu.Unlock()
	return w, s
}

func (e *oauthEnv) callback(provider, query string, cookie *http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/auth/"+provider+"/callback?"+query, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	e.r.ServeHTTP(w, req)
	return w
}

func okQuery(s *started) string {
	return url.Values{"code": {"good-code"}, "state": {s.state}}.Encode()
}

func location(w *httptest.ResponseRecorder) string { return w.Header().Get("Location") }

func newSessionCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == "dtw_session" && c.MaxAge > 0 {
			return c
		}
	}
	return nil
}

func TestOAuthSignInSucceeds(t *testing.T) {
	e := newOAuthEnv(t)
	_, s := e.start("github", oauthOrigin, `{"redirect":"/me/api-key"}`)
	if s == nil {
		t.Fatal("start failed")
	}
	q := s.authURL.Query()
	if q.Get("code_challenge_method") != "S256" || len(s.challenge) < 43 || len(s.state) < 40 {
		t.Errorf("authorization URL lacks PKCE or state: %v", q)
	}
	if s.redirectURI != oauthOrigin+"/api/v1/auth/github/callback" {
		t.Errorf("redirect_uri = %q", s.redirectURI)
	}
	if s.cookie == nil || !s.cookie.HttpOnly || s.cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("browser binding cookie = %+v", s.cookie)
	}
	for k := range e.store.logins {
		if k == s.state {
			t.Error("state stored in plain text")
		}
	}

	w := e.callback("github", okQuery(s), s.cookie)
	if w.Code != http.StatusSeeOther || location(w) != "/me/api-key" {
		t.Fatalf("callback = %d %q", w.Code, location(w))
	}
	if c := newSessionCookie(w); c == nil || !c.HttpOnly {
		t.Fatalf("no HttpOnly session cookie: %v", w.Result().Cookies())
	}
	if e.prov.tokenSeen.Get("code_verifier") == "" || e.prov.tokenSeen.Get("redirect_uri") != s.redirectURI {
		t.Errorf("token request = %v", e.prov.tokenSeen)
	}
	if len(e.store.idents) != 1 || e.store.idents[0].Subject != "4242" || e.store.idents[0].Email != "octo@example.com" {
		t.Errorf("identity = %+v", e.store.idents)
	}
	if strings.Contains(w.Body.String()+location(w)+w.Header().Get("Set-Cookie"), "SECRET_PROVIDER_TOKEN") {
		t.Error("provider token leaked into the response")
	}
	if len(e.store.audits) != 1 || e.store.audits[0] != "login" {
		t.Errorf("audits = %v", e.store.audits)
	}

	_, s2 := e.start("github", oauthOrigin, "")
	if w := e.callback("github", okQuery(s2), s2.cookie); location(w) != "/" || len(e.store.users) != 1 {
		t.Errorf("second sign-in = %q, users %d", location(w), len(e.store.users))
	}
}

func TestOAuthStateCanOnlyBeUsedOnce(t *testing.T) {
	e := newOAuthEnv(t)
	_, s := e.start("github", oauthOrigin, "")
	if w := e.callback("github", okQuery(s), s.cookie); newSessionCookie(w) == nil {
		t.Fatal("first use should sign in")
	}
	w := e.callback("github", okQuery(s), s.cookie)
	if location(w) != "/login?error=expired" || newSessionCookie(w) != nil {
		t.Errorf("replayed state = %q", location(w))
	}
}

func TestOAuthRejectsForgedOrMissingState(t *testing.T) {
	e := newOAuthEnv(t)
	_, s := e.start("github", oauthOrigin, "")
	for name, q := range map[string]string{
		"unknown state": url.Values{"code": {"good-code"}, "state": {"not-a-real-state"}}.Encode(),
		"no state":      url.Values{"code": {"good-code"}}.Encode(),
	} {
		w := e.callback("github", q, s.cookie)
		if !strings.HasPrefix(location(w), "/login?error=") || newSessionCookie(w) != nil {
			t.Errorf("%s: %q", name, location(w))
		}
	}
	if w := e.callback("google", okQuery(s), s.cookie); location(w) != "/login?error=expired" {
		t.Errorf("state used with another provider: %q", location(w))
	}
}

func TestOAuthCallbackMustComeBackToTheSameBrowser(t *testing.T) {
	e := newOAuthEnv(t)
	_, s := e.start("github", oauthOrigin, "")
	if w := e.callback("github", okQuery(s), nil); location(w) != "/login?error=failed" || newSessionCookie(w) != nil {
		t.Errorf("no cookie: %q", location(w))
	}
	_, s2 := e.start("github", oauthOrigin, "")
	other := &http.Cookie{Name: "dtw_oauth", Value: "someone-elses-cookie"}
	if w := e.callback("github", okQuery(s2), other); location(w) != "/login?error=failed" || newSessionCookie(w) != nil {
		t.Errorf("wrong cookie: %q", location(w))
	}
	if e.store.sessions != 0 || len(e.store.idents) != 0 {
		t.Errorf("a session or identity was created: %d %d", e.store.sessions, len(e.store.idents))
	}
}

func TestOAuthLoginExpires(t *testing.T) {
	e := newOAuthEnv(t)
	e.store.expireNow = true
	_, s := e.start("github", oauthOrigin, "")
	if w := e.callback("github", okQuery(s), s.cookie); location(w) != "/login?error=expired" {
		t.Errorf("expired login: %q", location(w))
	}
}

func TestOAuthStartNeedsOurOrigin(t *testing.T) {
	e := newOAuthEnv(t)
	for _, origin := range []string{"", "https://evil.example"} {
		if w, _ := e.start("github", origin, ""); w.Code != http.StatusForbidden {
			t.Errorf("origin %q: %d", origin, w.Code)
		}
	}
	if len(e.store.logins) != 0 {
		t.Error("a login was started from another site")
	}
}

func TestOAuthNeverRedirectsOffSite(t *testing.T) {
	for _, target := range []string{"//evil.example", "https://evil.example/x", "/\\evil.example", "javascript:alert(1)", "/ok\r\nSet-Cookie: x=1"} {
		e := newOAuthEnv(t)
		body, _ := json.Marshal(map[string]string{"redirect": target})
		_, s := e.start("github", oauthOrigin, string(body))
		if w := e.callback("github", okQuery(s), s.cookie); location(w) != "/" {
			t.Errorf("redirect %q → %q", target, location(w))
		}
	}
}

func TestOAuthRefusesNewGitHubAccounts(t *testing.T) {
	e := newOAuthEnv(t)
	e.prov.createdAt = time.Now().Add(-10 * 24 * time.Hour)
	_, s := e.start("github", oauthOrigin, `{"redirect":"/me/api-key"}`)
	w := e.callback("github", okQuery(s), s.cookie)
	if !strings.HasPrefix(location(w), "/login?error=too_new") || len(e.store.users) != 0 {
		t.Errorf("young account: %q, users %d", location(w), len(e.store.users))
	}
}

func TestOAuthUserCancels(t *testing.T) {
	e := newOAuthEnv(t)
	_, s := e.start("google", oauthOrigin, "")
	w := e.callback("google", url.Values{"error": {"access_denied"}, "state": {s.state}}.Encode(), s.cookie)
	if location(w) != "/login?error=cancelled" || len(e.store.logins) != 0 {
		t.Errorf("cancel: %q, pending logins %d", location(w), len(e.store.logins))
	}
}

func TestOAuthDisabledAccountCannotSignIn(t *testing.T) {
	e := newOAuthEnv(t)
	e.store.disabled["github|4242"] = true
	_, s := e.start("github", oauthOrigin, "")
	if w := e.callback("github", okQuery(s), s.cookie); location(w) != "/login?error=disabled" || newSessionCookie(w) != nil {
		t.Errorf("disabled: %q", location(w))
	}
}

func TestOAuthProviderErrors(t *testing.T) {
	e := newOAuthEnv(t)
	_, s := e.start("github", oauthOrigin, "")
	bad := url.Values{"code": {"stolen-or-wrong-code"}, "state": {s.state}}.Encode()
	if w := e.callback("github", bad, s.cookie); location(w) != "/login?error=failed" {
		t.Errorf("bad code: %q", location(w))
	}
	if w := e.callback("nope", okQuery(s), s.cookie); location(w) != "/login?error=unavailable" {
		t.Errorf("unknown provider: %q", location(w))
	}
}

func TestOAuthGoogleKeepsOnlyVerifiedEmail(t *testing.T) {
	e := newOAuthEnv(t)
	e.prov.emailVerify = false
	_, s := e.start("google", oauthOrigin, "")
	if s.scope != "openid email profile" {
		t.Errorf("google scope = %q", s.scope)
	}
	if w := e.callback("google", okQuery(s), s.cookie); newSessionCookie(w) == nil {
		t.Fatalf("google sign-in: %q", location(w))
	}
	if id := e.store.idents[0]; id.Subject != "google-sub-1" || id.Email != "" {
		t.Errorf("identity = %+v (unverified email must not be kept)", id)
	}
}

func TestProvidersListsRealOnesWithoutDevInProduction(t *testing.T) {
	ps := auth.Providers(true, true, auth.GitHub("a", "b"), auth.Google("c", "d"))
	if len(ps) != 2 || ps[0].ID != "github" || ps[1].ID != "google" {
		t.Errorf("providers = %+v", ps)
	}
	if got := auth.ConfiguredOAuth("id", "", "", ""); len(got) != 0 {
		t.Errorf("a provider without its secret was enabled: %v", got)
	}
}
