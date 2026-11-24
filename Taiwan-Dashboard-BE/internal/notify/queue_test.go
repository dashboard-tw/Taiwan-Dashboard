package notify

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/db"
	"dashboard.tw/backend/internal/mail"
	"dashboard.tw/backend/internal/testdb"
)

type fakeSender struct {
	mu   sync.Mutex
	sent []mail.Message
	fail int
}

func (f *fakeSender) Send(_ context.Context, m mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail > 0 {
		f.fail--
		return errors.New("smtp down")
	}
	f.sent = append(f.sent, m)
	return nil
}

func drain(t *testing.T, n *Notifier) {
	t.Helper()
	ctx := context.Background()
	for range 50 {
		worked, err := n.sendNext(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			return
		}
	}
	t.Fatal("queue did not drain")
}

func setup(t *testing.T) (*Notifier, *fakeSender, *auth.Store) {
	db := testdb.App(t, testdb.New(t))
	vault, err := auth.NewEmailVault(bytes.Repeat([]byte{7}, auth.EmailKeySize))
	if err != nil {
		t.Fatal(err)
	}
	store := &auth.Store{DB: db, Emails: vault}
	f := &fakeSender{}
	n := New(f, store, "https://dashboard.tw")
	store.OnEvent = n.OnEvent
	return n, f, store
}

func TestQueueSignupAndRetry(t *testing.T) {
	n, f, store := setup(t)
	ctx := context.Background()
	f.fail = 1
	if _, _, err := store.SignInExternal(ctx, "github", auth.Identity{Subject: "1", Name: "Octo", Email: "octo@example.com"}); err != nil {
		t.Fatal(err)
	}
	drain(t, n)
	if len(f.sent) != 0 {
		t.Fatalf("sent %d before the retry time", len(f.sent))
	}
	dueNow(t, store)
	drain(t, n)
	if len(f.sent) != 1 || f.sent[0].To != "octo@example.com" || f.sent[0].Subject != "歡迎使用臺灣儀表板" || f.sent[0].Tag != "signup" {
		t.Fatalf("sent = %+v", f.sent)
	}
	if !strings.Contains(f.sent[0].Text, "Octo 您好") || !strings.Contains(f.sent[0].Text, "透過 GitHub 完成帳號建立") {
		t.Errorf("text = %q", f.sent[0].Text)
	}
	var status string
	var attempts int
	var enc []byte
	if err := db.AsSystem(ctx, store.DB, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT status, attempts, email_enc FROM app.mail_queue`).Scan(&status, &attempts, &enc)
	}); err != nil {
		t.Fatal(err)
	}
	if status != "sent" || attempts != 2 {
		t.Errorf("status = %s attempts = %d", status, attempts)
	}
	if sent, err := n.SentToday(ctx); err != nil || sent != 1 {
		t.Errorf("SentToday = %d, %v", sent, err)
	}
}

func TestQueueMergesSubmissions(t *testing.T) {
	n, f, store := setup(t)
	ctx := context.Background()
	admin, err := store.CreateAdmin(ctx, "Admin")
	if err != nil {
		t.Fatal(err)
	}
	enc, _ := store.Emails.Seal(admin.ID, "admin@example.com")
	if _, err := store.DB.ExecContext(ctx, `UPDATE app.users SET email_enc = $2, email_hash = $3, email_lookup = $4 WHERE id = $1`,
		admin.ID, enc, store.Emails.Fingerprint(admin.ID, "admin@example.com"), store.Emails.Lookup("admin@example.com")); err != nil {
		t.Fatal(err)
	}
	n.AppSubmitted(admin.ID, "甲", "小明", false)
	n.AppSubmitted(admin.ID, "乙", "小明", false)
	n.AppSubmitted(admin.ID, "丙", "小華", false)
	drain(t, n)
	if len(f.sent) != 0 {
		t.Fatal("submissions were sent before the merge window ended")
	}
	dueNow(t, store)
	drain(t, n)
	if len(f.sent) != 2 {
		t.Fatalf("sent %d mails, want one merged mail for admins and one receipt", len(f.sent))
	}
	byTag := map[string]string{}
	for _, m := range f.sent {
		byTag[m.Tag] = m.Text
	}
	if !strings.Contains(byTag["app_submitted"], "開發者 小明、小華 已提交新應用「甲」、「乙」、「丙」") {
		t.Errorf("admin text = %q", byTag["app_submitted"])
	}
	if !strings.Contains(byTag["app_received"], "我們已收到您投稿的應用「甲」、「乙」、「丙」") {
		t.Errorf("receipt text = %q", byTag["app_received"])
	}
}

func TestQueueSkipsAndDeletedAccount(t *testing.T) {
	n, f, store := setup(t)
	ctx := context.Background()
	u, _, err := store.SignInExternal(ctx, "github", auth.Identity{Subject: "2", Name: "Bye", Email: "bye@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE app.mail_template SET enabled = false WHERE key = 'signup'`); err != nil {
		t.Fatal(err)
	}
	drain(t, n)
	done := n.AccountDeleting(ctx, u.ID)
	if _, err := store.DB.ExecContext(ctx, `DELETE FROM app.users WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	done()
	n.AppReviewed(u.ID, "應用", "approve", "")
	drain(t, n)
	if len(f.sent) != 1 || f.sent[0].To != "bye@example.com" || f.sent[0].Subject != "帳號刪除確認通知" {
		t.Fatalf("sent = %+v", f.sent)
	}
	var got []string
	err = db.AsSystem(ctx, store.DB, func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT template, status, email_enc IS NULL FROM app.mail_queue ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var tmpl, status string
			var cleared bool
			if err := rows.Scan(&tmpl, &status, &cleared); err != nil {
				return err
			}
			if !cleared {
				t.Errorf("%s kept the encrypted address after it finished", tmpl)
			}
			got = append(got, tmpl+":"+status)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "signup:skipped account_deleted:sent app_approved:skipped"
	if strings.Join(got, " ") != want {
		t.Errorf("queue = %v, want %s", got, want)
	}
}

func dueNow(t *testing.T, store *auth.Store) {
	t.Helper()
	err := db.AsSystem(context.Background(), store.DB, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE app.mail_queue SET next_attempt_at = now()`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueueSkipsSuppressed(t *testing.T) {
	n, f, store := setup(t)
	ctx := context.Background()
	if err := n.Suppress(ctx, "BOUNCE@example.com", "hard_bounce", "no such user"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.SignInExternal(ctx, "github", auth.Identity{Subject: "9", Name: "B", Email: "bounce@example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.SignInExternal(ctx, "github", auth.Identity{Subject: "10", Name: "C", Email: "fine@example.com"}); err != nil {
		t.Fatal(err)
	}
	drain(t, n)
	if len(f.sent) != 1 || f.sent[0].To != "fine@example.com" {
		t.Fatalf("sent = %+v", f.sent)
	}
	if err := n.Suppress(ctx, "x@example.com", "opened", ""); err != nil {
		t.Fatal(err)
	}
	var c int
	if err := store.DB.QueryRow(`SELECT count(*) FROM app.mail_suppression`).Scan(&c); err != nil || c != 1 {
		t.Errorf("suppression rows = %d, %v; an unrelated event was stored", c, err)
	}
}
