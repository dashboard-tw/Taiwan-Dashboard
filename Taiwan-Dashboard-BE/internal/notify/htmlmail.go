package notify

import (
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	discordURL = "https://discord.gg/gU5gndueZH"
	githubURL  = "https://github.com/dashboard-tw"
	fontStack  = `'Noto Sans TC','PingFang TC','Microsoft JhengHei',sans-serif`
)

var buttonLabels = map[string]string{
	"/me/devices":  "管理登入裝置",
	"/me/api-key":  "前往憑證管理",
	"/me/showcase": "查看我的應用",
	"/me":          "前往會員中心",
	"/showcase":    "查看應用活化",
	"/admin/apps":  "前往審核",
}

var (
	linkRe   = regexp.MustCompile(`https?://[^\s<>"'）」]+|[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	bulletRe = regexp.MustCompile(`^[•・\-*]\s*([^：:]{1,20})[：:]\s*(.*)$`)
)

func linkify(s, color string) string {
	var b strings.Builder
	last := 0
	for _, m := range linkRe.FindAllStringIndex(s, -1) {
		b.WriteString(html.EscapeString(s[last:m[0]]))
		target := s[m[0]:m[1]]
		href := target
		if !strings.HasPrefix(target, "http") {
			href = "mailto:" + target
		}
		b.WriteString(`<a href="` + html.EscapeString(href) + `" style="color:` + color + `;word-break:break-all;">` + html.EscapeString(target) + `</a>`)
		last = m[1]
	}
	b.WriteString(html.EscapeString(s[last:]))
	return b.String()
}

func isURLLine(s string) bool {
	return (strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")) && !strings.ContainsAny(s, " \t")
}

func buttonLabel(raw, site string) string {
	u, err := url.Parse(raw)
	if err == nil {
		if base, err := url.Parse(site); err == nil && u.Host == base.Host {
			if l, ok := buttonLabels[strings.TrimRight(u.Path, "/")]; ok {
				return l
			}
		}
	}
	return "開啟連結"
}

func textStyle(margin string) string {
	return `margin:` + margin + `;font-size:15px;line-height:1.75;color:#1f2933;`
}

func infoTable(rows [][2]string) string {
	var b strings.Builder
	b.WriteString(`<table role="presentation" class="info" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f6f8;border-radius:6px;margin:0 0 20px;"><tr><td style="padding:14px 18px;">`)
	b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="font-size:14px;line-height:1.9;">`)
	for _, r := range rows {
		b.WriteString(`<tr><td class="label" width="96" valign="top" style="color:#5b6675;padding-right:12px;white-space:nowrap;">` + html.EscapeString(r[0]) +
			`</td><td class="val" style="color:#1f2933;word-break:break-all;">` + linkify(r[1], "#2468cc") + `</td></tr>`)
	}
	b.WriteString(`</table></td></tr></table>`)
	return b.String()
}

func button(href, label string, more bool) string {
	gap := "0"
	if more {
		gap = "24px"
	}
	h := html.EscapeString(href)
	return `<table role="presentation" cellpadding="0" cellspacing="0" class="btn" style="margin:4px 0 0;"><tr><td style="border-radius:6px;background:#2a6fd0;">` +
		`<a href="` + h + `" style="display:inline-block;padding:12px 28px;font-size:15px;font-weight:700;color:#ffffff;text-decoration:none;">` + html.EscapeString(label) + `</a></td></tr></table>` +
		`<p class="muted" style="margin:16px 0 ` + gap + `;font-size:12px;line-height:1.6;color:#8a94a0;">按鈕無法使用時，請開啟這個網址：<br>` +
		`<a href="` + h + `" style="color:#2468cc;word-break:break-all;">` + h + `</a></p>`
}

func bodyHTML(body, site string) (string, string) {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(body), "\r\n", "\n"), "\n")
	lastURL, lastText := -1, -1
	for i, l := range lines {
		l = strings.TrimSpace(l)
		if isURLLine(l) {
			lastURL = i
		}
		if l != "" {
			lastText = i
		}
	}
	var b strings.Builder
	var para []string
	var rows [][2]string
	preheader := ""
	flushPara := func() {
		if len(para) == 0 {
			return
		}
		text := strings.Join(para, "\n")
		if preheader == "" && !strings.HasSuffix(strings.TrimSpace(para[0]), "您好：") {
			preheader = text
		}
		parts := make([]string, len(para))
		for i, p := range para {
			parts[i] = linkify(p, "#2468cc")
		}
		b.WriteString(`<p class="text" style="` + textStyle("0 0 14px") + `">` + strings.Join(parts, "<br>") + `</p>`)
		para = nil
	}
	flushRows := func() {
		if len(rows) > 0 {
			b.WriteString(infoTable(rows))
			rows = nil
		}
	}
	for i, raw := range lines {
		l := strings.TrimSpace(raw)
		switch {
		case l == "":
			flushPara()
			flushRows()
		case i == lastURL:
			flushPara()
			flushRows()
			b.WriteString(button(l, buttonLabel(l, site), i < lastText))
		case bulletRe.MatchString(l):
			flushPara()
			m := bulletRe.FindStringSubmatch(l)
			rows = append(rows, [2]string{strings.TrimSpace(m[1]), strings.TrimSpace(m[2])})
		default:
			flushRows()
			para = append(para, l)
		}
	}
	flushPara()
	flushRows()
	preheader = strings.Join(strings.Fields(preheader), " ")
	if utf8.RuneCountInString(preheader) > 90 {
		preheader = string([]rune(preheader)[:90]) + "…"
	}
	return b.String(), preheader
}

func footerHTML(footer string) string {
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(footer, "\r\n", "\n"), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.Trim(l, "-—–_ ") == "" {
			continue
		}
		lines = append(lines, linkify(l, "#6b7480"))
	}
	return strings.Join(lines, "<br>")
}

func renderHTML(subject, body, footer, site string) string {
	content, preheader := bodyHTML(body, site)
	logo := site + "/mail/logo-light.png"
	logoDark := site + "/mail/logo-dark.png"
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="zh-Hant"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1">` +
		`<meta name="color-scheme" content="light dark"><meta name="supported-color-schemes" content="light dark">` +
		`<title>` + html.EscapeString(subject) + `</title><style>` +
		`body{margin:0;padding:0;background:#eef0f2;}` +
		`@media (max-width:600px){.card{padding:24px 20px !important;}}` +
		`@media (prefers-color-scheme:dark){body,.bg{background:#111315 !important;}.card{background:#1e2022 !important;}` +
		`.text,.h1,.val{color:#e7e9ec !important;}.muted,.label,.foot{color:#9aa3ad !important;}.info{background:#282a2c !important;}` +
		`.logo-light{display:none !important;}.logo-dark{display:inline-block !important;}}` +
		`</style></head><body style="margin:0;padding:0;background:#eef0f2;">`)
	b.WriteString(`<div style="display:none;max-height:0;overflow:hidden;opacity:0;mso-hide:all;">` + html.EscapeString(preheader) + `</div>`)
	b.WriteString(`<table role="presentation" class="bg" width="100%" cellpadding="0" cellspacing="0" style="background:#eef0f2;"><tr><td align="center" style="padding:32px 12px;">`)
	b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;">`)
	b.WriteString(`<tr><td style="padding:0 4px 16px;">` +
		`<img class="logo-light" src="` + html.EscapeString(logo) + `" width="192" height="60" alt="臺灣儀表板 Taiwan Dashboard" style="display:inline-block;border:0;">` +
		`<img class="logo-dark" src="` + html.EscapeString(logoDark) + `" width="192" height="60" alt="" style="display:none;mso-hide:all;border:0;">` +
		`</td></tr>`)
	b.WriteString(`<tr><td><table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border-radius:8px;overflow:hidden;"><tr>`)
	for _, s := range []struct{ w, c string }{{"40%", "#2f7ce0"}, {"25%", "#22b8a6"}, {"20%", "#f5b638"}, {"15%", "#f06a5a"}} {
		b.WriteString(`<td width="` + s.w + `" height="4" style="background:` + s.c + `;font-size:0;line-height:0;">&nbsp;</td>`)
	}
	b.WriteString(`</tr><tr><td colspan="4" class="card" style="background:#ffffff;padding:32px 36px;font-family:` + fontStack + `;">`)
	b.WriteString(`<h1 class="h1" style="margin:0 0 20px;font-size:20px;line-height:1.4;color:#0d1b2e;font-weight:700;">` + html.EscapeString(subject) + `</h1>`)
	b.WriteString(content)
	b.WriteString(`</td></tr></table></td></tr>`)
	b.WriteString(`<tr><td style="padding:24px 4px 4px;"><table role="presentation" cellpadding="0" cellspacing="0"><tr>` +
		`<td style="padding-right:14px;"><a href="` + discordURL + `"><img src="` + html.EscapeString(site) + `/mail/discord.png" width="24" height="24" alt="Discord" style="display:block;border:0;"></a></td>` +
		`<td><a href="` + githubURL + `"><img src="` + html.EscapeString(site) + `/mail/github.png" width="24" height="24" alt="GitHub" style="display:block;border:0;"></a></td>` +
		`</tr></table></td></tr>`)
	if f := footerHTML(footer); f != "" {
		b.WriteString(`<tr><td class="foot" style="padding:8px 4px 0;font-family:` + fontStack + `;font-size:12px;line-height:1.8;color:#6b7480;">` + f + `</td></tr>`)
	}
	b.WriteString(`</table></td></tr></table></body></html>`)
	return b.String()
}
