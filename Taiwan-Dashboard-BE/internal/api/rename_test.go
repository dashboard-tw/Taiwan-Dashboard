package api

import (
	"net/http"
	"strings"
	"testing"

	"dashboard.tw/backend/internal/auth"
)

func TestCleanDisplayName(t *testing.T) {
	for in, want := range map[string]string{
		"  小明  ":                "小明",
		"Alice":                 "Alice",
		"陳 小 明":                 "陳 小 明",
		"🐱 Octo":                "🐱 Octo",
		strings.Repeat("字", 30): strings.Repeat("字", 30),
	} {
		if got, ok := cleanDisplayName(in); !ok || got != want {
			t.Errorf("cleanDisplayName(%q) = %q, %v", in, got, ok)
		}
	}
	for _, bad := range []string{
		"", "   ",
		strings.Repeat("字", 31),
		"Kev​in",      // 零寬空白
		"‮nimda",      // 由右至左覆寫，看起來像 admin
		"line\nbreak", // 控制字元
		"tab\there",
		"bad\xffutf8",
	} {
		if got, ok := cleanDisplayName(bad); ok {
			t.Errorf("cleanDisplayName(%q) accepted as %q", bad, got)
		}
	}
}

func TestRename(t *testing.T) {
	e := newMeEnv(t)
	a := e.signInAs(t, auth.RoleUser, "active")
	b := e.signInAs(t, auth.RoleUser, "active")
	if w := e.send("PUT", "/api/v1/me/account", "", map[string]any{"display_name": "x"}); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous rename: %d", w.Code)
	}
	if w := e.call("PUT", "/api/v1/me/account", a, "https://evil.example"); w.Code != http.StatusForbidden {
		t.Errorf("cross-site rename: %d", w.Code)
	}
	if w := e.send("PUT", "/api/v1/me/account", a, map[string]any{"display_name": "Kev​in"}); w.Code != http.StatusBadRequest {
		t.Errorf("invisible characters: %d", w.Code)
	}
	w := e.send("PUT", "/api/v1/me/account", a, map[string]any{"display_name": "  新名字 "})
	if w.Code != http.StatusOK || decode[map[string]string](t, w)["display_name"] != "新名字" {
		t.Fatalf("rename: %d %s", w.Code, w.Body)
	}
	if me := decode[auth.User](t, e.send("GET", "/api/v1/me", a, nil)); me.DisplayName != "新名字" {
		t.Errorf("after rename /me = %q", me.DisplayName)
	}
	if me := decode[auth.User](t, e.send("GET", "/api/v1/me", b, nil)); me.DisplayName == "新名字" {
		t.Error("renamed another user too")
	}
	if n := e.count(t, `SELECT count(*) FROM app.audit_log WHERE action = 'account.rename'`); n != 1 {
		t.Errorf("audit rows = %d", n)
	}
}
