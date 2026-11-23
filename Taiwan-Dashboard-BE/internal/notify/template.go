package notify

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

type Var struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Sample string `json:"sample"`
}

var common = []Var{
	{"Name", "收件人名稱", "王小明"},
	{"Site", "網站網址", "https://dashboard.tw"},
}

var Vars = map[string][]Var{
	"signup":          {{"Provider", "登入方式", "GitHub"}},
	"new_device":      {{"Time", "登入時間", "2026/11/23 21:05"}, {"Device", "裝置", "Chrome／Windows"}, {"IP", "IP", "203.0.113.8"}},
	"link":            {{"Provider", "登入方式", "Google"}, {"Time", "時間", "2026/11/23 21:05"}},
	"unlink":          {{"Provider", "登入方式", "Google"}, {"Time", "時間", "2026/11/23 21:05"}},
	"account_deleted": {{"Time", "刪除時間", "2026/11/23 21:05"}},
	"api_usage": {
		{"Requests", "今日呼叫次數", "4,012"}, {"RequestLimit", "每日次數上限", "5,000"},
		{"TrafficMB", "今日流量（MB）", "120.5"}, {"TrafficLimitMB", "每日流量上限（MB）", "500"},
	},
	"api_key_created":  {{"Key", "憑證名稱", "我的天氣 App"}, {"ClientID", "Client ID", "dtw_3f9a…"}, {"Time", "時間", "2026/11/23 21:05"}},
	"api_key_disabled": {{"Key", "憑證名稱", "我的天氣 App"}, {"ClientID", "Client ID", "dtw_3f9a…"}, {"Time", "時間", "2026/11/23 21:05"}},
	"api_key_deleted":  {{"Key", "憑證名稱", "我的天氣 App"}, {"ClientID", "Client ID", "dtw_3f9a…"}, {"Time", "時間", "2026/11/23 21:05"}},
	"api_key_revoked":  {{"Key", "憑證名稱", "我的天氣 App"}, {"Time", "時間", "2026/11/23 21:05"}},
	"app_approved":     {{"App", "應用名稱", "雨天出門提醒"}},
	"app_rejected":     {{"App", "應用名稱", "雨天出門提醒"}, {"Note", "退回原因", "網址無法開啟"}},
	"app_removed":      {{"App", "應用名稱", "雨天出門提醒"}, {"Note", "下架原因", "網站已停止服務"}},
	"app_submitted":    {{"App", "應用名稱", "雨天出門提醒"}, {"Author", "投稿者", "王小明"}},
	"app_resubmitted":  {{"App", "應用名稱", "雨天出門提醒"}, {"Author", "投稿者", "王小明"}},
	"footer":           {},
}

func VarsOf(key string) []Var {
	return append(append([]Var{}, common...), Vars[key]...)
}

func Sample(key string) map[string]string {
	data := map[string]string{}
	for _, v := range VarsOf(key) {
		data[v.Key] = v.Sample
	}
	return data
}

var ErrDisabled = errors.New("this mail is turned off")

func parse(name, text string) (*template.Template, error) {
	return template.New(name).Option("missingkey=error").Parse(text)
}

func execute(t *template.Template, data map[string]string) (string, error) {
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

var missingKeyRe = regexp.MustCompile(`no entry for key "([^"]+)"`)

func Check(key, subject, body string) error {
	data := Sample(key)
	for _, part := range []struct{ name, text string }{{"主旨", subject}, {"內容", body}} {
		t, err := parse(part.name, part.text)
		if err != nil {
			return fmt.Errorf("%s的格式不對，檢查 {{ 和 }} 有沒有成對", part.name)
		}
		if _, err := execute(t, data); err != nil {
			if m := missingKeyRe.FindStringSubmatch(err.Error()); m != nil {
				return fmt.Errorf("%s用到 {{.%s}}，這封信沒有這個變數", part.name, m[1])
			}
			return fmt.Errorf("%s的格式不對：%v", part.name, err)
		}
	}
	if strings.ContainsAny(subject, "\r\n") {
		return errors.New("主旨不能換行")
	}
	return nil
}

type stored struct {
	key, subject, body, footer string
	enabled                    bool
}

func load(ctx context.Context, db *sql.DB, key string) (*stored, error) {
	t := stored{key: key}
	err := db.QueryRowContext(ctx, `
		SELECT t.subject, t.body, t.enabled, COALESCE(f.body, '')
		  FROM app.mail_template t LEFT JOIN app.mail_template f ON f.key = 'footer'
		 WHERE t.key = $1`, key).Scan(&t.subject, &t.body, &t.enabled, &t.footer)
	if err != nil {
		return nil, fmt.Errorf("mail template %s: %w", key, err)
	}
	return &t, nil
}

func (t *stored) render(data map[string]string) (subject, body string, err error) {
	parts := []struct{ name, text string }{{"subject", t.subject}, {"body", t.body}, {"footer", t.footer}}
	out := make([]string, len(parts))
	for i, p := range parts {
		if p.text == "" {
			continue
		}
		tmpl, err := parse(p.name, p.text)
		if err == nil {
			out[i], err = execute(tmpl, data)
		}
		if err != nil {
			return "", "", fmt.Errorf("mail template %s %s: %w", t.key, p.name, err)
		}
	}
	body = strings.TrimRight(out[1], "\n")
	if out[2] != "" {
		body += "\n\n" + out[2]
	}
	return strings.TrimSpace(out[0]), body, nil
}

func Render(ctx context.Context, db *sql.DB, key string, data map[string]string) (subject, body string, err error) {
	t, err := load(ctx, db, key)
	if err != nil {
		return "", "", err
	}
	if !t.enabled {
		return "", "", ErrDisabled
	}
	return t.render(data)
}

func Preview(ctx context.Context, db *sql.DB, key, subject, body, site string) (string, string, error) {
	t := stored{key: key, subject: subject, body: body}
	if key != "footer" {
		if err := db.QueryRowContext(ctx, `SELECT body FROM app.mail_template WHERE key = 'footer'`).Scan(&t.footer); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", "", err
		}
	}
	data := Sample(key)
	data["Site"] = site
	return t.render(data)
}
