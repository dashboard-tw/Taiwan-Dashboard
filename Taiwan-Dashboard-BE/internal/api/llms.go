package api

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/screen"
)

func llmsTxt(svc *screen.Service, base string) gin.HandlerFunc {
	return func(c *gin.Context) {
		cat, _, err := svc.Catalog(c.Request.Context())
		if err != nil || cat == nil {
			slog.Error("llms.txt", "error", err)
			writeError(c, http.StatusServiceUnavailable, "unavailable", "暫時無法產生")
			return
		}
		var b strings.Builder
		line := func(s string) { b.WriteString(s + "\n") }
		link := func(name, path, desc string) {
			s := "- [" + name + "](" + base + path + ")"
			if desc != "" {
				s += "：" + desc
			}
			line(s)
		}

		line("# " + siteName)
		line("")
		line("> " + defaultDesc)
		line("")
		line("資料來自各機關公開的政府開放資料，網站整理後以圖表和地圖呈現。")
		line("主題頁可以加上縣市代碼只看單一縣市，例如 " + base + "/weather/63000 是臺北市的天氣。")
		line("")

		line("## 主題")
		line("")
		for _, d := range cat.Dashboards {
			if slices.Contains(sitemapSkipDashboards, d.Slug) {
				continue
			}
			link(d.Name, "/"+d.Slug, d.Description)
		}
		line("")

		line("## 縣市")
		line("")
		for _, county := range cat.Counties {
			link(county.Name+"總覽", "/county/"+county.Code, "縣市代碼 "+county.Code)
		}
		line("")

		line("## 開放資料 API")
		line("")
		link("API 說明", "/developers", "登入後可以免費申請 API 憑證")
		link("OpenAPI 規格", "/api/v1/openapi.json", "")
		link("資料集清單", "/api/v1/catalog", "")
		line("")

		line("## 其他")
		line("")
		link("元件庫", "/components", "所有圖表元件，可以單獨查看或嵌入網站")
		link("歷次颱風", "/typhoons", "每個颱風的路徑、警報與災情紀錄")
		link("關於", "/about", "")
		link("使用條款", "/terms", "")
		link("隱私權政策", "/privacy", "")

		c.Header("Cache-Control", "public, max-age=3600")
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(b.String()))
	}
}
