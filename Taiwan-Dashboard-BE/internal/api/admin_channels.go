package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"dashboard.tw/backend/internal/hooks"
)

type notifyChannel struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Kind      string     `json:"kind"`
	Hint      string     `json:"hint"`
	Events    []string   `json:"events"`
	Enabled   bool       `json:"enabled"`
	UpdatedAt time.Time  `json:"updated_at"`
	LastAt    *time.Time `json:"last_at"`
	LastOK    *bool      `json:"last_ok"`
	LastError string     `json:"last_error"`
}

type outboxRow struct {
	ID        int64      `json:"id"`
	Channel   string     `json:"channel"`
	Event     string     `json:"event"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	Attempts  int        `json:"attempts"`
	LastError string     `json:"last_error"`
	CreatedAt time.Time  `json:"created_at"`
	SentAt    *time.Time `json:"sent_at"`
}

func (h adminHandlers) channels(c *gin.Context) {
	ctx := c.Request.Context()
	rows, err := h.db.QueryContext(ctx, `
		SELECT c.id, c.name, c.kind, c.target_hint, c.events, c.enabled, c.updated_at,
		       l.at, l.status = 'sent', COALESCE(l.last_error, '')
		  FROM app.notify_channel c
		  LEFT JOIN LATERAL (
		    SELECT COALESCE(o.sent_at, o.created_at) AS at, o.status, o.last_error FROM app.notify_outbox o
		     WHERE o.channel_id = c.id AND o.status IN ('sent', 'failed') ORDER BY o.id DESC LIMIT 1) l ON true
		 ORDER BY c.id`)
	if err != nil {
		adminFail(c, err)
		return
	}
	defer rows.Close()
	list := []notifyChannel{}
	m := pgtype.NewMap()
	for rows.Next() {
		var ch notifyChannel
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Kind, &ch.Hint, m.SQLScanner(&ch.Events), &ch.Enabled, &ch.UpdatedAt,
			&ch.LastAt, &ch.LastOK, &ch.LastError); err != nil {
			adminFail(c, err)
			return
		}
		list = append(list, ch)
	}
	if err := rows.Err(); err != nil {
		adminFail(c, err)
		return
	}
	recent := []outboxRow{}
	orows, err := h.db.QueryContext(ctx, `
		SELECT o.id, c.name, o.event, o.message->>'title', o.status, o.attempts, o.last_error, o.created_at, o.sent_at
		  FROM app.notify_outbox o JOIN app.notify_channel c ON c.id = o.channel_id
		 ORDER BY o.id DESC LIMIT 30`)
	if err != nil {
		adminFail(c, err)
		return
	}
	defer orows.Close()
	for orows.Next() {
		var r outboxRow
		if err := orows.Scan(&r.ID, &r.Channel, &r.Event, &r.Title, &r.Status, &r.Attempts, &r.LastError, &r.CreatedAt, &r.SentAt); err != nil {
			adminFail(c, err)
			return
		}
		recent = append(recent, r)
	}
	writeData(c, http.StatusOK, gin.H{
		"channels": list, "recent": recent, "events": hooks.Events, "kinds": hooks.Kinds, "can_save": h.sealer != nil,
	}, nil)
}

type channelInput struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	URL     string   `json:"url"`
	Token   string   `json:"token"`
	Chat    string   `json:"chat"`
	Secret  string   `json:"secret"`
	Events  []string `json:"events"`
	Enabled bool     `json:"enabled"`
}

func (in *channelInput) target() hooks.Target {
	return hooks.Target{URL: strings.TrimSpace(in.URL), Token: strings.TrimSpace(in.Token),
		Chat: strings.TrimSpace(in.Chat), Secret: in.Secret}
}

func (in *channelInput) hasTarget() bool {
	t := in.target()
	return t.URL != "" || t.Token != "" || t.Chat != "" || t.Secret != ""
}

func (in *channelInput) validate(c *gin.Context) bool {
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Name == "" || utf8.RuneCountInString(in.Name) > 40:
		writeError(c, http.StatusBadRequest, "invalid_name", "名稱要 1 到 40 個字")
		return false
	case !slices.Contains(hooks.Kinds, in.Kind):
		writeError(c, http.StatusBadRequest, "invalid_kind", "不支援這種頻道")
		return false
	}
	seen := map[string]bool{}
	events := []string{}
	for _, e := range in.Events {
		if !hooks.ValidEvent(e) {
			writeError(c, http.StatusBadRequest, "invalid_event", "沒有這個事件："+e)
			return false
		}
		if !seen[e] {
			seen[e] = true
			events = append(events, e)
		}
	}
	in.Events = events
	return true
}

func channelID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusNotFound, "not_found", "找不到這個頻道")
		return 0, false
	}
	return id, true
}

func (h adminHandlers) createChannel(c *gin.Context) {
	me, ok := mustAdmin(c)
	if !ok {
		return
	}
	var in channelInput
	if !bindJSON(c, &in) || !in.validate(c) {
		return
	}
	hint, err := hooks.Check(in.Kind, in.target())
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_target", err.Error())
		return
	}
	enc, err := hooks.SealTarget(h.sealer, in.target())
	if err != nil {
		writeError(c, http.StatusConflict, "no_key", err.Error())
		return
	}
	ctx := c.Request.Context()
	var id int64
	if err := h.db.QueryRowContext(ctx, `
		INSERT INTO app.notify_channel (name, kind, target_enc, target_hint, events, enabled)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		in.Name, in.Kind, enc, hint, in.Events, in.Enabled).Scan(&id); err != nil {
		adminFail(c, err)
		return
	}
	h.auditChannel(ctx, c, me.ID, "create", id)
	c.Status(http.StatusNoContent)
}

func (h adminHandlers) updateChannel(c *gin.Context) {
	me, ok := mustAdmin(c)
	if !ok {
		return
	}
	id, ok := channelID(c)
	if !ok {
		return
	}
	var in channelInput
	if !bindJSON(c, &in) || !in.validate(c) {
		return
	}
	ctx := c.Request.Context()
	var kind string
	if err := h.db.QueryRowContext(ctx, `SELECT kind FROM app.notify_channel WHERE id = $1`, id).Scan(&kind); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(c, http.StatusNotFound, "not_found", "找不到這個頻道")
			return
		}
		adminFail(c, err)
		return
	}
	var err error
	if in.hasTarget() || kind != in.Kind {
		hint, cerr := hooks.Check(in.Kind, in.target())
		if cerr != nil {
			writeError(c, http.StatusBadRequest, "invalid_target", cerr.Error())
			return
		}
		enc, serr := hooks.SealTarget(h.sealer, in.target())
		if serr != nil {
			writeError(c, http.StatusConflict, "no_key", serr.Error())
			return
		}
		_, err = h.db.ExecContext(ctx, `
			UPDATE app.notify_channel SET name = $2, kind = $3, target_enc = $4, target_hint = $5, events = $6,
			       enabled = $7, updated_at = now() WHERE id = $1`,
			id, in.Name, in.Kind, enc, hint, in.Events, in.Enabled)
	} else {
		_, err = h.db.ExecContext(ctx, `
			UPDATE app.notify_channel SET name = $2, events = $3, enabled = $4, updated_at = now() WHERE id = $1`,
			id, in.Name, in.Events, in.Enabled)
	}
	if err != nil {
		adminFail(c, err)
		return
	}
	h.auditChannel(ctx, c, me.ID, "update", id)
	c.Status(http.StatusNoContent)
}

func (h adminHandlers) deleteChannel(c *gin.Context) {
	me, ok := mustAdmin(c)
	if !ok {
		return
	}
	id, ok := channelID(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	res, err := h.db.ExecContext(ctx, `DELETE FROM app.notify_channel WHERE id = $1`, id)
	if err != nil {
		adminFail(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(c, http.StatusNotFound, "not_found", "找不到這個頻道")
		return
	}
	h.auditChannel(ctx, c, me.ID, "delete", id)
	c.Status(http.StatusNoContent)
}

func (h adminHandlers) testChannel(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	id, ok := channelID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()
	var kind string
	var enc []byte
	if err := h.db.QueryRowContext(ctx, `SELECT kind, target_enc FROM app.notify_channel WHERE id = $1`, id).Scan(&kind, &enc); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(c, http.StatusNotFound, "not_found", "找不到這個頻道")
			return
		}
		adminFail(c, err)
		return
	}
	t, err := hooks.OpenTarget(h.sealer, enc)
	if err != nil {
		writeError(c, http.StatusConflict, "no_key", err.Error())
		return
	}
	client := hooks.SafeClient()
	if h.hooks != nil && h.hooks.Client != nil {
		client = h.hooks.Client
	}
	msg := hooks.Message{Title: "測試訊息", Text: "臺灣儀表板的通知會送到這個頻道。", Level: hooks.LevelInfo, URL: h.site}
	err = hooks.Send(ctx, client, kind, t, "test", msg)
	h.recordTest(c.Request.Context(), id, msg, err)
	if err != nil {
		writeError(c, http.StatusBadGateway, "send_failed", "送出失敗："+err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

func (h adminHandlers) auditChannel(ctx context.Context, c *gin.Context, adminID int64, action string, id int64) {
	if err := h.store.Audit(ctx, &adminID, "admin.channel."+action, "notify_channel:"+strconv.FormatInt(id, 10), nil, c.ClientIP()); err != nil {
		slog.Error("audit channel", "action", action, "error", err)
	}
}

func (h adminHandlers) recordTest(ctx context.Context, channelID int64, m hooks.Message, sendErr error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return
	}
	status, reason := "sent", ""
	if sendErr != nil {
		status, reason = "failed", sendErr.Error()
	}
	if _, err := h.db.ExecContext(ctx, `
		INSERT INTO app.notify_outbox (channel_id, event, message, status, attempts, last_error, sent_at)
		VALUES ($1, 'test', $2, $3, 1, left($4, 500), CASE WHEN $3 = 'sent' THEN now() END)`,
		channelID, raw, status, reason); err != nil {
		slog.Error("record channel test", "error", err)
	}
}
