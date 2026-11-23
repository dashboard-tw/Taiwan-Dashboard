package api

import (
	"html"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"dashboard.tw/backend/internal/component"
)

const appMount = `<div id="app"></div>`

var prerendered = map[string]string{"/privacy": "privacy", "/terms": "terms"}

func homeBody(cat *component.Catalog) string {
	var b strings.Builder
	esc := html.EscapeString
	b.WriteString(`<div class="page"><h1>` + siteName + `</h1><p>` + esc(defaultDesc) + `</p>`)
	b.WriteString(`<p>資料來自各機關公開的政府開放資料，網站整理後以圖表和地圖呈現。瀏覽網站不需要登入，申請開放資料 API 時才需要以 GitHub 或 Google 帳號登入。</p>`)
	if cat != nil {
		b.WriteString(`<h2>主題</h2><ul>`)
		for _, d := range cat.Dashboards {
			if slices.Contains(sitemapSkipDashboards, d.Slug) {
				continue
			}
			b.WriteString(`<li><a href="/` + esc(d.Slug) + `">` + esc(d.Name) + `</a>`)
			if d.Description != "" {
				b.WriteString(`：` + esc(d.Description))
			}
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`<h2>更多</h2><ul>`)
	for _, l := range [][2]string{
		{"/about", "關於"}, {"/developers", "開放資料 API"}, {"/showcase/app", "應用活化"},
		{"/privacy", "隱私權政策"}, {"/terms", "使用條款"},
	} {
		b.WriteString(`<li><a href="` + l[0] + `">` + l[1] + `</a></li>`)
	}
	b.WriteString(`</ul></div>`)
	return b.String()
}

func injectBody(page []byte, root, p string, cat *component.Catalog) []byte {
	body := ""
	if name, ok := prerendered[p]; ok {
		if b, err := os.ReadFile(filepath.Join(root, "prerender", name+".html")); err == nil {
			body = string(b)
		}
	} else if p == "/" {
		body = homeBody(cat)
	}
	if body == "" {
		return page
	}
	return []byte(strings.Replace(string(page), appMount, `<div id="app">`+body+`</div>`, 1))
}
