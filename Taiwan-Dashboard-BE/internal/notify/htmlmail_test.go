package notify

import (
	"os"
	"strings"
	"testing"
)

func TestRenderHTML(t *testing.T) {
	body := `王小明 您好：

系統偵測到您的帳號剛透過一台新裝置登入，詳細資訊如下：

• 登入時間：2026/11/24 14:31
• 登入地點：臺灣・桃園
• IP 位址：203.0.113.8

若非您本人操作，請立即前往以下連結：
https://dashboard.tw/me/devices`
	footer := "--\n臺灣儀表板 | https://dashboard.tw\n客服信箱：support@dashboard.tw"
	out := renderHTML("新裝置登入通知 <測試>", body, footer, "https://dashboard.tw")
	for _, want := range []string{
		`<h1 class="h1"`, "新裝置登入通知 &lt;測試&gt;",
		`>登入地點</td>`, `>臺灣・桃園</td>`,
		`>管理登入裝置</a>`, `href="https://dashboard.tw/me/devices"`,
		`src="https://dashboard.tw/mail/logo-light.png"`,
		`href="mailto:support@dashboard.tw"`,
		"系統偵測到您的帳號剛透過一台新裝置登入",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(out, "<br>--") || strings.Contains(out, ">--<") {
		t.Error("the footer divider was kept")
	}
	if strings.Contains(out, "<測試>") {
		t.Error("the subject was not escaped")
	}
	if p := os.Getenv("MAIL_PREVIEW_OUT"); p != "" {
		_ = os.WriteFile(p, []byte(out), 0o644)
	}
}

func TestButtonLabel(t *testing.T) {
	for raw, want := range map[string]string{
		"https://dashboard.tw/me/api-key": "前往憑證管理",
		"https://dashboard.tw/me/":        "前往會員中心",
		"https://dashboard.tw/admin/apps": "前往審核",
		"https://dashboard.tw/other":      "開啟連結",
		"https://evil.example/me/api-key": "開啟連結",
	} {
		if got := buttonLabel(raw, "https://dashboard.tw"); got != want {
			t.Errorf("buttonLabel(%q) = %q", raw, got)
		}
	}
}
