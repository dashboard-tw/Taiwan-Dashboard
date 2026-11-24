package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const brevoEndpoint = "https://api.brevo.com/v3/smtp/email"

type Brevo struct {
	APIKey   string
	From     *mail.Address
	Endpoint string
	Client   *http.Client
	Sandbox  bool
}

func NewBrevo(apiKey, from, fromName string) (*Brevo, error) {
	if apiKey == "" {
		return nil, errors.New("brevo api key is empty")
	}
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("mail from: %w", err)
	}
	if fromName != "" {
		addr.Name = fromName
	}
	return &Brevo{APIKey: apiKey, From: addr, Endpoint: brevoEndpoint, Client: &http.Client{Timeout: 20 * time.Second}}, nil
}

type brevoAddr struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type brevoBody struct {
	Sender      brevoAddr         `json:"sender"`
	To          []brevoAddr       `json:"to"`
	Subject     string            `json:"subject"`
	TextContent string            `json:"textContent"`
	HTMLContent string            `json:"htmlContent"`
	Tags        []string          `json:"tags,omitempty"`
	Headers     map[string]string `json:"headers"`
}

func (b *Brevo) Send(ctx context.Context, m Message) error {
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return fmt.Errorf("mail to: %w", err)
	}
	key, err := idempotencyKey(m.ID)
	if err != nil {
		return err
	}
	body := brevoBody{
		Sender:      brevoAddr{Email: b.From.Address, Name: b.From.Name},
		To:          []brevoAddr{{Email: to.Address, Name: to.Name}},
		Subject:     m.Subject,
		TextContent: m.Text,
		HTMLContent: m.HTML,
		Headers:     map[string]string{"idempotencyKey": key},
	}
	if body.HTMLContent == "" {
		body.HTMLContent = textToHTML(m.Text)
	}
	if m.Tag != "" {
		body.Tags = []string{m.Tag}
	}
	if b.Sandbox {
		body.Headers["X-Sib-Sandbox"] = "drop"
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var last error
	var wait time.Duration
	for attempt := range 3 {
		if attempt > 0 {
			if wait <= 0 {
				wait = time.Duration(attempt*2) * time.Second
			}
			select {
			case <-ctx.Done():
				return errors.Join(last, ctx.Err())
			case <-time.After(wait):
			}
		}
		var retry bool
		retry, wait, err = b.post(ctx, payload)
		if err == nil {
			return nil
		}
		last = err
		if !retry {
			return err
		}
	}
	return last
}

func idempotencyKey(id string) (string, error) {
	b := make([]byte, 16)
	if id == "" {
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
	} else {
		sum := sha256.Sum256([]byte("dashboard.tw mail " + id))
		copy(b, sum[:16])
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

func (b *Brevo) post(ctx context.Context, payload []byte) (retry bool, wait time.Duration, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return false, 0, err
	}
	req.Header.Set("api-key", b.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := b.Client.Do(req)
	if err != nil {
		return true, 0, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusCreated || res.StatusCode == http.StatusAccepted {
		_, _ = io.Copy(io.Discard, res.Body)
		return false, 0, nil
	}
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	_ = json.Unmarshal(raw, &e)
	if e.Code == "duplicate_parameter" && strings.Contains(strings.ToLower(e.Message), "idempotency") {
		return false, 0, nil
	}
	err = fmt.Errorf("brevo: %s %s %s", res.Status, e.Code, e.Message)
	if res.StatusCode == http.StatusTooManyRequests {
		if sec, perr := strconv.Atoi(res.Header.Get("x-sib-ratelimit-reset")); perr == nil && sec > 0 {
			wait = min(time.Duration(sec)*time.Second, time.Minute)
		}
		return true, wait, err
	}
	return res.StatusCode >= 500, 0, err
}

var linkRe = regexp.MustCompile(`https?://[^\s<>"）」]+`)

func textToHTML(text string) string {
	var b strings.Builder
	b.WriteString(`<div style="font-family:sans-serif;font-size:15px;line-height:1.7;color:#1f2937;white-space:pre-wrap">`)
	esc := html.EscapeString(text)
	b.WriteString(linkRe.ReplaceAllStringFunc(esc, func(u string) string {
		return `<a href="` + u + `">` + u + `</a>`
	}))
	b.WriteString(`</div>`)
	return b.String()
}

func (b *Brevo) Ping(ctx context.Context) error {
	u := strings.TrimSuffix(b.Endpoint, "/smtp/email") + "/smtp/statistics/aggregatedReport?days=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("api-key", b.APIKey)
	req.Header.Set("Accept", "application/json")
	res, err := b.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	if res.StatusCode != http.StatusOK {
		var e struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		return fmt.Errorf("brevo: %s %s %s", res.Status, e.Code, e.Message)
	}
	return nil
}
