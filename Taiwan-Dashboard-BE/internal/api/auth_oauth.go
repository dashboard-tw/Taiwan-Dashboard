package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/auth"
)

type oauthStore interface {
	SaveOAuthLogin(ctx context.Context, l auth.OAuthLogin) error
	TakeOAuthLogin(ctx context.Context, stateHash []byte, provider string) (*auth.OAuthLogin, error)
	SignInExternal(ctx context.Context, provider string, id auth.Identity) (*auth.User, bool, error)
	CreateSession(ctx context.Context, userID int64, ip, userAgent string) (string, time.Time, error)
	Lookup(ctx context.Context, token string) (*auth.User, error)
	Revoke(ctx context.Context, token string) error
	Audit(ctx context.Context, userID *int64, action, target string, detail map[string]any, ip string) error
	SavePendingLink(ctx context.Context, l auth.PendingLink) error
	PendingLinkInfo(ctx context.Context, tokenHash []byte) (*auth.PendingLink, error)
	TakePendingLink(ctx context.Context, tokenHash []byte) (*auth.PendingLink, error)
	LinkIdentity(ctx context.Context, userID int64, provider, subject, ip string) error
}

func (a AuthDeps) linkCookieName() string {
	if a.Secure {
		return "__Host-dtw_link"
	}
	return "dtw_link"
}

func (a AuthDeps) setLinkCookie(c *gin.Context, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: a.linkCookieName(), Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteLaxMode,
	})
}

func (a AuthDeps) linkInfo(c *gin.Context) {
	token, _ := c.Cookie(a.linkCookieName())
	if token == "" || len(token) > 100 {
		writeError(c, http.StatusNotFound, "not_found", "沒有待綁定的帳號")
		return
	}
	l, err := a.oauthDB().PendingLinkInfo(c.Request.Context(), auth.HashToken(token))
	if errors.Is(err, auth.ErrNoSession) {
		a.setLinkCookie(c, "", -1)
		writeError(c, http.StatusNotFound, "not_found", "沒有待綁定的帳號")
		return
	}
	if err != nil {
		slog.Error("link info", "error", err)
		writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
		return
	}
	writeData(c, http.StatusOK, gin.H{"provider": l.Provider, "existing": l.Existing, "expires_at": l.ExpiresAt}, nil)
}

func (a AuthDeps) cancelLink(c *gin.Context) {
	if token, _ := c.Cookie(a.linkCookieName()); token != "" && len(token) <= 100 {
		if _, err := a.oauthDB().TakePendingLink(c.Request.Context(), auth.HashToken(token)); err != nil && !errors.Is(err, auth.ErrNoSession) {
			slog.Error("cancel link", "error", err)
		}
	}
	a.setLinkCookie(c, "", -1)
	c.Status(http.StatusNoContent)
}

func (a AuthDeps) oauthDB() oauthStore {
	if a.oauthStore != nil {
		return a.oauthStore
	}
	return a.Store
}

func (a AuthDeps) httpClient() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (a AuthDeps) oauthProvider(id string) *auth.OAuthProvider {
	for _, p := range a.OAuth {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (a AuthDeps) oauthCookieName() string {
	if a.Secure {
		return "__Host-dtw_oauth"
	}
	return "dtw_oauth"
}

func (a AuthDeps) setOAuthCookie(c *gin.Context, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: a.oauthCookieName(), Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteLaxMode,
	})
}

func safeReturn(s string) string {
	if s == "" || len(s) > 300 || !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") || strings.HasPrefix(s, "/\\") {
		return "/"
	}
	for _, r := range s {
		if unicode.IsControl(r) || r == '\\' {
			return "/"
		}
	}
	return s
}

func (a AuthDeps) startLink(c *gin.Context) {
	p := a.oauthProvider(c.Param("provider"))
	if p == nil {
		writeError(c, http.StatusNotFound, "not_found", "目前不提供這個登入方式")
		return
	}
	a.startOAuth(c, p, currentUser(c).ID)
}

func (a AuthDeps) startOAuth(c *gin.Context, p *auth.OAuthProvider, linkUserID int64) {
	var body struct {
		Redirect string `json:"redirect"`
	}
	_ = c.ShouldBindJSON(&body)
	origin := c.GetHeader("Origin")
	state, err1 := auth.NewToken()
	browser, err2 := auth.NewToken()
	verifier, err3 := auth.NewToken()
	if err := errors.Join(err1, err2, err3); err != nil {
		slog.Error("oauth start: random", "error", err)
		writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
		return
	}
	redirectURI := origin + "/api/v1/auth/" + p.ID + "/callback"
	err := a.oauthDB().SaveOAuthLogin(c.Request.Context(), auth.OAuthLogin{
		StateHash: auth.HashToken(state), BrowserHash: auth.HashToken(browser),
		Provider: p.ID, Verifier: verifier, RedirectURI: redirectURI, ReturnTo: safeReturn(body.Redirect),
		ExpiresAt: time.Now().Add(auth.OAuthLoginTTL), LinkUserID: linkUserID,
	})
	if err != nil {
		slog.Error("oauth start: save", "error", err)
		writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
		return
	}
	a.setOAuthCookie(c, browser, int(auth.OAuthLoginTTL.Seconds()))
	writeData(c, http.StatusOK, gin.H{"redirect_url": p.AuthCodeURL(state, verifier, redirectURI)}, nil)
}

func (a AuthDeps) callback(c *gin.Context) {
	linking := false
	fail := func(code, returnTo string) {
		if linking {
			c.Redirect(http.StatusSeeOther, "/me?"+url.Values{"link_error": {code}}.Encode())
			return
		}
		q := url.Values{"error": {code}}
		if returnTo != "" && returnTo != "/" {
			q.Set("redirect", returnTo)
		}
		c.Redirect(http.StatusSeeOther, "/login?"+q.Encode())
	}
	p := a.oauthProvider(c.Param("provider"))
	if p == nil {
		fail("unavailable", "")
		return
	}
	ctx := c.Request.Context()
	state, code := c.Query("state"), c.Query("code")
	browser, _ := c.Cookie(a.oauthCookieName())
	a.setOAuthCookie(c, "", -1)
	if state == "" || len(state) > 200 {
		fail("failed", "")
		return
	}
	l, err := a.oauthDB().TakeOAuthLogin(ctx, auth.HashToken(state), p.ID)
	if errors.Is(err, auth.ErrNoSession) {
		fail("expired", "")
		return
	}
	if err != nil {
		slog.Error("oauth callback: take", "error", err)
		fail("failed", "")
		return
	}
	linking = l.LinkUserID != 0
	if c.Query("error") != "" {
		fail("cancelled", l.ReturnTo)
		return
	}
	if browser == "" || subtle.ConstantTimeCompare(auth.HashToken(browser), l.BrowserHash) != 1 {
		fail("failed", l.ReturnTo)
		return
	}
	if code == "" || len(code) > 500 {
		fail("failed", l.ReturnTo)
		return
	}
	sctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	id, err := p.SignIn(sctx, a.httpClient(), code, l.Verifier, l.RedirectURI, time.Now())
	if errors.Is(err, auth.ErrAccountTooNew) {
		fail("too_new", l.ReturnTo)
		return
	}
	if err != nil {
		slog.Warn("oauth callback: provider", "provider", p.ID, "error", err)
		fail("failed", l.ReturnTo)
		return
	}
	if linking {
		a.linkFromAccount(c, l, p, id)
		return
	}
	u, created, err := a.oauthDB().SignInExternal(ctx, p.ID, id)
	if errors.Is(err, auth.ErrDisabled) {
		fail("disabled", l.ReturnTo)
		return
	}
	var need *auth.LinkRequired
	if errors.As(err, &need) {
		token, terr := auth.NewToken()
		if terr == nil {
			terr = a.oauthDB().SavePendingLink(ctx, auth.PendingLink{
				TokenHash: auth.HashToken(token), UserID: need.UserID, Provider: p.ID, Subject: id.Subject,
				ExpiresAt: time.Now().Add(auth.LinkTTL),
			})
		}
		if terr != nil {
			slog.Error("oauth callback: pending link", "error", terr)
			fail("failed", l.ReturnTo)
			return
		}
		a.setLinkCookie(c, token, int(auth.LinkTTL.Seconds()))
		fail("link", l.ReturnTo)
		return
	}
	if err != nil {
		slog.Error("oauth callback: user", "error", err)
		fail("failed", l.ReturnTo)
		return
	}
	if old, err := c.Cookie(a.cookieName()); err == nil && old != "" {
		_ = a.oauthDB().Revoke(ctx, old)
	}
	token, expires, err := a.oauthDB().CreateSession(ctx, u.ID, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		slog.Error("oauth callback: session", "error", err)
		fail("failed", l.ReturnTo)
		return
	}
	if err := a.oauthDB().Audit(ctx, &u.ID, "login", "", map[string]any{"provider": p.ID, "new_account": created}, c.ClientIP()); err != nil {
		slog.Error("audit login", "error", err)
	}
	a.setCookie(c, token, expires)
	c.Redirect(http.StatusSeeOther, a.finishLink(c, u.ID, l.ReturnTo))
}

func (a AuthDeps) linkFromAccount(c *gin.Context, l *auth.OAuthLogin, p *auth.OAuthProvider, id auth.Identity) {
	back := func(key, value string) {
		c.Redirect(http.StatusSeeOther, "/me?"+url.Values{key: {value}}.Encode())
	}
	if u := currentUser(c); u == nil || u.ID != l.LinkUserID {
		back("link_error", "signed_out")
		return
	}
	err := a.oauthDB().LinkIdentity(c.Request.Context(), l.LinkUserID, p.ID, id.Subject, c.ClientIP())
	switch {
	case errors.Is(err, auth.ErrIdentityTaken):
		back("link_error", "taken")
	case errors.Is(err, auth.ErrProviderLinked):
		back("link_error", "provider")
	case err != nil:
		slog.Error("link from account", "error", err)
		back("link_error", "failed")
	default:
		back("linked", p.ID)
	}
}

func (a AuthDeps) finishLink(c *gin.Context, userID int64, returnTo string) string {
	token, _ := c.Cookie(a.linkCookieName())
	if token == "" || len(token) > 100 {
		return returnTo
	}
	a.setLinkCookie(c, "", -1)
	ctx := c.Request.Context()
	pl, err := a.oauthDB().TakePendingLink(ctx, auth.HashToken(token))
	if err != nil {
		if !errors.Is(err, auth.ErrNoSession) {
			slog.Error("finish link: take", "error", err)
		}
		return returnTo
	}
	if pl.UserID != userID {
		return returnTo
	}
	err = a.oauthDB().LinkIdentity(ctx, userID, pl.Provider, pl.Subject, c.ClientIP())
	if errors.Is(err, auth.ErrIdentityTaken) {
		return "/me?" + url.Values{"link_error": {"taken"}}.Encode()
	}
	if errors.Is(err, auth.ErrProviderLinked) {
		return "/me?" + url.Values{"link_error": {"provider"}}.Encode()
	}
	if err != nil {
		slog.Error("finish link", "error", err)
		return "/me?" + url.Values{"link_error": {"failed"}}.Encode()
	}
	return "/me?" + url.Values{"linked": {pl.Provider}}.Encode()
}
