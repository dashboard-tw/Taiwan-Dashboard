package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/mail"
	"dashboard.tw/backend/internal/notify"
	"dashboard.tw/backend/internal/testdb"
)

type nopSender struct{}

func (nopSender) Send(context.Context, mail.Message) error { return nil }

func TestBrevoWebhook(t *testing.T) {
	db := testdb.App(t, testdb.New(t))
	vault, err := auth.NewEmailVault(bytes.Repeat([]byte{5}, auth.EmailKeySize))
	if err != nil {
		t.Fatal(err)
	}
	n := notify.New(nopSender{}, &auth.Store{DB: db, Emails: vault}, "https://dashboard.tw")
	token := strings.Repeat("t", 32)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/hook", brevoWebhook(n, token))
	post := func(auth, body string) int {
		req := httptest.NewRequest("POST", "/hook", strings.NewReader(body))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	bounce := `{"event":"hard_bounce","email":"Gone@Example.com","reason":"mailbox does not exist"}`
	if code := post("", bounce); code != http.StatusUnauthorized {
		t.Errorf("no token: %d", code)
	}
	if code := post("Bearer wrong", bounce); code != http.StatusUnauthorized {
		t.Errorf("wrong token: %d", code)
	}
	if code := post("Bearer "+token, bounce); code != http.StatusNoContent {
		t.Fatalf("bounce: %d", code)
	}
	batch := `[{"event":"delivered","email":"ok@example.com"},{"event":"spam","email":"angry@example.com"}]`
	if code := post("Bearer "+token, batch); code != http.StatusNoContent {
		t.Fatalf("batch: %d", code)
	}
	rows, err := db.Query(`SELECT reason, detail FROM app.mail_suppression ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var reason, detail string
		if err := rows.Scan(&reason, &detail); err != nil {
			t.Fatal(err)
		}
		got = append(got, reason+":"+detail)
	}
	if strings.Join(got, ",") != "hard_bounce:mailbox does not exist,spam:" {
		t.Errorf("suppressed = %v", got)
	}
	var plain int
	if err := db.QueryRow(`SELECT count(*) FROM app.mail_suppression WHERE email_lookup::text ILIKE '%gone%'`).Scan(&plain); err != nil || plain != 0 {
		t.Errorf("stored the address in plain text: %d %v", plain, err)
	}
}
