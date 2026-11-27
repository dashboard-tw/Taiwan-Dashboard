package api

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/screen"
)

// siteGroups 和前端 frontend/src/lib/topics.ts 的大分類一致
var siteGroups = []struct {
	Name  string
	Slugs []string
}{
	{"天氣", []string{"weather", "forecast", "health", "ocean"}},
	{"防災", []string{"warning", "flood", "earthquake", "fire"}},
	{"環境", []string{"aqi", "reservoir", "sustain"}},
	{"能源", []string{"power", "power-use", "fuel"}},
	{"民生", []string{"outage", "agri", "housing"}},
	{"醫療衛生", []string{"resources", "disease", "food"}},
	{"交通", []string{"traffic", "rail", "aviation", "bike"}},
	{"人口與社會", []string{"population", "education", "economy", "animal", "tourism"}},
}

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

		bySlug := map[string]int{}
		for i, d := range cat.Dashboards {
			bySlug[d.Slug] = i
		}
		listed := map[string]bool{}
		for _, g := range siteGroups {
			line("## " + g.Name)
			line("")
			for _, slug := range g.Slugs {
				if i, ok := bySlug[slug]; ok {
					d := cat.Dashboards[i]
					link(d.Name, "/"+d.Slug, d.Description)
					listed[slug] = true
				}
			}
			line("")
		}
		var rest []string
		for _, d := range cat.Dashboards {
			if !listed[d.Slug] && !slices.Contains(sitemapSkipDashboards, d.Slug) {
				rest = append(rest, d.Slug)
			}
		}
		if len(rest) > 0 {
			line("## 專題")
			line("")
			for _, slug := range rest {
				d := cat.Dashboards[bySlug[slug]]
				link(d.Name, "/"+d.Slug, d.Description)
			}
			line("")
		}

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
		link("MCP 伺服器", "/mcp", "AI 工具連接後可以查詢資料與 API 文件，支援 OAuth 登入授權")
		link("MCP 連接說明", "/developers/mcp", "")
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
