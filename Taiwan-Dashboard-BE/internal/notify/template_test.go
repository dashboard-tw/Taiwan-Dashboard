package notify_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"dashboard.tw/backend/internal/notify"
	"dashboard.tw/backend/internal/testdb"
)

func TestSeededTemplates(t *testing.T) {
	db := testdb.App(t, testdb.New(t))
	ctx := context.Background()
	rows, err := db.QueryContext(ctx, `SELECT key, subject, body FROM app.mail_template`)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for rows.Next() {
		var key, subject, body string
		if err := rows.Scan(&key, &subject, &body); err != nil {
			t.Fatal(err)
		}
		seen[key] = true
		if _, ok := notify.Vars[key]; !ok {
			t.Errorf("template %s has no variables defined in code", key)
		}
		if err := notify.Check(key, subject, body); err != nil {
			t.Errorf("template %s: %v", key, err)
		}
	}
	rows.Close()
	for key := range notify.Vars {
		if !seen[key] {
			t.Errorf("no template row for %s", key)
		}
	}

	data := notify.Sample("app_rejected")
	subject, body, err := notify.Render(ctx, db, "app_rejected", data)
	if err != nil {
		t.Fatal(err)
	}
	if subject != "應用審查未通過：雨天出門提醒" {
		t.Errorf("subject = %q", subject)
	}
	if !strings.HasPrefix(body, "王小明 您好：") || !strings.Contains(body, "原因：網址無法開啟") ||
		!strings.HasSuffix(body, "這封信由系統自動寄出，請不要直接回覆。") {
		t.Errorf("body = %q", body)
	}

	if _, err := db.ExecContext(ctx, `UPDATE app.mail_template SET enabled = false WHERE key = 'signup'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := notify.Render(ctx, db, "signup", notify.Sample("signup")); !errors.Is(err, notify.ErrDisabled) {
		t.Errorf("disabled template: err = %v", err)
	}
}

func TestCheck(t *testing.T) {
	for _, c := range []struct {
		key, subject, body string
		ok                 bool
	}{
		{"signup", "歡迎 {{.Name}}", "{{.Provider}} {{.Site}}", true},
		{"signup", "歡迎", "{{.App}}", false},
		{"signup", "歡迎", "{{.Name", false},
		{"app_rejected", "{{.App}}", "原因：{{.Note}}", true},
	} {
		if err := notify.Check(c.key, c.subject, c.body); (err == nil) != c.ok {
			t.Errorf("Check(%q, %q, %q) = %v", c.key, c.subject, c.body, err)
		}
	}
}

func TestCheckMessage(t *testing.T) {
	err := notify.Check("signup", "歡迎", "{{.App}}")
	if err == nil || err.Error() != "內容用到 {{.App}}，這封信沒有這個變數" {
		t.Errorf("err = %v", err)
	}
}
