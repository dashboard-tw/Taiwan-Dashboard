package api

import (
	"strings"
	"testing"

	"dashboard.tw/backend/internal/component"
)

func TestPageMeta(t *testing.T) {
	cat := &component.Catalog{
		Dashboards: []*component.Dashboard{{Slug: "home", Name: "首頁總覽"}, {Slug: "weather", Name: "天氣", Description: "縣市天氣預報"}},
		Counties:   []component.County{{Code: "63000", Name: "臺北市"}},
		Components: map[string]*component.Component{"wind-top": {Name: "目前風最強的測站", ShortDesc: "風速最大的測站"}},
	}
	cases := map[string]pageInfo{
		"/":                    {},
		"/weather":             {"天氣", "全臺天氣：縣市天氣預報。"},
		"/weather/63000/map":   {"臺北市天氣", "臺北市天氣：縣市天氣預報。"},
		"/county/63000":        {"臺北市總覽", "臺北市的天氣、空氣品質、水情、交通與防災即時資訊。"},
		"/components/wind-top": {"目前風最強的測站", "風速最大的測站。"},
		"/about":               staticPages["/about"],
		"/home":                {},
		"/weather/99999":       {},
		"/nope":                {},
	}
	for p, want := range cases {
		if got := lookupPage(cat, p); got != want {
			t.Errorf("%s: got %+v, want %+v", p, got, want)
		}
	}

	page := []byte("<head>\n<title>x</title>\n<meta\n  name=\"description\"\n  content=\"old\"\n/>\n<!--page-meta-->\n</head>")
	out := string(renderIndex(page, pageInfo{"A&B", "說明"}, "https://dashboard.tw", "/weather"))
	for _, s := range []string{
		"<title>A&amp;B｜臺灣儀表板</title>",
		`<meta name="description" content="說明" />`,
		`<link rel="canonical" href="https://dashboard.tw/weather" />`,
		`<meta property="og:image" content="https://dashboard.tw/og.jpg" />`,
	} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q in\n%s", s, out)
		}
	}
	if strings.Contains(out, "old") || strings.Contains(out, metaMarker) {
		t.Errorf("old tags left in\n%s", out)
	}
}
