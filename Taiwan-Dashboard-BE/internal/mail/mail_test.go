package mail

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	netmail "net/mail"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBuildMessage(t *testing.T) {
	s, err := NewSMTP(Config{Host: "smtp.example.com", From: "noreply@dashboard.tw", FromName: "臺灣儀表板"})
	if err != nil {
		t.Fatal(err)
	}
	to, _ := netmail.ParseAddress("user@example.com")
	raw, err := s.build(to, Message{Subject: "新裝置登入通知", Text: "第一行\n第二行"})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := netmail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	dec := new(netmail.AddressParser)
	from, err := dec.Parse(msg.Header.Get("From"))
	if err != nil || from.Name != "臺灣儀表板" || from.Address != "noreply@dashboard.tw" {
		t.Errorf("From = %q (%v)", msg.Header.Get("From"), err)
	}
	if subj, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject")); subj != "新裝置登入通知" {
		t.Errorf("Subject = %q", subj)
	}
	if !strings.HasSuffix(msg.Header.Get("Message-ID"), "@dashboard.tw>") {
		t.Errorf("Message-ID = %q", msg.Header.Get("Message-ID"))
	}
	enc, err := io.ReadAll(msg.Body)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(enc), "\r\n", ""))
	if string(b) != "第一行\r\n第二行" {
		t.Errorf("body = %q", b)
	}
	if _, err := s.build(to, Message{Subject: "a\r\nBcc: x@example.com"}); err == nil {
		t.Error("a subject with a line break was accepted")
	}
}

func TestBrevoSend(t *testing.T) {
	var calls atomic.Int32
	var got brevoBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.Header.Get("api-key") != "key" {
			t.Errorf("api-key = %q", r.Header.Get("api-key"))
		}
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"messageId":"<1@relay>"}`))
	}))
	defer srv.Close()
	b, err := NewBrevo("key", "noreply@dashboard.tw", "臺灣儀表板")
	if err != nil {
		t.Fatal(err)
	}
	b.Endpoint = srv.URL
	err = b.Send(t.Context(), Message{To: "user@example.com", Subject: "主旨", Text: "看這裡 https://dashboard.tw/me <b>", Tag: "signup"})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want a retry after 500", calls.Load())
	}
	if got.Sender.Name != "臺灣儀表板" || got.To[0].Email != "user@example.com" || got.Tags[0] != "signup" {
		t.Errorf("body = %+v", got)
	}
	if !uuidRe.MatchString(got.Headers["idempotencyKey"]) {
		t.Errorf("idempotencyKey = %q, want a UUID", got.Headers["idempotencyKey"])
	}
	if !strings.Contains(got.HTMLContent, `<a href="https://dashboard.tw/me">`) || strings.Contains(got.HTMLContent, "<b>") {
		t.Errorf("html = %q", got.HTMLContent)
	}
}

func TestBrevoClientError(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"invalid_parameter","message":"sender is invalid"}`))
	}))
	defer srv.Close()
	b, _ := NewBrevo("key", "noreply@dashboard.tw", "")
	b.Endpoint = srv.URL
	err := b.Send(t.Context(), Message{To: "user@example.com", Subject: "s", Text: "t"})
	if err == nil || !strings.Contains(err.Error(), "sender is invalid") {
		t.Errorf("err = %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("a 400 was retried %d times", calls.Load())
	}
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestIdempotencyKey(t *testing.T) {
	a, _ := idempotencyKey("mail_queue:7")
	b, _ := idempotencyKey("mail_queue:7")
	c, _ := idempotencyKey("mail_queue:8")
	r, _ := idempotencyKey("")
	if a != b || a == c || !uuidRe.MatchString(a) || !uuidRe.MatchString(r) {
		t.Errorf("keys: %s %s %s %s", a, b, c, r)
	}
}

func TestBrevoDuplicateAndRateLimit(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("x-sib-ratelimit-reset", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"duplicate_parameter","message":"idempotencyKey already used"}`))
	}))
	defer srv.Close()
	b, _ := NewBrevo("key", "noreply@dashboard.tw", "")
	b.Endpoint = srv.URL
	if err := b.Send(t.Context(), Message{To: "user@example.com", Subject: "s", Text: "t", ID: "mail_queue:1"}); err != nil {
		t.Errorf("a duplicate after 429 should count as sent: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d", calls.Load())
	}
}

func TestBrevoPing(t *testing.T) {
	status := http.StatusOK
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path + "?" + r.URL.RawQuery
		if r.Header.Get("api-key") != "key" {
			t.Errorf("api-key = %q", r.Header.Get("api-key"))
		}
		w.WriteHeader(status)
		if status != http.StatusOK {
			_, _ = w.Write([]byte(`{"code":"unauthorized","message":"Key not found"}`))
		}
	}))
	defer srv.Close()
	b, _ := NewBrevo("key", "noreply@dashboard.tw", "")
	b.Endpoint = srv.URL + "/v3/smtp/email"
	if err := b.Ping(t.Context()); err != nil {
		t.Fatal(err)
	}
	if path != "/v3/smtp/statistics/aggregatedReport?days=1" {
		t.Errorf("path = %s", path)
	}
	status = http.StatusUnauthorized
	if err := b.Ping(t.Context()); err == nil || !strings.Contains(err.Error(), "Key not found") {
		t.Errorf("err = %v", err)
	}
}
