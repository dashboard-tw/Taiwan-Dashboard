package api

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/notify"
)

type brevoEvent struct {
	Event  string `json:"event"`
	Email  string `json:"email"`
	Reason string `json:"reason"`
}

func brevoWebhook(n *notify.Notifier, token string) gin.HandlerFunc {
	want := []byte("Bearer " + token)
	return func(c *gin.Context) {
		got := []byte(c.GetHeader("Authorization"))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			writeError(c, http.StatusUnauthorized, "unauthorized", "token 不正確")
			return
		}
		raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_body", "讀取內容失敗")
			return
		}
		var events []brevoEvent
		if raw = bytes.TrimSpace(raw); len(raw) > 0 && raw[0] == '[' {
			err = json.Unmarshal(raw, &events)
		} else {
			var e brevoEvent
			err = json.Unmarshal(raw, &e)
			events = []brevoEvent{e}
		}
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_body", "內容不是 JSON")
			return
		}
		for _, e := range events {
			kind := strings.ToLower(e.Event)
			if !notify.SuppressReasons[kind] {
				continue
			}
			detail := e.Reason
			if err := n.Suppress(c.Request.Context(), e.Email, kind, detail); err != nil {
				slog.Error("brevo webhook: suppress", "event", kind, "error", err)
				writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
				return
			}
			slog.Info("brevo webhook: address suppressed", "event", kind)
		}
		c.Status(http.StatusNoContent)
	}
}
