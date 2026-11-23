package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/component"
	"dashboard.tw/backend/internal/opendata"
	"dashboard.tw/backend/internal/screen"
	"dashboard.tw/backend/internal/testdb"
)

func TestRedirectURIRules(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://chat.example/api/mcp/auth_callback":             true,
		"http://127.0.0.1:33418/callback":                        true,
		"http://localhost/cb":                                    true,
		"http://[::1]:8080/cb":                                   true,
		"cursor://anysphere.cursor-retrieval/oauth/callback":     true,
		"vscode://vscode.github-authentication/did-authenticate": true,
		"http://example.com/cb":                                  false,
		"javascript:alert(1)":                                    false,
		"data:text/html,hi":                                      false,
		"https://example.com/cb#frag":                            false,
		"https://user:pw@example.com/cb":                         false,
		"":                                                       false,
		"/relative":                                              false,
	} {
		if got := validRedirectURI(raw); got != want {
			t.Errorf("validRedirectURI(%q) = %v", raw, got)
		}
	}
	reg := []string{"http://127.0.0.1/callback", "https://app.example/cb"}
	for given, want := range map[string]bool{
		"http://127.0.0.1:51234/callback": true,
		"http://127.0.0.1:51234/other":    false,
		"https://app.example/cb":          true,
		"https://app.example:444/cb":      false,
		"http://localhost:51234/callback": false,
	} {
		if got := redirectMatches(reg, given); got != want {
			t.Errorf("redirectMatches(%q) = %v", given, got)
		}
	}
	for _, addr := range []string{"127.0.0.1:443", "10.0.0.8:443", "172.16.5.1:443", "169.254.169.254:80", "[::1]:443", "100.64.0.1:443"} {
		if blockPrivate("tcp", addr, nil) == nil {
			t.Errorf("%s should be blocked", addr)
		}
	}
	if err := blockPrivate("tcp", "1.1.1.1:443", nil); err != nil {
		t.Errorf("public address blocked: %v", err)
	}
}

func TestClientMetadataDocument(t *testing.T) {
	var body string
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer ts.Close()
	id := ts.URL + "/oauth/client.json"
	body = fmt.Sprintf(`{"client_id":%q,"client_name":"Test Tool","redirect_uris":["http://127.0.0.1/cb"]}`, id)
	c, err := fetchClientDoc(context.Background(), ts.Client(), id)
	if err != nil || c.Name != "Test Tool" || c.AuthMethod != "none" {
		t.Fatalf("doc = %+v, %v", c, err)
	}
	body = `{"client_id":"https://evil.example/x","redirect_uris":["http://127.0.0.1/cb"]}`
	if _, err := fetchClientDoc(context.Background(), ts.Client(), id); err == nil {
		t.Error("a mismatched client_id was accepted")
	}
	body = fmt.Sprintf(`{"client_id":%q,"redirect_uris":["javascript:alert(1)"]}`, id)
	if _, err := fetchClientDoc(context.Background(), ts.Client(), id); err == nil {
		t.Error("a bad redirect_uri was accepted")
	}
	if _, err := fetchClientDoc(context.Background(), metadataHTTP, id); err == nil {
		t.Error("a loopback metadata URL was fetched")
	}
}

func pkcePair() (string, string) {
	verifier := strings.Repeat("abcdefghij", 5)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func TestMCPOAuth(t *testing.T) {
	e := newOpenEnv(t)
	scr := screen.New(e.store.DB, &component.Executor{DB: testdb.Reader(t, e.db)})
	e.router = NewRouter(Deps{DB: fakePinger{}, OpenData: &opendata.Service{DB: e.store.DB}, Screen: scr,
		RatePerMinute: 6000, RateBurst: 1000,
		Auth: AuthDeps{Store: e.store, Providers: auth.Providers(false, false), Origins: []string{testOrigin}}})
	srv := httptest.NewServer(e.router)
	defer srv.Close()

	do := func(method, path, contentType, body, cookie string) (int, map[string]any, http.Header) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: "dtw_session", Value: cookie})
			req.Header.Set("Origin", testOrigin)
		}
		w := httptest.NewRecorder()
		e.router.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out, w.Header()
	}
	form := func(path string, v url.Values) (int, map[string]any) {
		t.Helper()
		code, out, _ := do("POST", path, "application/x-www-form-urlencoded", v.Encode(), "")
		return code, out
	}

	if _, prm, _ := do("GET", "/.well-known/oauth-protected-resource/mcp", "", "", ""); prm["resource"] != testOrigin+"/mcp" {
		t.Fatalf("resource metadata = %v", prm)
	}
	_, asm, _ := do("GET", "/.well-known/oauth-authorization-server", "", "", "")
	if asm["issuer"] != testOrigin || fmt.Sprint(asm["code_challenge_methods_supported"]) != "[S256]" ||
		asm["client_id_metadata_document_supported"] != true {
		t.Fatalf("server metadata = %v", asm)
	}
	if _, oidc, _ := do("GET", "/.well-known/openid-configuration", "", "", ""); oidc["token_endpoint"] != testOrigin+"/oauth/token" {
		t.Errorf("openid configuration = %v", oidc)
	}
	_, _, h := do("POST", "/mcp", "application/json", `{}`, "")
	if !strings.Contains(h.Get("WWW-Authenticate"), `resource_metadata="`+testOrigin+`/.well-known/oauth-protected-resource/mcp"`) {
		t.Errorf("challenge = %q", h.Get("WWW-Authenticate"))
	}

	if code, _, _ := do("POST", "/oauth/register", "application/json", `{"redirect_uris":["javascript:alert(1)"]}`, ""); code != 400 {
		t.Errorf("bad redirect registered: %d", code)
	}
	redirect := "http://127.0.0.1/callback"
	code, reg, _ := do("POST", "/oauth/register", "application/json",
		`{"client_name":"測試 AI","redirect_uris":["`+redirect+`"],"token_endpoint_auth_method":"none"}`, "")
	clientID, _ := reg["client_id"].(string)
	if code != 201 || clientID == "" || reg["client_secret"] != nil {
		t.Fatalf("register = %d %v", code, reg)
	}

	session := e.signInAs(t, auth.RoleUser, "active")
	verifier, challenge := pkcePair()
	cb := "http://127.0.0.1:41234/callback"
	q := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {cb},
		"code_challenge": {challenge}, "code_challenge_method": {"S256"}, "state": {"xyz"},
		"resource": {testOrigin + "/mcp"}, "scope": {"mcp"}}
	_, info, _ := do("GET", "/api/v1/oauth/authorize?"+q.Encode(), "", "", session)
	if data, _ := info["data"].(map[string]any); data == nil || data["redirect_host"] != "本機程式" {
		t.Fatalf("authorize info = %v", info)
	}
	approve := func(query url.Values, yes bool) *url.URL {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"query": "?" + query.Encode(), "approve": yes})
		code, out, _ := do("POST", "/api/v1/oauth/authorize", "application/json", string(body), session)
		data, _ := out["data"].(map[string]any)
		u, err := url.Parse(fmt.Sprint(data["redirect"]))
		if code != 200 || err != nil || !strings.HasPrefix(u.String(), cb) {
			t.Fatalf("approve = %d %v", code, out)
		}
		return u
	}
	if code, _, _ := do("POST", "/api/v1/oauth/authorize", "application/json", `{"query":"","approve":true}`, ""); code != 401 && code != 403 {
		t.Errorf("approve without a session: %d", code)
	}
	if denied := approve(q, false); denied.Query().Get("error") != "access_denied" || denied.Query().Get("state") != "xyz" {
		t.Errorf("deny = %s", denied)
	}

	first := approve(q, true)
	if first.Query().Get("state") != "xyz" || first.Query().Get("iss") != testOrigin {
		t.Errorf("approve redirect = %s", first)
	}
	exchange := func(code, verifier string) (int, map[string]any) {
		return form("/oauth/token", url.Values{"grant_type": {"authorization_code"}, "code": {code},
			"redirect_uri": {cb}, "client_id": {clientID}, "code_verifier": {verifier}, "resource": {testOrigin + "/mcp"}})
	}
	if code, out := exchange(first.Query().Get("code"), strings.Repeat("x", 50)); code != 400 || out["error"] != "invalid_grant" {
		t.Errorf("wrong verifier: %d %v", code, out)
	}
	if code, _ := exchange(first.Query().Get("code"), verifier); code != 400 {
		t.Errorf("a used code was accepted: %d", code)
	}
	second := approve(q, true)
	code, tok := exchange(second.Query().Get("code"), verifier)
	access, _ := tok["access_token"].(string)
	refresh, _ := tok["refresh_token"].(string)
	if code != 200 || access == "" || refresh == "" || tok["token_type"] != "Bearer" {
		t.Fatalf("token = %d %v", code, tok)
	}

	callMCP := func(token string) (*mcp.CallToolResult, error) {
		hc := &http.Client{Transport: headerTransport{value: token, base: http.DefaultTransport}}
		c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
		sess, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: hc}, nil)
		if err != nil {
			return nil, err
		}
		defer sess.Close()
		return sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_latest", Arguments: map[string]any{"dataset": "aqi_obs", "limit": 1}})
	}
	if r, err := callMCP(access); err != nil || r.IsError {
		t.Fatalf("mcp with oauth token: %v %+v", err, r)
	}
	if used, err := e.store.UsedToday(context.Background(), userIDOf(t, e, session)); err != nil || used.Requests != 1 {
		t.Errorf("oauth usage = %+v %v", used, err)
	}
	req := httptest.NewRequest("GET", "/api/v1/datasets/aqi_obs/latest", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("oauth token on the REST API: %d", w.Code)
	}

	code, next := form("/oauth/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}})
	if code != 200 || next["refresh_token"] == refresh || next["access_token"] == nil {
		t.Fatalf("refresh = %d %v", code, next)
	}
	if code, _ := form("/oauth/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}}); code != 400 {
		t.Errorf("an old refresh token was accepted: %d", code)
	}

	_, apps, _ := do("GET", "/api/v1/me/oauth-apps", "", "", session)
	list, _ := apps["data"].([]any)
	if len(list) != 1 {
		t.Fatalf("oauth apps = %v", apps)
	}
	_, clients, _ := do("GET", "/api/v1/me/api-clients", "", "", session)
	if raw, _ := json.Marshal(clients); strings.Contains(string(raw), "測試 AI") {
		t.Errorf("oauth grant listed as an API credential: %s", raw)
	}
	id := fmt.Sprint(list[0].(map[string]any)["id"])
	if code, _, _ := do("DELETE", "/api/v1/me/oauth-apps/"+id, "", "", session); code != 204 {
		t.Fatalf("revoke = %d", code)
	}
	if _, err := callMCP(next["access_token"].(string)); err == nil {
		t.Error("a revoked grant still works")
	}
	if code, _ := form("/oauth/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {fmt.Sprint(next["refresh_token"])}, "client_id": {clientID}}); code != 400 {
		t.Errorf("refresh after revoke: %d", code)
	}

	bad := url.Values{}
	for k, v := range q {
		bad[k] = v
	}
	bad.Set("redirect_uri", "https://evil.example/cb")
	_, info, _ = do("GET", "/api/v1/oauth/authorize?"+bad.Encode(), "", "", session)
	if data, _ := info["data"].(map[string]any); data == nil || data["error"] == nil || data["redirect"] != nil {
		t.Errorf("unregistered redirect: %v", info)
	}
	bad = url.Values{}
	for k, v := range q {
		bad[k] = v
	}
	bad.Del("code_challenge")
	_, info, _ = do("GET", "/api/v1/oauth/authorize?"+bad.Encode(), "", "", session)
	if data, _ := info["data"].(map[string]any); data == nil || !strings.Contains(fmt.Sprint(data["redirect"]), "error=invalid_request") {
		t.Errorf("missing PKCE: %v", info)
	}
}
