package api

import (
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/auth"
)

type AuthDeps struct {
	Store      *auth.Store
	ImageDir   string
	Providers  []auth.Provider
	Origins    []string
	Secure     bool
	OAuth      []*auth.OAuthProvider
	HTTPClient *http.Client
	oauthStore oauthStore
}

const userKey = "dtw.user"

func (a AuthDeps) cookieName() string {
	if a.Secure {
		return "__Host-dtw_session"
	}
	return "dtw_session"
}

func (a AuthDeps) setCookie(c *gin.Context, token string, expires time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     a.cookieName(),
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   a.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a AuthDeps) clearCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: a.cookieName(), Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteLaxMode,
	})
}

func (a AuthDeps) session() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(a.cookieName())
		if err == nil && token != "" {
			u, err := a.oauthDB().Lookup(c.Request.Context(), token)
			switch {
			case err == nil:
				c.Set(userKey, u)
			case !errors.Is(err, auth.ErrNoSession):
				slog.Error("session lookup", "error", err)
				noStore(c)
				writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
				return
			}
		}
		c.Next()
	}
}

func currentUser(c *gin.Context) *auth.User {
	if v, ok := c.Get(userKey); ok {
		if u, ok := v.(*auth.User); ok {
			return u
		}
	}
	return nil
}

func private() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("Vary", "Cookie")
		c.Next()
	}
}

func (a AuthDeps) sameOrigin() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		if o := c.GetHeader("Origin"); o == "" || !slices.Contains(a.Origins, o) {
			writeError(c, http.StatusForbidden, "bad_origin", "請從 dashboard.tw 網站操作")
			return
		}
		c.Next()
	}
}

func requireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		if currentUser(c) == nil {
			writeError(c, http.StatusUnauthorized, "unauthenticated", "請先登入")
			return
		}
		c.Next()
	}
}

func requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		if u == nil {
			writeError(c, http.StatusUnauthorized, "unauthenticated", "請先登入")
			return
		}
		if !u.IsAdmin() {
			writeError(c, http.StatusForbidden, "forbidden", "沒有權限")
			return
		}
		c.Next()
	}
}

func mustAdmin(c *gin.Context) (*auth.User, bool) {
	u := currentUser(c)
	if !u.IsAdmin() {
		writeError(c, http.StatusForbidden, "forbidden", "沒有權限")
		return nil, false
	}
	return u, true
}

func registerAuth(v1 *gin.RouterGroup, a AuthDeps) {
	g := v1.Group("/auth", private(), newLimiter(20, 10).middleware(), a.sameOrigin(), a.session())
	g.GET("/providers", func(c *gin.Context) {
		providers := a.Providers
		if providers == nil {
			providers = []auth.Provider{}
		}
		writeData(c, http.StatusOK, providers, nil)
	})
	g.GET("/session", func(c *gin.Context) {
		writeData(c, http.StatusOK, currentUser(c), nil)
	})
	g.POST("/:provider/start", a.start)
	g.GET("/:provider/callback", a.callback)
	g.POST("/logout", a.logout)
	g.GET("/link", a.linkInfo)
	g.DELETE("/link", a.cancelLink)
	g.POST("/:provider/link", requireUser(), a.startLink)
}

func (a AuthDeps) provider(id string) *auth.Provider {
	for i := range a.Providers {
		if a.Providers[i].ID == id {
			return &a.Providers[i]
		}
	}
	return nil
}

func (a AuthDeps) start(c *gin.Context) {
	if op := a.oauthProvider(c.Param("provider")); op != nil {
		a.startOAuth(c, op, 0)
		return
	}
	p := a.provider(c.Param("provider"))
	if p == nil || !p.Dev {
		writeError(c, http.StatusNotFound, "provider_not_found", "不支援這個登入方式")
		return
	}
	ctx := c.Request.Context()
	var u *auth.User
	var err error
	if p.ID == auth.DevUserProvider.ID {
		u, err = a.Store.EnsureDevUser(ctx)
	} else {
		u, err = a.Store.FindIdentity(ctx, "dev", auth.DevSubject)
	}
	if errors.Is(err, auth.ErrNoSession) {
		writeError(c, http.StatusConflict, "no_admin", "還沒有管理員，請先執行 dashboardtw admin create")
		return
	}
	if err != nil {
		slog.Error("dev sign-in", "error", err)
		writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
		return
	}
	if old, err := c.Cookie(a.cookieName()); err == nil && old != "" {
		_ = a.Store.Revoke(ctx, old)
	}
	token, expires, err := a.Store.CreateSession(ctx, u.ID, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		slog.Error("create session", "error", err)
		writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
		return
	}
	if err := a.Store.Audit(ctx, &u.ID, "login", "", map[string]any{"provider": p.ID}, c.ClientIP()); err != nil {
		slog.Error("audit login", "error", err)
	}
	a.setCookie(c, token, expires)
	writeData(c, http.StatusOK, u, nil)
}

func (a AuthDeps) logout(c *gin.Context) {
	ctx := c.Request.Context()
	if token, err := c.Cookie(a.cookieName()); err == nil && token != "" {
		if err := a.Store.Revoke(ctx, token); err != nil {
			slog.Error("revoke session", "error", err)
		}
	}
	if u := currentUser(c); u != nil {
		if err := a.Store.Audit(ctx, &u.ID, "logout", "", nil, c.ClientIP()); err != nil {
			slog.Error("audit logout", "error", err)
		}
	}
	a.clearCookie(c)
	c.Status(http.StatusNoContent)
}
