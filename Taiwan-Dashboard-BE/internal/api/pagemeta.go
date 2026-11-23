package api

import (
	"context"
	"html"
	"regexp"
	"strings"

	"dashboard.tw/backend/internal/component"
)

const (
	siteName    = "臺灣儀表板"
	defaultDesc = "整合政府開放資料，即時呈現全臺各縣市的生活、環境與防災資訊。"
	defaultHead = "臺灣儀表板｜dashboard.tw"
	metaMarker  = "<!--page-meta-->"
)

type pageInfo struct {
	Title string
	Desc  string
}

var staticPages = map[string]pageInfo{
	"/about":               {"關於", "臺灣儀表板是雲際科技工作室的公益專案，把政府開放資料整理成即時的圖表與地圖。"},
	"/developers":          {"開放資料 API", "免費申請 API 憑證，取得儀表板整理好的即時與歷史資料。"},
	"/showcase/app":        {"應用活化・App", "使用臺灣儀表板開放資料 API 做成的 App。"},
	"/showcase/web":        {"應用活化・網頁應用", "使用臺灣儀表板開放資料 API 做成的網站與網頁工具。"},
	"/showcase/innovation": {"應用活化・創意應用", "使用臺灣儀表板開放資料做成的研究、報導與資料視覺化。"},
	"/typhoons":            {"歷次颱風", "每個颱風的路徑、警報、停班停課、雨量、停電與收容紀錄。"},
	"/components":          {"元件庫", "儀表板所有的圖表元件，可以單獨查看或嵌入到自己的網站。"},
	"/sitemap":             {"網站導覽", "臺灣儀表板所有頁面的清單：主題儀表板、縣市總覽、開放資料與網站資訊。"},
	"/privacy":             {"隱私權政策", ""},
	"/terms":               {"使用條款", ""},
}

var countyCode = regexp.MustCompile(`^\d{5}$`)

func lookupPage(cat *component.Catalog, p string) pageInfo {
	if p == "/" || p == "" {
		return pageInfo{}
	}
	if info, ok := staticPages[p]; ok {
		return info
	}
	if cat == nil {
		return pageInfo{}
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) > 0 && parts[len(parts)-1] == "map" {
		parts = parts[:len(parts)-1]
	}
	countyName := func(code string) string {
		for _, c := range cat.Counties {
			if c.Code == code {
				return c.Name
			}
		}
		return ""
	}
	switch {
	case len(parts) == 2 && parts[0] == "county" && countyCode.MatchString(parts[1]):
		if name := countyName(parts[1]); name != "" {
			return pageInfo{name + "總覽", name + "的天氣、空氣品質、水情、交通與防災即時資訊。"}
		}
	case len(parts) == 2 && parts[0] == "components":
		if c := cat.Components[parts[1]]; c != nil {
			return pageInfo{c.Name, sentence(c.ShortDesc)}
		}
	case len(parts) == 1 || len(parts) == 2:
		d := cat.Dashboard(parts[0])
		if d == nil || d.Slug == "home" {
			break
		}
		if len(parts) == 1 {
			return pageInfo{d.Name, sentence("全臺" + d.Name + "：" + d.Description)}
		}
		if name := countyName(parts[1]); name != "" {
			return pageInfo{name + d.Name, sentence(name + d.Name + "：" + d.Description)}
		}
	}
	return pageInfo{}
}

func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasSuffix(s, "。") {
		return s
	}
	return s + "。"
}

var (
	titleTag = regexp.MustCompile(`(?s)<title>.*?</title>`)
	descTag  = regexp.MustCompile(`(?s)<meta\s+name="description"\s+content="[^"]*"\s*/?>`)
)

func renderIndex(page []byte, info pageInfo, base, p string) []byte {
	head := defaultHead
	if info.Title != "" {
		head = info.Title + "｜" + siteName
	}
	desc := info.Desc
	if desc == "" {
		desc = defaultDesc
	}
	title, d := html.EscapeString(head), html.EscapeString(desc)
	s := string(page)
	s = titleTag.ReplaceAllLiteralString(s, "<title>"+title+"</title>")
	s = descTag.ReplaceAllLiteralString(s, `<meta name="description" content="`+d+`" />`)
	if base != "" {
		u := html.EscapeString(base + p)
		img := html.EscapeString(base + "/og.jpg")
		var b strings.Builder
		b.WriteString(`<link rel="canonical" href="` + u + `" />` + "\n")
		b.WriteString(`    <meta property="og:type" content="website" />` + "\n")
		b.WriteString(`    <meta property="og:site_name" content="` + siteName + `" />` + "\n")
		b.WriteString(`    <meta property="og:locale" content="zh_TW" />` + "\n")
		b.WriteString(`    <meta property="og:title" content="` + title + `" />` + "\n")
		b.WriteString(`    <meta property="og:description" content="` + d + `" />` + "\n")
		b.WriteString(`    <meta property="og:url" content="` + u + `" />` + "\n")
		b.WriteString(`    <meta property="og:image" content="` + img + `" />` + "\n")
		b.WriteString(`    <meta property="og:image:width" content="1200" />` + "\n")
		b.WriteString(`    <meta property="og:image:height" content="600" />` + "\n")
		b.WriteString(`    <meta property="og:image:alt" content="臺灣儀表板" />` + "\n")
		b.WriteString(`    <meta name="twitter:card" content="summary_large_image" />`)
		s = strings.Replace(s, metaMarker, b.String(), 1)
	}
	return []byte(s)
}

type catalogFunc func(ctx context.Context) *component.Catalog
