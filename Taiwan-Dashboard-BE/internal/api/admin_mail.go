package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/db"
	"dashboard.tw/backend/internal/hooks"
	"dashboard.tw/backend/internal/notify"
)

type mailTemplate struct {
	Key       string       `json:"key"`
	Name      string       `json:"name"`
	Subject   string       `json:"subject"`
	Body      string       `json:"body"`
	Enabled   bool         `json:"enabled"`
	UpdatedAt time.Time    `json:"updated_at"`
	UpdatedBy *string      `json:"updated_by"`
	Vars      []notify.Var `json:"vars"`
}

var mailKeyRe = regexp.MustCompile(`^[a-z_]{1,40}$`)

func (h adminHandlers) mailTemplates(c *gin.Context) {
	ctx := c.Request.Context()
	rows, err := h.db.QueryContext(ctx, `
		SELECT t.key, t.name, t.subject, t.body, t.enabled, t.updated_at, u.display_name
		  FROM app.mail_template t LEFT JOIN app.users u ON u.id = t.updated_by
		 ORDER BY t.sort, t.key`)
	if err != nil {
		adminFail(c, err)
		return
	}
	defer rows.Close()
	out := []mailTemplate{}
	for rows.Next() {
		var t mailTemplate
		if err := rows.Scan(&t.Key, &t.Name, &t.Subject, &t.Body, &t.Enabled, &t.UpdatedAt, &t.UpdatedBy); err != nil {
			adminFail(c, err)
			return
		}
		t.Vars = notify.VarsOf(t.Key)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		adminFail(c, err)
		return
	}
	writeData(c, http.StatusOK, gin.H{"sending": h.notify.Enabled(), "templates": out}, nil)
}

type mailTemplateInput struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Enabled *bool  `json:"enabled"`
}

func (in *mailTemplateInput) validate(c *gin.Context, key string) bool {
	in.Subject = strings.TrimSpace(in.Subject)
	in.Body = strings.TrimSpace(strings.ReplaceAll(in.Body, "\r\n", "\n"))
	switch {
	case key != "footer" && in.Subject == "":
		writeError(c, http.StatusBadRequest, "invalid_subject", "請填寫主旨")
	case utf8.RuneCountInString(in.Subject) > 200:
		writeError(c, http.StatusBadRequest, "invalid_subject", "主旨最多 200 個字")
	case in.Body == "":
		writeError(c, http.StatusBadRequest, "invalid_body", "請填寫內容")
	case utf8.RuneCountInString(in.Body) > 5000:
		writeError(c, http.StatusBadRequest, "invalid_body", "內容最多 5000 個字")
	default:
		if err := notify.Check(key, in.Subject, in.Body); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_template", err.Error())
			return false
		}
		return true
	}
	return false
}

func mailKey(c *gin.Context) (string, bool) {
	key := c.Param("key")
	if !mailKeyRe.MatchString(key) {
		writeError(c, http.StatusNotFound, "not_found", "找不到這個信件模板")
		return "", false
	}
	return key, true
}

func (h adminHandlers) updateMailTemplate(c *gin.Context) {
	me, ok := mustAdmin(c)
	if !ok {
		return
	}
	key, ok := mailKey(c)
	if !ok {
		return
	}
	var in mailTemplateInput
	if !bindJSON(c, &in) || !in.validate(c, key) {
		return
	}
	ctx := c.Request.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		adminFail(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck // a no-op after Commit
	res, err := tx.ExecContext(ctx, `
		UPDATE app.mail_template
		   SET subject = $2, body = $3, enabled = COALESCE($4, enabled), updated_at = now(), updated_by = $5
		 WHERE key = $1`, key, in.Subject, in.Body, in.Enabled, me.ID)
	if err == nil {
		if n, _ := res.RowsAffected(); n == 0 {
			err = sql.ErrNoRows
		}
	}
	if err == nil {
		err = auth.AuditTx(ctx, tx, &me.ID, "admin.mail_template.update", "mail:"+key, nil, c.ClientIP())
	}
	if err == nil {
		err = tx.Commit()
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "not_found", "找不到這個信件模板")
		return
	}
	if err != nil {
		adminFail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h adminHandlers) previewMailTemplate(c *gin.Context) {
	key, ok := mailKey(c)
	if !ok {
		return
	}
	var in mailTemplateInput
	if !bindJSON(c, &in) || !in.validate(c, key) {
		return
	}
	subject, body, err := notify.Preview(c.Request.Context(), h.db, key, in.Subject, in.Body, h.site)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_template", err.Error())
		return
	}
	writeData(c, http.StatusOK, gin.H{"subject": subject, "body": body}, nil)
}

func (h adminHandlers) testMailTemplate(c *gin.Context) {
	me, ok := mustAdmin(c)
	if !ok {
		return
	}
	key, ok := mailKey(c)
	if !ok {
		return
	}
	if !h.notify.Enabled() {
		writeError(c, http.StatusConflict, "mail_off", "伺服器沒有設定寄信，無法寄出測試信")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	to, err := h.store.UserEmail(ctx, me.ID)
	if err != nil {
		adminFail(c, err)
		return
	}
	if to == "" {
		writeError(c, http.StatusConflict, "no_email", "您的帳號沒有信箱，無法寄出測試信")
		return
	}
	if err := h.notify.SendTest(ctx, key, to); err != nil {
		writeError(c, http.StatusBadGateway, "send_failed", "寄信失敗："+err.Error())
		return
	}
	writeData(c, http.StatusOK, gin.H{"to": to}, nil)
}

func siteOf(origins []string) string {
	if len(origins) > 0 {
		return origins[0]
	}
	return "https://dashboard.tw"
}

type queuedMail struct {
	ID            int64      `json:"id"`
	Template      string     `json:"template"`
	Name          string     `json:"name"`
	Status        string     `json:"status"`
	Attempts      int        `json:"attempts"`
	LastError     string     `json:"last_error"`
	CreatedAt     time.Time  `json:"created_at"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	SentAt        *time.Time `json:"sent_at"`
}

func (h adminHandlers) mailQueue(c *gin.Context) {
	ctx := c.Request.Context()
	counts := map[string]int{"pending": 0, "sending": 0, "sent": 0, "failed": 0, "skipped": 0}
	recent := []queuedMail{}
	err := db.AsSystem(ctx, h.db, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT status, count(*) FROM app.mail_queue WHERE created_at > now() - interval '7 days' GROUP BY status`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var s string
			var n int
			if err := rows.Scan(&s, &n); err != nil {
				rows.Close()
				return err
			}
			counts[s] = n
		}
		rows.Close()
		rows, err = tx.QueryContext(ctx, `
			SELECT q.id, q.template, COALESCE(t.name, q.template), q.status, q.attempts, q.last_error,
			       q.created_at, q.next_attempt_at, q.sent_at
			  FROM app.mail_queue q LEFT JOIN app.mail_template t ON t.key = q.template
			 ORDER BY q.id DESC LIMIT 50`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m queuedMail
			if err := rows.Scan(&m.ID, &m.Template, &m.Name, &m.Status, &m.Attempts, &m.LastError,
				&m.CreatedAt, &m.NextAttemptAt, &m.SentAt); err != nil {
				return err
			}
			recent = append(recent, m)
		}
		return rows.Err()
	})
	if err != nil {
		adminFail(c, err)
		return
	}
	out := gin.H{"sending": h.notify.Enabled(), "sandbox": h.notify.Enabled() && h.notify.Sandbox,
		"counts": counts, "recent": recent, "daily_limit": 0, "sent_today": 0}
	if h.notify.Enabled() {
		sent, err := h.notify.SentToday(ctx)
		if err != nil {
			adminFail(c, err)
			return
		}
		out["daily_limit"], out["sent_today"] = h.notify.DailyLimit, sent
	}
	writeData(c, http.StatusOK, out, nil)
}

func (h adminHandlers) retryMail(c *gin.Context) {
	me, ok := mustAdmin(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusNotFound, "not_found", "找不到這封信")
		return
	}
	ctx := c.Request.Context()
	var n int64
	err = db.AsSystem(ctx, h.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE app.mail_queue SET status = 'pending', attempts = 0, next_attempt_at = now(), last_error = ''
			 WHERE id = $1 AND status = 'failed'`, id)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	})
	if err != nil {
		adminFail(c, err)
		return
	}
	if n == 0 {
		writeError(c, http.StatusConflict, "state_changed", "這封信不是失敗狀態，請重新整理")
		return
	}
	if err := h.store.Audit(ctx, &me.ID, "admin.mail.retry", "mail_queue:"+strconv.FormatInt(id, 10), nil, c.ClientIP()); err != nil {
		adminFail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type suppression struct {
	ID        int64     `json:"id"`
	Reason    string    `json:"reason"`
	Detail    string    `json:"detail"`
	User      *string   `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

func (h adminHandlers) mailSuppressions(c *gin.Context) {
	ctx := c.Request.Context()
	rows, err := h.db.QueryContext(ctx, `
		SELECT s.id, s.reason, s.detail, u.display_name, s.created_at
		  FROM app.mail_suppression s LEFT JOIN app.users u ON u.email_lookup = s.email_lookup
		 ORDER BY s.created_at DESC LIMIT 200`)
	if err != nil {
		adminFail(c, err)
		return
	}
	defer rows.Close()
	out := []suppression{}
	for rows.Next() {
		var s suppression
		if err := rows.Scan(&s.ID, &s.Reason, &s.Detail, &s.User, &s.CreatedAt); err != nil {
			adminFail(c, err)
			return
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		adminFail(c, err)
		return
	}
	writeData(c, http.StatusOK, out, nil)
}

func (h adminHandlers) deleteSuppression(c *gin.Context) {
	me, ok := mustAdmin(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusNotFound, "not_found", "找不到這筆紀錄")
		return
	}
	ctx := c.Request.Context()
	res, err := h.db.ExecContext(ctx, `DELETE FROM app.mail_suppression WHERE id = $1`, id)
	if err != nil {
		adminFail(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(c, http.StatusNotFound, "not_found", "找不到這筆紀錄")
		return
	}
	if err := h.store.Audit(ctx, &me.ID, "admin.mail.unsuppress", "mail_suppression:"+strconv.FormatInt(id, 10), nil, c.ClientIP()); err != nil {
		adminFail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func sealerOf(s *auth.Store) hooks.Sealer {
	if s == nil || s.Emails == nil {
		return nil
	}
	return s.Emails
}
