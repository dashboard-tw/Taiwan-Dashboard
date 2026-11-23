package api

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/db"
)

type meHandlers struct {
	db     *sql.DB
	store  *auth.Store
	auth   AuthDeps
	submit *submitGate
}

func registerMe(v1 *gin.RouterGroup, a AuthDeps) {
	h := meHandlers{db: a.Store.DB, store: a.Store, auth: a, submit: newSubmitGate()}
	me := v1.Group("/me", private(), a.sameOrigin(), a.session(), requireUser())
	me.GET("", func(c *gin.Context) {
		writeData(c, http.StatusOK, currentUser(c), nil)
	})
	me.GET("/account", h.account)
	me.PUT("/account", h.rename)
	me.DELETE("/identities/:provider", h.unlink)
	me.DELETE("", h.deleteAccount)
	me.GET("/api-clients", h.apiClients)
	me.POST("/api-clients", h.createClient)
	me.POST("/api-clients/:id/secret", h.rotateSecret)
	me.PATCH("/api-clients/:id", h.updateClient)
	me.DELETE("/api-clients/:id", h.deleteClient)
	me.GET("/oauth-apps", listOAuthApps(a))
	me.DELETE("/oauth-apps/:id", revokeOAuthApp(a))
	me.PUT("/api-survey", h.saveSurvey)
	me.GET("/apps", h.myApps)
	me.POST("/apps", h.createApp)
	me.PUT("/apps/:id", h.updateApp)
	me.PUT("/apps/:id/image", h.uploadAppImage)
	me.PUT("/apps/:id/screenshot", h.uploadAppScreenshot)
	me.DELETE("/apps/:id", h.deleteApp)
	me.GET("/devices", h.devices)
	me.DELETE("/devices", h.signOutOthers)
	me.DELETE("/devices/:id", h.signOutDevice)
}

func (h meHandlers) token(c *gin.Context) string {
	t, _ := c.Cookie(h.auth.cookieName())
	return t
}

func (h meHandlers) devices(c *gin.Context) {
	list, err := h.store.Devices(c.Request.Context(), h.user(c).ID, h.token(c))
	if err != nil {
		meFail(c, err)
		return
	}
	writeData(c, http.StatusOK, list, nil)
}

func (h meHandlers) signOutDevice(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusNotFound, "not_found", "找不到這個裝置")
		return
	}
	current, err := h.store.SignOutDevice(c.Request.Context(), h.user(c).ID, id, h.token(c), c.ClientIP())
	if errors.Is(err, auth.ErrDeviceNotFound) {
		writeError(c, http.StatusNotFound, "not_found", "找不到這個裝置")
		return
	}
	if err != nil {
		meFail(c, err)
		return
	}
	if current {
		h.auth.clearCookie(c)
	}
	c.Status(http.StatusNoContent)
}

func (h meHandlers) signOutOthers(c *gin.Context) {
	n, err := h.store.SignOutOtherDevices(c.Request.Context(), h.user(c).ID, h.token(c), c.ClientIP())
	if err != nil {
		meFail(c, err)
		return
	}
	writeData(c, http.StatusOK, gin.H{"signed_out": n}, nil)
}

func (h meHandlers) user(c *gin.Context) *auth.User { return currentUser(c) }

func (h meHandlers) asUser(c *gin.Context, fn func(*sql.Tx) error) error {
	return db.AsUser(c.Request.Context(), h.db, h.user(c).ID, fn)
}

func meFail(c *gin.Context, err error) {
	slog.Error("me api", "path", c.FullPath(), "error", err)
	writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
}

type account struct {
	ID          int64     `json:"id"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
	Providers   []string  `json:"providers"`
}

func (h meHandlers) account(c *gin.Context) {
	u := h.user(c)
	out := account{ID: u.ID, DisplayName: u.DisplayName, Role: u.Role, Providers: []string{}}
	err := h.asUser(c, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(c.Request.Context(),
			`SELECT created_at FROM app.users WHERE id = $1`, u.ID).Scan(&out.CreatedAt); err != nil {
			return err
		}
		rows, err := tx.QueryContext(c.Request.Context(),
			`SELECT provider FROM app.user_identity WHERE user_id = $1 ORDER BY created_at`, u.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				return err
			}
			out.Providers = append(out.Providers, p)
		}
		return rows.Err()
	})
	if err != nil {
		meFail(c, err)
		return
	}
	writeData(c, http.StatusOK, out, nil)
}

const MaxDisplayName = 30

func cleanDisplayName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	n := 0
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == utf8.RuneError {
			return "", false
		}
		n++
	}
	return s, n >= 1 && n <= MaxDisplayName
}

func (h meHandlers) rename(c *gin.Context) {
	var in struct {
		DisplayName string `json:"display_name"`
	}
	if !bindJSON(c, &in) {
		return
	}
	name, ok := cleanDisplayName(in.DisplayName)
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_name", fmt.Sprintf("名稱要 1 到 %d 個字，不能有看不見的字元", MaxDisplayName))
		return
	}
	u := h.user(c)
	ctx := c.Request.Context()
	err := h.asUser(c, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE app.users SET display_name = $2 WHERE id = $1`, u.ID, name); err != nil {
			return err
		}
		return auth.AuditTx(ctx, tx, &u.ID, "account.rename", "user:"+strconv.FormatInt(u.ID, 10), nil, c.ClientIP())
	})
	if err != nil {
		meFail(c, err)
		return
	}
	writeData(c, http.StatusOK, gin.H{"display_name": name}, nil)
}

func (h meHandlers) unlink(c *gin.Context) {
	err := h.store.UnlinkIdentity(c.Request.Context(), h.user(c).ID, c.Param("provider"), c.ClientIP())
	switch {
	case errors.Is(err, auth.ErrIdentityNotFound):
		writeError(c, http.StatusNotFound, "not_found", "還沒有綁定這個登入方式")
	case errors.Is(err, auth.ErrLastIdentity):
		writeError(c, http.StatusConflict, "last_identity", "至少需保留一種登入方式")
	case err != nil:
		meFail(c, err)
	default:
		c.Status(http.StatusNoContent)
	}
}

func (h meHandlers) deleteAccount(c *gin.Context) {
	u := h.user(c)
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if !bindJSON(c, &in) {
		return
	}
	if !in.Confirm {
		writeError(c, http.StatusBadRequest, "confirm_required", "請確認要刪除帳號")
		return
	}
	if u.IsAdmin() {
		writeError(c, http.StatusForbidden, "admin_account", "管理員帳號無法在網頁上刪除")
		return
	}
	ctx := c.Request.Context()
	sendGoodbye := h.auth.Notify.AccountDeleting(ctx, u.ID)
	err := h.asUser(c, func(tx *sql.Tx) error {
		if err := auth.AuditTx(ctx, tx, nil, "account.delete", "user:"+strconv.FormatInt(u.ID, 10), nil, c.ClientIP()); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM app.users WHERE id = $1`, u.ID)
		return err
	})
	if err != nil {
		meFail(c, err)
		return
	}
	sendGoodbye()
	h.auth.clearCookie(c)
	c.Status(http.StatusNoContent)
}

func (h meHandlers) apiClients(c *gin.Context) {
	ctx := c.Request.Context()
	uid := h.user(c).ID
	clients, err := h.store.Clients(ctx, uid)
	if err != nil {
		meFail(c, err)
		return
	}
	usage, err := h.store.APIKeyUsage(ctx, uid, 30)
	if err != nil {
		meFail(c, err)
		return
	}
	used, err := h.store.UsedToday(ctx, uid)
	if err != nil {
		meFail(c, err)
		return
	}
	tier, err := h.store.TierOf(ctx, uid)
	if err != nil {
		meFail(c, err)
		return
	}
	mcpCalls, err := h.store.MCPCallsToday(ctx, uid)
	if err != nil {
		meFail(c, err)
		return
	}
	var s *survey
	if err := h.asUser(c, func(tx *sql.Tx) (err error) {
		s, err = loadSurvey(tx, c)
		return err
	}); err != nil {
		meFail(c, err)
		return
	}
	writeData(c, http.StatusOK, gin.H{
		"clients": clients, "max_clients": auth.MaxClients, "used_today": used, "mcp_calls_today": mcpCalls,
		"usage": usage, "tier": tier, "limits": auth.Tiers[tier], "partner_limits": auth.Tiers[auth.TierPartner], "token_ttl": int(auth.TokenTTL.Seconds()),
		"survey": s, "terms_version": APITermsVersion,
	}, nil)
}

func clientID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusNotFound, "not_found", "找不到這組憑證")
		return 0, false
	}
	return id, true
}

func clientFail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrClientNotFound):
		writeError(c, http.StatusNotFound, "not_found", "找不到這組憑證")
	case errors.Is(err, auth.ErrTooManyClients):
		writeError(c, http.StatusConflict, "too_many_clients", fmt.Sprintf("每個帳號最多 %d 組憑證", auth.MaxClients))
	default:
		meFail(c, err)
	}
}

func (h meHandlers) createClient(c *gin.Context) {
	var in struct {
		Name string `json:"name"`
	}
	if !bindJSON(c, &in) {
		return
	}
	name, ok := cleanDisplayName(in.Name)
	if !ok {
		writeError(c, http.StatusBadRequest, "invalid_name", "名稱要 1 到 30 個字，不能有看不見的字元")
		return
	}
	done, err := h.surveyDone(c)
	if err != nil {
		meFail(c, err)
		return
	}
	if !done {
		writeError(c, http.StatusConflict, "survey_required", "請先填寫使用調查並同意使用規範")
		return
	}
	client, secret, err := h.store.CreateClient(c.Request.Context(), h.user(c).ID, name, c.ClientIP())
	if err != nil {
		clientFail(c, err)
		return
	}
	writeData(c, http.StatusCreated, gin.H{"client": client, "client_secret": secret}, nil)
}

func (h meHandlers) rotateSecret(c *gin.Context) {
	id, ok := clientID(c)
	if !ok {
		return
	}
	done, err := h.surveyDone(c)
	if err != nil {
		meFail(c, err)
		return
	}
	if !done {
		writeError(c, http.StatusConflict, "survey_required", "請先填寫使用調查並同意使用規範")
		return
	}
	secret, err := h.store.RotateClientSecret(c.Request.Context(), h.user(c).ID, id, c.ClientIP())
	if err != nil {
		clientFail(c, err)
		return
	}
	writeData(c, http.StatusOK, gin.H{"client_secret": secret}, nil)
}

func (h meHandlers) updateClient(c *gin.Context) {
	id, ok := clientID(c)
	if !ok {
		return
	}
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if !bindJSON(c, &in) {
		return
	}
	if in.Enabled == nil {
		writeError(c, http.StatusBadRequest, "invalid_body", "請指定 enabled")
		return
	}
	if err := h.store.SetClientEnabled(c.Request.Context(), h.user(c).ID, id, *in.Enabled, c.ClientIP()); err != nil {
		clientFail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h meHandlers) deleteClient(c *gin.Context) {
	id, ok := clientID(c)
	if !ok {
		return
	}
	if err := h.store.DeleteClient(c.Request.Context(), h.user(c).ID, id, c.ClientIP()); err != nil {
		clientFail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
