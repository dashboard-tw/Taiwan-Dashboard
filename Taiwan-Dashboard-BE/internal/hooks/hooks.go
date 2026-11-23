package hooks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	EventAppPublished   = "app_published"
	EventAppSubmitted   = "app_submitted"
	EventPipelineFailed = "pipeline_failed"
	EventMailFailed     = "mail_failed"
)

var Events = []struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}{
	{EventAppPublished, "應用上架"},
	{EventAppSubmitted, "新投稿或修改"},
	{EventPipelineFailed, "資料管線失敗與恢復"},
	{EventMailFailed, "寄信失敗"},
}

func ValidEvent(e string) bool {
	for _, v := range Events {
		if v.Key == e {
			return true
		}
	}
	return false
}

var Kinds = []string{"discord", "slack", "telegram", "json"}

const (
	LevelInfo  = "info"
	LevelOK    = "ok"
	LevelWarn  = "warn"
	LevelError = "error"
)

type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
	Icon  string `json:"icon,omitempty"`
}

type Emoji struct {
	ID   string
	Name string
}

var DiscordEmojis = map[string]Emoji{}

func ParseEmojis(s string) (map[string]Emoji, error) {
	out := map[string]Emoji{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		f := strings.Split(part, ":")
		if len(f) != 3 || f[0] == "" || f[1] == "" || f[2] == "" {
			return nil, fmt.Errorf("DISCORD_LINK_EMOJIS: %q 要寫成 icon:名稱:ID", part)
		}
		for _, r := range f[2] {
			if r < '0' || r > '9' {
				return nil, fmt.Errorf("DISCORD_LINK_EMOJIS: %q 的 ID 要是數字", part)
			}
		}
		out[f[0]] = Emoji{ID: f[2], Name: f[1]}
	}
	return out, nil
}

type Message struct {
	Title     string  `json:"title"`
	Text      string  `json:"text"`
	URL       string  `json:"url,omitempty"`
	Level     string  `json:"level"`
	Fields    []Field `json:"fields,omitempty"`
	Thumbnail string  `json:"thumbnail,omitempty"`
	Image     string  `json:"image,omitempty"`
	Links     []Link  `json:"links,omitempty"`
}

type Target struct {
	URL    string `json:"url,omitempty"`
	Token  string `json:"token,omitempty"`
	Chat   string `json:"chat,omitempty"`
	Secret string `json:"secret,omitempty"`
}

type Sealer interface {
	Seal(id int64, plain string) ([]byte, error)
	Open(id int64, enc []byte) (string, error)
}

func SealTarget(s Sealer, t Target) ([]byte, error) {
	if s == nil {
		return nil, errors.New("沒有設定 EMAIL_KEY，無法加密保存頻道網址")
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}
	return s.Seal(0, string(raw))
}

func OpenTarget(s Sealer, enc []byte) (Target, error) {
	var t Target
	if s == nil {
		return t, errors.New("沒有設定 EMAIL_KEY，無法解開頻道網址")
	}
	raw, err := s.Open(0, enc)
	if err != nil {
		return t, err
	}
	return t, json.Unmarshal([]byte(raw), &t)
}

var (
	telegramTokenRe = regexp.MustCompile(`^\d{5,}:[A-Za-z0-9_-]{30,}$`)
	telegramChatRe  = regexp.MustCompile(`^(-?\d{3,}|@[A-Za-z0-9_]{5,})$`)
)

func Check(kind string, t Target) (hint string, err error) {
	switch kind {
	case "discord":
		u, err := httpsURL(t.URL)
		if err != nil {
			return "", err
		}
		host := strings.ToLower(u.Hostname())
		if !(host == "discord.com" || host == "discordapp.com" || strings.HasSuffix(host, ".discord.com")) ||
			!strings.HasPrefix(u.Path, "/api/webhooks/") {
			return "", errors.New("請貼上 Discord 頻道設定裡的 Webhook 網址")
		}
		return host + "/api/webhooks/…" + tail(u.Path, 4), nil
	case "slack":
		u, err := httpsURL(t.URL)
		if err != nil {
			return "", err
		}
		if strings.ToLower(u.Hostname()) != "hooks.slack.com" || !strings.HasPrefix(u.Path, "/services/") {
			return "", errors.New("請貼上 Slack 的 Incoming Webhook 網址")
		}
		return "hooks.slack.com/services/…" + tail(u.Path, 4), nil
	case "telegram":
		if !telegramTokenRe.MatchString(t.Token) {
			return "", errors.New("Bot token 格式不對，應該像 123456:ABC-DEF…")
		}
		if !telegramChatRe.MatchString(t.Chat) {
			return "", errors.New("Chat ID 要是數字（群組通常是 -100 開頭）或 @頻道名稱")
		}
		return "bot …" + tail(t.Token, 4) + " → " + t.Chat, nil
	case "json":
		u, err := httpsURL(t.URL)
		if err != nil {
			return "", err
		}
		if len(t.Secret) > 0 && len(t.Secret) < 16 {
			return "", errors.New("簽章密鑰至少 16 個字，或留空")
		}
		return u.Host + "/…", nil
	}
	return "", errors.New("不支援這種頻道")
}

func httpsURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || len(raw) > 500 {
		return nil, errors.New("網址要以 https:// 開頭")
	}
	return u, nil
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func Emit(ctx context.Context, db *sql.DB, event string, m Message) error {
	if db == nil {
		return nil
	}
	if !ValidEvent(event) {
		return fmt.Errorf("unknown event %q", event)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO app.notify_outbox (channel_id, event, message)
		SELECT id, $1, $2 FROM app.notify_channel WHERE enabled AND $1 = ANY(events)`, event, raw)
	if err != nil {
		return fmt.Errorf("emit %s: %w", event, err)
	}
	return nil
}
