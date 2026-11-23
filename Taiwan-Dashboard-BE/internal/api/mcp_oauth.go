package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/auth"
)

type oauthServer struct {
	store *auth.Store
	site  string
	fetch func(ctx context.Context, id string) (*auth.OAuthClient, error)
}

type oauthError struct {
	Code        string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

func (e *oauthError) Error() string { return e.Code + ": " + e.Description }

func oauthErr(code, desc string) *oauthError { return &oauthError{Code: code, Description: desc} }

func oauthCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version")
		h.Set("Access-Control-Max-Age", "86400")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func registerMCPOAuth(r *gin.Engine, v1 *gin.RouterGroup, a AuthDeps, site string) *oauthServer {
	o := &oauthServer{store: a.Store, site: site}
	o.fetch = o.fetchMetadataDoc

	meta := r.Group("", oauthCORS())
	for _, p := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp",
		"/.well-known/oauth-protected-resource/mcp/sse"} {
		meta.GET(p, o.resourceMetadata)
		meta.OPTIONS(p)
	}
	for _, p := range []string{"/.well-known/oauth-authorization-server", "/.well-known/openid-configuration",
		"/.well-known/oauth-authorization-server/mcp", "/.well-known/openid-configuration/mcp", "/mcp/.well-known/openid-configuration"} {
		meta.GET(p, o.serverMetadata)
		meta.OPTIONS(p)
	}

	ep := r.Group("/oauth", oauthCORS(), private())
	ep.OPTIONS("/register")
	ep.OPTIONS("/token")
	ep.OPTIONS("/revoke")
	ep.POST("/register", newLimiter(10, 10).middleware(), o.register)
	ep.POST("/token", newLimiter(120, 60).middleware(), o.token)
	ep.POST("/revoke", newLimiter(120, 60).middleware(), o.revoke)

	g := v1.Group("/oauth", private(), newLimiter(60, 30).middleware(), a.sameOrigin(), a.session())
	g.GET("/authorize", o.authorizeInfo)
	g.POST("/authorize", requireUser(), o.authorizeDecide)
	return o
}

func (o *oauthServer) resourceURL() string { return o.site + "/mcp" }

func (o *oauthServer) resourceMetadata(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(http.StatusOK, gin.H{
		"resource":                 o.resourceURL(),
		"authorization_servers":    []string{o.site},
		"scopes_supported":         []string{auth.OAuthScope},
		"bearer_methods_supported": []string{"header"},
		"resource_name":            "dashboard.tw MCP",
		"resource_documentation":   o.site + "/developers",
	})
}

func (o *oauthServer) serverMetadata(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(http.StatusOK, gin.H{
		"issuer":                                         o.site,
		"authorization_endpoint":                         o.site + "/oauth/authorize",
		"token_endpoint":                                 o.site + "/oauth/token",
		"registration_endpoint":                          o.site + "/oauth/register",
		"revocation_endpoint":                            o.site + "/oauth/revoke",
		"scopes_supported":                               []string{auth.OAuthScope},
		"response_types_supported":                       []string{"code"},
		"response_modes_supported":                       []string{"query"},
		"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none", "client_secret_post", "client_secret_basic"},
		"revocation_endpoint_auth_methods_supported":     []string{"none", "client_secret_post", "client_secret_basic"},
		"client_id_metadata_document_supported":          true,
		"authorization_response_iss_parameter_supported": true,
		"service_documentation":                          o.site + "/developers",
	})
}

var badSchemes = []string{"javascript", "data", "vbscript", "file", "about", "blob", "ftp", "ws", "wss", "mailto", "tel", "chrome", "intent"}

func loopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "[::1]" || host == "::1"
}

func validRedirectURI(raw string) bool {
	if raw == "" || len(raw) > 500 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || strings.Contains(raw, "#") || u.User != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	switch {
	case scheme == "https":
		return u.Host != ""
	case scheme == "http":
		return loopbackHost(u.Hostname())
	case scheme == "" || slices.Contains(badSchemes, scheme):
		return false
	}
	for _, r := range scheme {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}

func redirectMatches(registered []string, given string) bool {
	if slices.Contains(registered, given) {
		return true
	}
	g, err := url.Parse(given)
	if err != nil || g.Scheme != "http" || !loopbackHost(g.Hostname()) {
		return false
	}
	for _, r := range registered {
		u, err := url.Parse(r)
		if err == nil && u.Scheme == "http" && u.Hostname() == g.Hostname() && u.Path == g.Path && u.RawQuery == g.RawQuery {
			return true
		}
	}
	return false
}

func (o *oauthServer) register(c *gin.Context) {
	var in struct {
		RedirectURIs []string `json:"redirect_uris"`
		ClientName   string   `json:"client_name"`
		ClientURI    string   `json:"client_uri"`
		AuthMethod   string   `json:"token_endpoint_auth_method"`
		GrantTypes   []string `json:"grant_types"`
		Response     []string `json:"response_types"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if err := json.NewDecoder(c.Request.Body).Decode(&in); err != nil {
		c.JSON(http.StatusBadRequest, oauthErr("invalid_client_metadata", "request body must be JSON"))
		return
	}
	if len(in.RedirectURIs) == 0 || len(in.RedirectURIs) > 10 {
		c.JSON(http.StatusBadRequest, oauthErr("invalid_redirect_uri", "give 1 to 10 redirect_uris"))
		return
	}
	for _, u := range in.RedirectURIs {
		if !validRedirectURI(u) {
			c.JSON(http.StatusBadRequest, oauthErr("invalid_redirect_uri", "redirect_uri not allowed: "+u))
			return
		}
	}
	if in.AuthMethod == "" {
		in.AuthMethod = "none"
	}
	if !slices.Contains([]string{"none", "client_secret_post", "client_secret_basic"}, in.AuthMethod) {
		c.JSON(http.StatusBadRequest, oauthErr("invalid_client_metadata", "unsupported token_endpoint_auth_method"))
		return
	}
	if len(in.GrantTypes) == 0 {
		in.GrantTypes = []string{"authorization_code", "refresh_token"}
	}
	for _, g := range in.GrantTypes {
		if g != "authorization_code" && g != "refresh_token" {
			c.JSON(http.StatusBadRequest, oauthErr("invalid_client_metadata", "unsupported grant_type: "+g))
			return
		}
	}
	for _, rt := range in.Response {
		if rt != "code" {
			c.JSON(http.StatusBadRequest, oauthErr("invalid_client_metadata", "unsupported response_type: "+rt))
			return
		}
	}
	name := strings.TrimSpace(in.ClientName)
	if name == "" {
		name = hostOf(in.RedirectURIs[0])
	}
	clientURI := ""
	if u, err := url.Parse(in.ClientURI); err == nil && u.Scheme == "https" && u.Host != "" && len(in.ClientURI) <= 500 {
		clientURI = in.ClientURI
	}
	cl, secret, err := o.store.RegisterOAuthClient(c.Request.Context(), auth.OAuthClient{
		Name: name, ClientURI: clientURI, RedirectURIs: in.RedirectURIs, AuthMethod: in.AuthMethod})
	if err != nil {
		slog.Error("register oauth client", "error", err)
		c.JSON(http.StatusInternalServerError, oauthErr("server_error", ""))
		return
	}
	out := gin.H{
		"client_id":                  cl.ID,
		"client_id_issued_at":        time.Now().Unix(),
		"client_name":                cl.Name,
		"redirect_uris":              cl.RedirectURIs,
		"token_endpoint_auth_method": cl.AuthMethod,
		"grant_types":                in.GrantTypes,
		"response_types":             []string{"code"},
		"scope":                      auth.OAuthScope,
	}
	if clientURI != "" {
		out["client_uri"] = clientURI
	}
	if secret != "" {
		out["client_secret"] = secret
		out["client_secret_expires_at"] = 0
	}
	c.JSON(http.StatusCreated, out)
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Host != "" {
		return u.Hostname()
	}
	return u.Scheme
}

func isMetadataURL(id string) bool {
	u, err := url.Parse(id)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.Path != "" && u.Path != "/" &&
		u.Fragment == "" && u.User == nil && !strings.Contains(u.Path, "/./") && !strings.Contains(u.Path, "/../")
}

func (o *oauthServer) client(ctx context.Context, id string) (*auth.OAuthClient, error) {
	if !strings.HasPrefix(id, "https://") {
		return o.store.OAuthClientByID(ctx, id)
	}
	if !isMetadataURL(id) || len(id) > 500 {
		return nil, auth.ErrOAuthClient
	}
	cached, err := o.store.OAuthClientByID(ctx, id)
	if err != nil && !errors.Is(err, auth.ErrOAuthClient) {
		return nil, err
	}
	if cached != nil && cached.FetchedAt != nil && time.Since(*cached.FetchedAt) < auth.MetadataDocTTL {
		return cached, nil
	}
	fresh, ferr := o.fetch(ctx, id)
	if ferr != nil {
		slog.Warn("fetch client metadata", "client", id, "error", ferr)
		if cached != nil && cached.FetchedAt != nil && time.Since(*cached.FetchedAt) < 7*auth.MetadataDocTTL {
			return cached, nil
		}
		return nil, auth.ErrOAuthClient
	}
	if err := o.store.SaveMetadataClient(ctx, *fresh); err != nil {
		return nil, err
	}
	return o.store.OAuthClientByID(ctx, id)
}

func blockPrivate(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() ||
		(ip.To4() != nil && ip.To4()[0] == 100 && ip.To4()[1]&0xc0 == 64) {
		return fmt.Errorf("address %s is not allowed", host)
	}
	return nil
}

var metadataHTTP = &http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 3 * time.Second, Control: blockPrivate}).DialContext,
		TLSHandshakeTimeout:   3 * time.Second,
		ResponseHeaderTimeout: 4 * time.Second,
		MaxIdleConns:          10,
	},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func (o *oauthServer) fetchMetadataDoc(ctx context.Context, id string) (*auth.OAuthClient, error) {
	return fetchClientDoc(ctx, metadataHTTP, id)
}

func fetchClientDoc(ctx context.Context, hc *http.Client, id string) (*auth.OAuthClient, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, id, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "dashboard.tw-oauth")
	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", res.StatusCode)
	}
	var doc struct {
		ClientID     string   `json:"client_id"`
		ClientName   string   `json:"client_name"`
		ClientURI    string   `json:"client_uri"`
		RedirectURIs []string `json:"redirect_uris"`
		AuthMethod   string   `json:"token_endpoint_auth_method"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&doc); err != nil {
		return nil, err
	}
	if doc.ClientID != id {
		return nil, errors.New("client_id does not match the document URL")
	}
	if doc.AuthMethod != "" && doc.AuthMethod != "none" {
		return nil, errors.New("only public clients are supported")
	}
	if len(doc.RedirectURIs) == 0 || len(doc.RedirectURIs) > 10 {
		return nil, errors.New("bad redirect_uris")
	}
	for _, u := range doc.RedirectURIs {
		if !validRedirectURI(u) {
			return nil, errors.New("redirect_uri not allowed: " + u)
		}
	}
	name := strings.TrimSpace(doc.ClientName)
	if name == "" {
		name = hostOf(id)
	}
	clientURI := ""
	if u, err := url.Parse(doc.ClientURI); err == nil && u.Scheme == "https" && u.Host != "" && len(doc.ClientURI) <= 500 {
		clientURI = doc.ClientURI
	}
	return &auth.OAuthClient{ID: id, Name: name, ClientURI: clientURI, RedirectURIs: doc.RedirectURIs, AuthMethod: "none"}, nil
}

type authorizeReq struct {
	client   *auth.OAuthClient
	redirect string
	state    string
	req      auth.OAuthRequest
}

func (o *oauthServer) validResource(r string) bool {
	r = strings.TrimSuffix(r, "/")
	return r == "" || r == o.site || r == o.site+"/mcp" || r == o.site+"/mcp/sse"
}

func (o *oauthServer) parseAuthorize(ctx context.Context, q url.Values) (*authorizeReq, *oauthError, bool) {
	cl, err := o.client(ctx, q.Get("client_id"))
	if errors.Is(err, auth.ErrOAuthClient) {
		return nil, oauthErr("invalid_client", "找不到此應用，請重新連接"), false
	}
	if err != nil {
		slog.Error("oauth client", "error", err)
		return nil, oauthErr("server_error", "伺服器發生錯誤"), false
	}
	redirect := q.Get("redirect_uri")
	if redirect == "" && len(cl.RedirectURIs) == 1 {
		redirect = cl.RedirectURIs[0]
	}
	if !redirectMatches(cl.RedirectURIs, redirect) {
		return nil, oauthErr("invalid_request", "回傳網址跟這個應用登記的不符"), false
	}
	a := &authorizeReq{client: cl, redirect: redirect, state: q.Get("state")}
	switch {
	case q.Get("response_type") != "code":
		return a, oauthErr("unsupported_response_type", "only response_type=code is supported"), true
	case q.Get("code_challenge") == "" || len(q.Get("code_challenge")) != 43:
		return a, oauthErr("invalid_request", "PKCE code_challenge is required"), true
	case q.Get("code_challenge_method") != "S256":
		return a, oauthErr("invalid_request", "code_challenge_method must be S256"), true
	case len(q.Get("state")) > 500:
		return a, oauthErr("invalid_request", "state is too long"), true
	}
	for _, r := range q["resource"] {
		if !o.validResource(r) {
			return a, oauthErr("invalid_target", "unknown resource"), true
		}
	}
	a.req = auth.OAuthRequest{ClientID: cl.ID, RedirectURI: redirect, CodeChallenge: q.Get("code_challenge"),
		Scope: auth.OAuthScope, Resource: strings.TrimSuffix(q.Get("resource"), "/")}
	return a, nil, false
}

func (o *oauthServer) back(a *authorizeReq, params url.Values) string {
	if a.state != "" {
		params.Set("state", a.state)
	}
	params.Set("iss", o.site)
	sep := "?"
	if strings.Contains(a.redirect, "?") {
		sep = "&"
	}
	return a.redirect + sep + params.Encode()
}

func (o *oauthServer) errorBack(a *authorizeReq, e *oauthError) string {
	p := url.Values{"error": {e.Code}}
	if e.Description != "" {
		p.Set("error_description", e.Description)
	}
	return o.back(a, p)
}

func redirectLabel(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Scheme == "http" && loopbackHost(u.Hostname()) {
		return "本機程式"
	}
	if u.Host == "" {
		return u.Scheme + "://"
	}
	return u.Hostname()
}

func (o *oauthServer) authorizeInfo(c *gin.Context) {
	a, e, redirectable := o.parseAuthorize(c.Request.Context(), c.Request.URL.Query())
	if e != nil {
		out := gin.H{"error": e.Code, "message": e.Description}
		if redirectable {
			out["redirect"] = o.errorBack(a, e)
		}
		writeData(c, http.StatusOK, out, nil)
		return
	}
	verified := ""
	if a.client.MetadataDoc {
		verified = hostOf(a.client.ID)
	}
	writeData(c, http.StatusOK, gin.H{
		"client":        gin.H{"name": a.client.Name, "uri": a.client.ClientURI, "verified_host": verified},
		"redirect_host": redirectLabel(a.redirect),
		"scope":         auth.OAuthScope,
	}, nil)
}

func (o *oauthServer) authorizeDecide(c *gin.Context) {
	var in struct {
		Query   string `json:"query"`
		Approve bool   `json:"approve"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || len(in.Query) > 8000 {
		writeError(c, http.StatusBadRequest, "bad_request", "請求格式不對")
		return
	}
	q, err := url.ParseQuery(strings.TrimPrefix(in.Query, "?"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "請求格式不對")
		return
	}
	a, e, redirectable := o.parseAuthorize(c.Request.Context(), q)
	if e != nil {
		if redirectable {
			writeData(c, http.StatusOK, gin.H{"redirect": o.errorBack(a, e)}, nil)
			return
		}
		writeError(c, http.StatusBadRequest, e.Code, e.Description)
		return
	}
	if !in.Approve {
		writeData(c, http.StatusOK, gin.H{"redirect": o.errorBack(a, oauthErr("access_denied", "the user denied access"))}, nil)
		return
	}
	u := currentUser(c)
	code, err := o.store.CreateOAuthCode(c.Request.Context(), u.ID, a.req)
	if err != nil {
		slog.Error("create oauth code", "error", err)
		writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
		return
	}
	writeData(c, http.StatusOK, gin.H{"redirect": o.back(a, url.Values{"code": {code}})}, nil)
}

func (o *oauthServer) tokenClient(c *gin.Context) (*auth.OAuthClient, *oauthError) {
	id, secret, basic := c.Request.BasicAuth()
	if basic {
		if v, err := url.QueryUnescape(id); err == nil {
			id = v
		}
		if v, err := url.QueryUnescape(secret); err == nil {
			secret = v
		}
	} else {
		id, secret = c.PostForm("client_id"), c.PostForm("client_secret")
	}
	cl, err := o.client(c.Request.Context(), id)
	if errors.Is(err, auth.ErrOAuthClient) {
		return nil, oauthErr("invalid_client", "unknown client")
	}
	if err != nil {
		slog.Error("oauth client", "error", err)
		return nil, oauthErr("server_error", "")
	}
	if !cl.CheckSecret(secret) {
		return nil, oauthErr("invalid_client", "client authentication failed")
	}
	return cl, nil
}

func oauthFail(c *gin.Context, e *oauthError) {
	status := http.StatusBadRequest
	switch e.Code {
	case "invalid_client":
		status = http.StatusUnauthorized
		c.Header("WWW-Authenticate", `Basic realm="dashboard.tw"`)
	case "server_error":
		status = http.StatusInternalServerError
	}
	c.JSON(status, e)
}

func (o *oauthServer) token(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if err := c.Request.ParseForm(); err != nil {
		oauthFail(c, oauthErr("invalid_request", "body must be application/x-www-form-urlencoded"))
		return
	}
	cl, e := o.tokenClient(c)
	if e != nil {
		oauthFail(c, e)
		return
	}
	ctx := c.Request.Context()
	var t *auth.OAuthTokens
	var err error
	switch c.PostForm("grant_type") {
	case "authorization_code":
		res := strings.TrimSuffix(c.PostForm("resource"), "/")
		if !o.validResource(res) {
			oauthFail(c, oauthErr("invalid_target", "unknown resource"))
			return
		}
		t, err = o.store.ExchangeOAuthCode(ctx, cl, c.PostForm("code"), c.PostForm("redirect_uri"), c.PostForm("code_verifier"), res, c.ClientIP())
	case "refresh_token":
		if !o.validResource(c.PostForm("resource")) {
			oauthFail(c, oauthErr("invalid_target", "unknown resource"))
			return
		}
		t, err = o.store.RefreshOAuth(ctx, cl, c.PostForm("refresh_token"))
	default:
		oauthFail(c, oauthErr("unsupported_grant_type", "use authorization_code or refresh_token"))
		return
	}
	if errors.Is(err, auth.ErrOAuthGrant) {
		oauthFail(c, oauthErr("invalid_grant", "the code or refresh token is invalid, expired or already used"))
		return
	}
	if err != nil {
		slog.Error("oauth token", "error", err)
		oauthFail(c, oauthErr("server_error", ""))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"access_token":  t.Access,
		"token_type":    "Bearer",
		"expires_in":    int(auth.OAuthAccessTTL.Seconds()),
		"refresh_token": t.Refresh,
		"scope":         t.Scope,
	})
}

func (o *oauthServer) revoke(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if err := c.Request.ParseForm(); err != nil {
		oauthFail(c, oauthErr("invalid_request", "body must be application/x-www-form-urlencoded"))
		return
	}
	cl, e := o.tokenClient(c)
	if e != nil {
		oauthFail(c, e)
		return
	}
	if err := o.store.RevokeOAuthToken(c.Request.Context(), cl, c.PostForm("token")); err != nil {
		slog.Error("oauth revoke", "error", err)
		oauthFail(c, oauthErr("server_error", ""))
		return
	}
	c.Status(http.StatusOK)
}

func listOAuthApps(a AuthDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		apps, err := a.Store.OAuthApps(c.Request.Context(), currentUser(c).ID)
		if err != nil {
			slog.Error("list oauth apps", "error", err)
			writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
			return
		}
		writeData(c, http.StatusOK, apps, nil)
	}
}

func revokeOAuthApp(a AuthDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			writeError(c, http.StatusNotFound, "not_found", "找不到這個授權")
			return
		}
		err = a.Store.RevokeOAuthApp(c.Request.Context(), currentUser(c).ID, id, c.ClientIP())
		if errors.Is(err, auth.ErrNoOAuthApp) {
			writeError(c, http.StatusNotFound, "not_found", "找不到這個授權")
			return
		}
		if err != nil {
			slog.Error("revoke oauth app", "error", err)
			writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
			return
		}
		c.Status(http.StatusNoContent)
	}
}
