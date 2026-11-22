package api

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/screen"
)

func embedSize(c *gin.Context, name string, def, lo, hi int) (int, bool) {
	v := c.Query(name)
	if v == "" {
		return def, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		return 0, false
	}
	return n, true
}

func (h screenHandlers) embed(base string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		slug := c.Param("slug")
		cat, _, err := h.svc.Catalog(c.Request.Context())
		if err != nil {
			h.fail(c, err)
			return
		}
		comp := cat.Components[slug]
		if !slugRe.MatchString(slug) || comp == nil {
			h.fail(c, &screen.Error{Status: http.StatusNotFound, Code: "component_not_found", Message: "找不到這個元件：" + slug})
			return
		}
		q := url.Values{}
		if county := c.Query("county"); county != "" {
			if !cat.IsCounty(county) || !slices.Contains(comp.Scopes(), "county") {
				h.fail(c, &screen.Error{Status: http.StatusBadRequest, Code: "invalid_county", Message: "這個元件沒有這個縣市的資料：" + county})
				return
			}
			q.Set("county", county)
		}
		switch theme := c.Query("theme"); theme {
		case "":
		case "light", "dark":
			q.Set("theme", theme)
		default:
			h.fail(c, &screen.Error{Status: http.StatusBadRequest, Code: "invalid_theme", Message: "theme 只能是 light 或 dark"})
			return
		}
		width, ok1 := embedSize(c, "width", 450, 200, 2000)
		height, ok2 := embedSize(c, "height", 400, 200, 2000)
		if !ok1 || !ok2 {
			h.fail(c, &screen.Error{Status: http.StatusBadRequest, Code: "invalid_size", Message: "width 與 height 要是 200 到 2000 之間的整數"})
			return
		}
		src := base + "/embed/" + slug
		if len(q) > 0 {
			src += "?" + q.Encode()
		}
		code := fmt.Sprintf(`<iframe id="dashboard-tw-%s" title="%s" src="%s" width="%d" height="%d" style="border-radius: 5px" frameborder="0" loading="lazy"></iframe>`,
			html.EscapeString(slug), html.EscapeString(comp.Name), html.EscapeString(src), width, height)
		c.Header("Cache-Control", cachePublic)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(code+"\n"))
	}
}
