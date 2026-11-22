package api

import (
	"crypto/hmac"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/db"
)

type userRow struct {
	ID          int64      `json:"id"`
	DisplayName string     `json:"display_name"`
	Role        string     `json:"role"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

func (h adminHandlers) searchUsers(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	var in struct {
		EmailSHA256 string `json:"email_sha256"`
	}
	if !bindJSON(c, &in) {
		return
	}
	digest, err := hex.DecodeString(in.EmailSHA256)
	if err != nil || len(digest) != 32 {
		writeError(c, http.StatusBadRequest, "invalid_digest", "請輸入 Email")
		return
	}
	vault := h.store.Emails
	if vault == nil {
		writeError(c, http.StatusServiceUnavailable, "email_disabled", "伺服器沒有設定 Email 金鑰，無法用 Email 搜尋")
		return
	}
	ctx := c.Request.Context()
	found := []userRow{}
	err = db.AsSystem(ctx, h.db, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT id, email_hash FROM app.users WHERE email_hash IS NOT NULL`)
		if err != nil {
			return err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			var fp []byte
			if err := rows.Scan(&id, &fp); err != nil {
				rows.Close()
				return err
			}
			if hmac.Equal(vault.FingerprintDigest(id, digest), fp) {
				ids = append(ids, id)
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, id := range ids {
			u, err := loadUserRow(c, tx, id)
			if err != nil {
				return err
			}
			found = append(found, *u)
		}
		return nil
	})
	if err != nil {
		adminFail(c, err)
		return
	}
	writeData(c, http.StatusOK, found, nil)
}

func loadUserRow(c *gin.Context, tx *sql.Tx, id int64) (*userRow, error) {
	var u userRow
	err := tx.QueryRowContext(c.Request.Context(), `
		SELECT id, display_name, role, status, created_at, last_login_at FROM app.users WHERE id = $1`, id).
		Scan(&u.ID, &u.DisplayName, &u.Role, &u.Status, &u.CreatedAt, &u.LastLoginAt)
	return &u, err
}

type userClient struct {
	Name       string     `json:"name"`
	ClientID   string     `json:"client_id"`
	Enabled    bool       `json:"enabled"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

type userEvent struct {
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	CreatedAt time.Time `json:"created_at"`
}

func (h adminHandlers) userDetail(c *gin.Context) {
	me, ok := mustAdmin(c)
	if !ok {
		return
	}
	id, ok := userParam(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	out := gin.H{}
	err := db.AsSystem(ctx, h.db, func(tx *sql.Tx) error {
		u, err := loadUserRow(c, tx, id)
		if err != nil {
			return err
		}
		out["user"] = u
		out["is_me"] = u.ID == me.ID

		var hasEmail bool
		var sessions int
		var lastSeen *time.Time
		if err := tx.QueryRowContext(ctx, `
			SELECT u.email_enc IS NOT NULL,
			       (SELECT count(*) FROM app.session s WHERE s.user_id = u.id AND s.revoked_at IS NULL AND s.expires_at > now()),
			       (SELECT max(coalesce(s.last_seen_at, s.created_at)) FROM app.session s WHERE s.user_id = u.id)
			  FROM app.users u WHERE u.id = $1`, id).Scan(&hasEmail, &sessions, &lastSeen); err != nil {
			return err
		}
		out["has_email"], out["active_sessions"], out["last_seen_at"] = hasEmail, sessions, lastSeen

		providers := []gin.H{}
		rows, err := tx.QueryContext(ctx, `
			SELECT provider, created_at FROM app.user_identity WHERE user_id = $1 ORDER BY created_at`, id)
		if err != nil {
			return err
		}
		for rows.Next() {
			var p string
			var at time.Time
			if err := rows.Scan(&p, &at); err != nil {
				rows.Close()
				return err
			}
			providers = append(providers, gin.H{"provider": p, "created_at": at})
		}
		if err := rows.Close(); err != nil {
			return err
		}
		out["providers"] = providers

		clients := []userClient{}
		rows, err = tx.QueryContext(ctx, `
			SELECT name, prefix, disabled_at IS NULL, created_at, last_used_at
			  FROM app.api_key WHERE user_id = $1 AND revoked_at IS NULL ORDER BY created_at`, id)
		if err != nil {
			return err
		}
		for rows.Next() {
			var k userClient
			if err := rows.Scan(&k.Name, &k.ClientID, &k.Enabled, &k.CreatedAt, &k.LastUsedAt); err != nil {
				rows.Close()
				return err
			}
			clients = append(clients, k)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		out["clients"] = clients

		var today, month auth.Usage
		if err := tx.QueryRowContext(ctx, `
			SELECT coalesce(sum(d.requests) FILTER (WHERE d.day = (now() AT TIME ZONE 'Asia/Taipei')::date), 0),
			       coalesce(sum(d.bytes) FILTER (WHERE d.day = (now() AT TIME ZONE 'Asia/Taipei')::date), 0),
			       coalesce(sum(d.requests), 0), coalesce(sum(d.bytes), 0)
			  FROM app.api_usage_daily d JOIN app.api_key k ON k.id = d.api_key_id
			 WHERE k.user_id = $1 AND d.day > (now() AT TIME ZONE 'Asia/Taipei')::date - 30`, id).
			Scan(&today.Requests, &today.Bytes, &month.Requests, &month.Bytes); err != nil {
			return err
		}
		out["usage_today"], out["usage_30d"] = today, month

		var survey *gin.H
		var identity, purpose string
		var push *string
		var updated time.Time
		err = tx.QueryRowContext(ctx, `
			SELECT identity, purpose, wants_push, updated_at FROM app.api_survey WHERE user_id = $1`, id).
			Scan(&identity, &purpose, &push, &updated)
		if err == nil {
			survey = &gin.H{"identity": identity, "purpose": purpose, "wants_push": push, "updated_at": updated}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		out["survey"] = survey

		events := []userEvent{}
		rows, err = tx.QueryContext(ctx, `
			SELECT action, target, created_at FROM app.audit_log
			 WHERE user_id = $1 OR target = $2 ORDER BY created_at DESC LIMIT 30`, id, fmt.Sprintf("user:%d", id))
		if err != nil {
			return err
		}
		for rows.Next() {
			var e userEvent
			if err := rows.Scan(&e.Action, &e.Target, &e.CreatedAt); err != nil {
				rows.Close()
				return err
			}
			events = append(events, e)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		out["events"] = events
		return nil
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "not_found", "找不到這個帳號")
		return
	}
	if err != nil {
		adminFail(c, err)
		return
	}
	writeData(c, http.StatusOK, out, nil)
}

func (h adminHandlers) updateUser(c *gin.Context) {
	me, ok := mustAdmin(c)
	if !ok {
		return
	}
	id, ok := userParam(c)
	if !ok {
		return
	}
	var in struct {
		Role   *string `json:"role"`
		Status *string `json:"status"`
	}
	if !bindJSON(c, &in) {
		return
	}
	if in.Role == nil && in.Status == nil {
		writeError(c, http.StatusBadRequest, "nothing_to_change", "沒有要修改的項目")
		return
	}
	if in.Role != nil && *in.Role != auth.RoleAdmin && *in.Role != "user" {
		writeError(c, http.StatusBadRequest, "invalid_role", "角色只能是 user 或 admin")
		return
	}
	if in.Status != nil && *in.Status != "active" && *in.Status != "disabled" {
		writeError(c, http.StatusBadRequest, "invalid_status", "狀態只能是 active 或 disabled")
		return
	}
	if id == me.ID {
		writeError(c, http.StatusForbidden, "cannot_change_self", "不能修改自己的角色或狀態")
		return
	}
	ctx := c.Request.Context()
	err := db.AsSystem(ctx, h.db, func(tx *sql.Tx) error {
		var role, status string
		if err := tx.QueryRowContext(ctx, `
			SELECT role, status FROM app.users WHERE id = $1 FOR UPDATE`, id).Scan(&role, &status); err != nil {
			return err
		}
		detail := map[string]any{}
		if in.Role != nil && *in.Role != role {
			detail["role"] = []string{role, *in.Role}
			role = *in.Role
		}
		if in.Status != nil && *in.Status != status {
			detail["status"] = []string{status, *in.Status}
			status = *in.Status
		}
		if len(detail) == 0 {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `UPDATE app.users SET role = $2, status = $3 WHERE id = $1`,
			id, role, status); err != nil {
			return err
		}
		if status == "disabled" {
			if _, err := tx.ExecContext(ctx, `
				UPDATE app.session SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				DELETE FROM app.api_token t USING app.api_key k WHERE k.id = t.api_key_id AND k.user_id = $1`, id); err != nil {
				return err
			}
		}
		return auth.AuditTx(ctx, tx, &me.ID, "admin.user.update", fmt.Sprintf("user:%d", id), detail, c.ClientIP())
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "not_found", "找不到這個帳號")
		return
	}
	if err != nil {
		adminFail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func userParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusNotFound, "not_found", "找不到這個帳號")
		return 0, false
	}
	return id, true
}
