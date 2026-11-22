package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"dashboard.tw/backend/internal/auth"
)

func (e *oauthEnv) callbackWith(provider, query string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/auth/"+provider+"/callback?"+query, nil)
	for _, c := range cookies {
		if c != nil {
			req.AddCookie(c)
		}
	}
	e.r.ServeHTTP(w, req)
	return w
}

func (e *oauthEnv) request(method, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Origin", oauthOrigin)
	for _, c := range cookies {
		if c != nil {
			req.AddCookie(c)
		}
	}
	e.r.ServeHTTP(w, req)
	return w
}

func (e *oauthEnv) signIn(provider string, extra ...*http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	_, s := e.start(provider, oauthOrigin, "")
	if s == nil {
		e.t.Fatalf("start %s failed", provider)
	}
	return e.callbackWith(provider, okQuery(s), append([]*http.Cookie{s.cookie}, extra...)...)
}

func linkCookie(w *httptest.ResponseRecorder) (set, cleared *http.Cookie) {
	for _, c := range w.Result().Cookies() {
		if c.Name == "dtw_link" {
			if c.MaxAge > 0 {
				set = c
			} else {
				cleared = c
			}
		}
	}
	return
}

func newLinkEnv(t *testing.T) (*oauthEnv, int64) {
	e := newOAuthEnv(t)
	e.prov.googleEmail = "octo@example.com"
	if w := e.signIn("github"); newSessionCookie(w) == nil {
		t.Fatalf("GitHub sign-in: %d %q", w.Code, location(w))
	}
	return e, e.store.users["github|4242"].ID
}

func TestLinkSameEmailAfterConfirming(t *testing.T) {
	e, owner := newLinkEnv(t)

	w := e.signIn("google")
	if w.Code != http.StatusSeeOther || location(w) != "/login?error=link" {
		t.Fatalf("Google sign-in = %d %q, want /login?error=link", w.Code, location(w))
	}
	if newSessionCookie(w) != nil {
		t.Fatal("signed in to the existing account on the email alone")
	}
	if len(e.store.users) != 1 {
		t.Fatalf("users = %d, a second account was created", len(e.store.users))
	}
	link, _ := linkCookie(w)
	if link == nil || !link.HttpOnly || link.SameSite != http.SameSiteLaxMode {
		t.Fatalf("link cookie = %+v", link)
	}
	for k := range e.store.pending {
		if k == link.Value {
			t.Error("pending link token stored in plain text")
		}
	}

	info := e.request("GET", "/api/v1/auth/link", link)
	var got struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(info.Body.Bytes(), &got)
	if info.Code != http.StatusOK || got.Data["provider"] != "google" || len(got.Data["existing"].([]any)) != 1 ||
		got.Data["existing"].([]any)[0] != "github" || len(got.Data) != 3 {
		t.Fatalf("link info = %d %s", info.Code, info.Body)
	}

	w = e.signIn("github", link)
	if newSessionCookie(w) == nil || location(w) != "/me?linked=google" {
		t.Fatalf("confirming sign-in = %q", location(w))
	}
	if _, cleared := linkCookie(w); cleared == nil {
		t.Error("link cookie not cleared")
	}
	if u := e.store.users["google|google-sub-1"]; u == nil || u.ID != owner {
		t.Fatalf("Google identity = %+v, want user %d", u, owner)
	}
	if e.store.audits[len(e.store.audits)-1] != "identity.link" && e.store.audits[len(e.store.audits)-2] != "identity.link" {
		t.Errorf("audits = %v", e.store.audits)
	}

	if w := e.signIn("google"); newSessionCookie(w) == nil || location(w) != "/" {
		t.Errorf("Google after linking = %q", location(w))
	}
	if len(e.store.users) != 2 || e.store.nextID != 1 {
		t.Errorf("identities %d, accounts %d", len(e.store.users), e.store.nextID)
	}
}

func TestLinkNeedsTheOwnerInTheSameBrowser(t *testing.T) {
	e, _ := newLinkEnv(t)
	w := e.signIn("google")
	link, _ := linkCookie(w)

	if w := e.signIn("github"); location(w) != "/" {
		t.Errorf("sign-in without the link cookie = %q", location(w))
	}
	if e.store.users["google|google-sub-1"] != nil {
		t.Fatal("linked without the browser that started it")
	}

	e.prov.githubID, e.prov.emailVerify = 5000, false
	if w := e.signIn("github", link); location(w) != "/" {
		t.Errorf("another account with the cookie = %q", location(w))
	}
	if e.store.users["google|google-sub-1"] != nil {
		t.Fatal("linked to an account that is not the email's owner")
	}
	if len(e.store.pending) != 0 {
		t.Error("pending link kept after the wrong account signed in")
	}
	if e.store.users["github|5000"] == nil || e.store.users["github|5000"].ID == e.store.users["github|4242"].ID {
		t.Error("the other GitHub account should be its own account")
	}
}

func TestLinkExpires(t *testing.T) {
	e, _ := newLinkEnv(t)
	link, _ := linkCookie(e.signIn("google"))
	for k, l := range e.store.pending {
		l.ExpiresAt = time.Now().Add(-time.Second)
		e.store.pending[k] = l
	}
	if w := e.request("GET", "/api/v1/auth/link", link); w.Code != http.StatusNotFound {
		t.Errorf("expired link info: %d", w.Code)
	}
	if w := e.signIn("github", link); location(w) != "/" || e.store.users["google|google-sub-1"] != nil {
		t.Errorf("expired link was used: %q", location(w))
	}
}

func TestLinkCanBeCancelled(t *testing.T) {
	e, _ := newLinkEnv(t)
	link, _ := linkCookie(e.signIn("google"))
	if w := e.request("DELETE", "/api/v1/auth/link", link); w.Code != http.StatusNoContent {
		t.Fatalf("cancel: %d", w.Code)
	}
	if w := e.request("GET", "/api/v1/auth/link", link); w.Code != http.StatusNotFound {
		t.Errorf("link info after cancel: %d", w.Code)
	}
	if w := e.request("GET", "/api/v1/auth/link"); w.Code != http.StatusNotFound {
		t.Errorf("link info without cookie: %d", w.Code)
	}
	link, _ = linkCookie(e.signIn("google"))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/v1/auth/link", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.AddCookie(link)
	e.r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden || len(e.store.pending) != 1 {
		t.Errorf("cross-site cancel: %d, pending %d", w.Code, len(e.store.pending))
	}
}

func (e *oauthEnv) startLink(provider, origin string, session *http.Cookie) (*httptest.ResponseRecorder, *started) {
	e.t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/"+provider+"/link", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if session != nil {
		req.AddCookie(session)
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
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	u, err := url.Parse(resp.Data.RedirectURL)
	if err != nil {
		e.t.Fatal(err)
	}
	q := u.Query()
	s := &started{state: q.Get("state"), challenge: q.Get("code_challenge"), redirectURI: q.Get("redirect_uri"), authURL: u}
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

func newAccountLinkEnv(t *testing.T) (*oauthEnv, *http.Cookie, int64) {
	e := newOAuthEnv(t)
	e.prov.googleEmail = "different@example.com"
	session := newSessionCookie(e.signIn("github"))
	if session == nil {
		t.Fatal("GitHub sign-in failed")
	}
	return e, session, e.store.users["github|4242"].ID
}

func TestLinkFromAccountPage(t *testing.T) {
	e, session, owner := newAccountLinkEnv(t)
	sessions := e.store.sessions

	_, s := e.startLink("google", oauthOrigin, session)
	if s == nil || s.cookie == nil {
		t.Fatal("start link failed")
	}
	w := e.callbackWith("google", okQuery(s), s.cookie, session)
	if w.Code != http.StatusSeeOther || location(w) != "/me?linked=google" {
		t.Fatalf("link callback = %d %q", w.Code, location(w))
	}
	if u := e.store.users["google|google-sub-1"]; u == nil || u.ID != owner {
		t.Fatalf("Google identity = %+v, want user %d", u, owner)
	}
	if newSessionCookie(w) != nil || e.store.sessions != sessions || e.store.nextID != 1 {
		t.Errorf("linking signed in again or created an account (sessions %d→%d, accounts %d)", sessions, e.store.sessions, e.store.nextID)
	}
	if w := e.signIn("google"); newSessionCookie(w) == nil || e.store.nextID != 1 {
		t.Errorf("Google after linking = %q, accounts %d", location(w), e.store.nextID)
	}
}

func TestLinkFromAccountNeedsSignInAndOurSite(t *testing.T) {
	e, session, _ := newAccountLinkEnv(t)
	if w, _ := e.startLink("google", oauthOrigin, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("start link signed out: %d, want 401", w.Code)
	}
	if w, _ := e.startLink("google", "https://evil.example", session); w.Code != http.StatusForbidden {
		t.Errorf("start link from another site: %d, want 403", w.Code)
	}
	if w, _ := e.startLink("nope", oauthOrigin, session); w.Code != http.StatusNotFound {
		t.Errorf("unknown provider: %d", w.Code)
	}
	_, s := e.startLink("google", oauthOrigin, session)
	if w := e.callbackWith("google", okQuery(s), s.cookie); location(w) != "/me?link_error=signed_out" {
		t.Errorf("callback signed out = %q", location(w))
	}
	if e.store.users["google|google-sub-1"] != nil {
		t.Fatal("linked without the account being signed in")
	}
}

func TestLinkFromAccountOtherBrowserOrCancel(t *testing.T) {
	e, session, _ := newAccountLinkEnv(t)
	_, s := e.startLink("google", oauthOrigin, session)
	if w := e.callbackWith("google", okQuery(s), session); location(w) != "/me?link_error=failed" {
		t.Errorf("callback in another browser = %q", location(w))
	}
	_, s = e.startLink("google", oauthOrigin, session)
	if w := e.callbackWith("google", "error=access_denied&state="+s.state, s.cookie, session); location(w) != "/me?link_error=cancelled" {
		t.Errorf("cancelled = %q", location(w))
	}
	if e.store.users["google|google-sub-1"] != nil {
		t.Error("linked anyway")
	}
}

func TestLinkFromAccountRefusesTakenOrDuplicate(t *testing.T) {
	e, session, owner := newAccountLinkEnv(t)
	e.store.nextID++
	e.store.users["google|google-sub-1"] = &auth.User{ID: e.store.nextID}
	_, s := e.startLink("google", oauthOrigin, session)
	if w := e.callbackWith("google", okQuery(s), s.cookie, session); location(w) != "/me?link_error=taken" {
		t.Errorf("taken = %q", location(w))
	}
	if e.store.users["google|google-sub-1"].ID == owner {
		t.Fatal("moved another account's Google sign-in")
	}
	e.prov.githubID = 5000
	_, s = e.startLink("github", oauthOrigin, session)
	if w := e.callbackWith("github", okQuery(s), s.cookie, session); location(w) != "/me?link_error=provider" {
		t.Errorf("second GitHub = %q", location(w))
	}
	if e.store.users["github|5000"] != nil {
		t.Error("a second GitHub account was linked")
	}
}

func TestUnverifiedEmailNeverLinks(t *testing.T) {
	e, _ := newLinkEnv(t)
	e.prov.emailVerify = false
	if w := e.signIn("google"); newSessionCookie(w) == nil || location(w) != "/" {
		t.Fatalf("unverified Google email = %q", location(w))
	}
	if _, set := linkCookie(e.signIn("google")); set != nil {
		t.Error("an unverified email started a link")
	}
	if u := e.store.users["google|google-sub-1"]; u == nil || u.ID == e.store.users["github|4242"].ID {
		t.Error("unverified email reached the GitHub account")
	}
}
