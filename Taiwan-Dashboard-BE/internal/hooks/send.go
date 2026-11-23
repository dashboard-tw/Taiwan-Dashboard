package hooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

var colors = map[string]int{LevelInfo: 0x2f7ce0, LevelOK: 0x22b8a6, LevelWarn: 0xf5b638, LevelError: 0xf06a5a}

func SafeClient() *http.Client {
	d := &net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			return fmt.Errorf("blocked address %s", host)
		}
		return nil
	}}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = d.DialContext
	t.Proxy = nil
	return &http.Client{
		Transport:     t,
		Timeout:       20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

type RetryError struct {
	After time.Duration
	Err   error
}

func (e *RetryError) Error() string { return e.Err.Error() }
func (e *RetryError) Unwrap() error { return e.Err }

type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

func Send(ctx context.Context, c *http.Client, kind string, t Target, event string, m Message) error {
	err := send(ctx, c, kind, t, event, m)
	var perm *PermanentError
	if kind == "discord" && len(m.Links) > 0 && errors.As(err, &perm) && strings.HasPrefix(perm.Error(), "400") {
		plain := m
		plain.Links = nil
		for _, l := range m.Links {
			plain.Text += "\n" + l.Label + "：" + l.URL
		}
		return send(ctx, c, kind, t, event, plain)
	}
	return err
}

func send(ctx context.Context, c *http.Client, kind string, t Target, event string, m Message) error {
	var (
		endpoint = t.URL
		body     any
		headers  = map[string]string{}
	)
	switch kind {
	case "discord":
		body = discordBody(m)
		if len(m.Links) > 0 {
			endpoint = withQuery(endpoint, "with_components", "true")
		}
	case "slack":
		body = slackBody(m)
	case "telegram":
		endpoint, body = telegramRequest(t, m)
	case "json":
		links := m.Links
		if links == nil {
			links = []Link{}
		}
		body = map[string]any{"event": event, "title": m.Title, "text": m.Text, "url": m.URL, "level": m.Level,
			"fields": nonNil(m.Fields), "thumbnail": m.Thumbnail, "image": m.Image, "links": links,
			"sent_at": time.Now().UTC().Format(time.RFC3339)}
		headers["X-Dashboard-Event"] = event
	default:
		return &PermanentError{fmt.Errorf("unknown channel kind %q", kind)}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	if kind == "json" && t.Secret != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(t.Secret))
		mac.Write([]byte(ts + "." + string(raw)))
		headers["X-Dashboard-Timestamp"] = ts
		headers["X-Dashboard-Signature"] = "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return &PermanentError{err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "dashboard.tw-notify")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := c.Do(req)
	if err != nil {
		var ue interface{ Unwrap() error }
		if errors.As(err, &ue) && ue.Unwrap() != nil {
			err = ue.Unwrap()
		}
		return err
	}
	defer res.Body.Close()
	reply, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	if res.StatusCode/100 == 2 {
		return nil
	}
	msg := fmt.Errorf("%s %s", res.Status, strings.TrimSpace(string(reply)))
	switch {
	case res.StatusCode == http.StatusTooManyRequests:
		return &RetryError{After: retryAfter(res, reply), Err: msg}
	case res.StatusCode >= 500:
		return msg
	default:
		return &PermanentError{msg}
	}
}

func retryAfter(res *http.Response, body []byte) time.Duration {
	if s, err := strconv.ParseFloat(res.Header.Get("Retry-After"), 64); err == nil && s > 0 {
		return time.Duration(s * float64(time.Second))
	}
	var v struct {
		RetryAfter float64 `json:"retry_after"`
		Parameters struct {
			RetryAfter float64 `json:"retry_after"`
		} `json:"parameters"`
	}
	if json.Unmarshal(body, &v) == nil {
		if v.RetryAfter > 0 {
			return time.Duration(v.RetryAfter * float64(time.Second))
		}
		if v.Parameters.RetryAfter > 0 {
			return time.Duration(v.Parameters.RetryAfter * float64(time.Second))
		}
	}
	return 0
}

func nonNil(f []Field) []Field {
	if f == nil {
		return []Field{}
	}
	return f
}

func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

func discordBody(m Message) map[string]any {
	embed := map[string]any{
		"title":       cut(m.Title, 256),
		"description": cut(m.Text, 4000),
		"color":       colors[m.Level],
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}
	if m.URL != "" {
		embed["url"] = m.URL
	}
	if len(m.Fields) > 0 {
		fields := make([]map[string]any, 0, len(m.Fields))
		for _, f := range m.Fields[:min(len(m.Fields), 25)] {
			fields = append(fields, map[string]any{"name": cut(f.Name, 256), "value": cut(orDash(f.Value), 1024), "inline": true})
		}
		embed["fields"] = fields
	}
	if m.Thumbnail != "" {
		embed["thumbnail"] = map[string]string{"url": m.Thumbnail}
	}
	if m.Image != "" {
		embed["image"] = map[string]string{"url": m.Image}
	}
	body := map[string]any{
		"username":         "臺灣儀表板",
		"embeds":           []any{embed},
		"allowed_mentions": map[string]any{"parse": []string{}},
	}
	if len(m.Links) > 0 {
		buttons := []map[string]any{}
		for _, l := range m.Links[:min(len(m.Links), 5)] {
			b := map[string]any{"type": 2, "style": 5, "label": cut(l.Label, 80), "url": l.URL}
			if e, ok := DiscordEmojis[l.Icon]; ok {
				b["emoji"] = map[string]string{"id": e.ID, "name": e.Name}
			}
			buttons = append(buttons, b)
		}
		body["components"] = []any{map[string]any{"type": 1, "components": buttons}}
	}
	return body
}

func slackEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func slackBody(m Message) map[string]any {
	title := "*" + slackEscape(m.Title) + "*"
	if m.URL != "" {
		title = "*<" + m.URL + "|" + slackEscape(m.Title) + ">*"
	}
	text := title
	if m.Text != "" {
		text += "\n" + slackEscape(m.Text)
	}
	section := map[string]any{"type": "section", "text": map[string]string{"type": "mrkdwn", "text": cut(text, 3000)}}
	if m.Thumbnail != "" {
		section["accessory"] = map[string]string{"type": "image", "image_url": m.Thumbnail, "alt_text": m.Title}
	}
	blocks := []any{section}
	if len(m.Fields) > 0 {
		fields := []map[string]string{}
		for _, f := range m.Fields[:min(len(m.Fields), 10)] {
			fields = append(fields, map[string]string{"type": "mrkdwn", "text": cut("*"+slackEscape(f.Name)+"*\n"+slackEscape(orDash(f.Value)), 2000)})
		}
		blocks = append(blocks, map[string]any{"type": "section", "fields": fields})
	}
	if m.Image != "" {
		blocks = append(blocks, map[string]string{"type": "image", "image_url": m.Image, "alt_text": m.Title})
	}
	if len(m.Links) > 0 {
		buttons := []any{}
		for _, l := range m.Links[:min(len(m.Links), 5)] {
			buttons = append(buttons, map[string]any{"type": "button", "text": map[string]string{"type": "plain_text", "text": cut(l.Label, 75)}, "url": l.URL})
		}
		blocks = append(blocks, map[string]any{"type": "actions", "elements": buttons})
	}
	return map[string]any{"text": cut(m.Title, 300), "blocks": blocks}
}

func telegramText(m Message) string {
	var b strings.Builder
	b.WriteString("<b>" + html.EscapeString(m.Title) + "</b>")
	if m.Text != "" {
		b.WriteString("\n" + html.EscapeString(m.Text))
	}
	if len(m.Fields) > 0 {
		b.WriteString("\n")
		for _, f := range m.Fields {
			b.WriteString("\n" + html.EscapeString(f.Name) + "：" + html.EscapeString(orDash(f.Value)))
		}
	}
	if m.URL != "" && len(m.Links) == 0 {
		b.WriteString("\n\n<a href=\"" + html.EscapeString(m.URL) + "\">打開連結</a>")
	}
	return b.String()
}

func telegramRequest(t Target, m Message) (string, map[string]any) {
	base := "https://api.telegram.org/bot" + t.Token
	body := map[string]any{"chat_id": t.Chat, "parse_mode": "HTML"}
	if len(m.Links) > 0 {
		row := []map[string]string{}
		for _, l := range m.Links[:min(len(m.Links), 5)] {
			row = append(row, map[string]string{"text": l.Label, "url": l.URL})
		}
		body["reply_markup"] = map[string]any{"inline_keyboard": [][]map[string]string{row}}
	}
	if img := m.Image; img != "" || m.Thumbnail != "" {
		if img == "" {
			img = m.Thumbnail
		}
		body["photo"] = img
		body["caption"] = cutHTML(telegramText(m), 1024)
		return base + "/sendPhoto", body
	}
	body["text"] = cutHTML(telegramText(m), 4000)
	body["link_preview_options"] = map[string]bool{"is_disabled": true}
	return base + "/sendMessage", body
}

func cutHTML(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return cut(html.UnescapeString(stripTags(s)), n)
}

func stripTags(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func withQuery(raw, k, v string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set(k, v)
	u.RawQuery = q.Encode()
	return u.String()
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
