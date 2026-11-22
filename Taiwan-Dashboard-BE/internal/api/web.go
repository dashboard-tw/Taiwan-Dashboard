package api

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/component"
)

func noIndex() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Robots-Tag", "noindex, nofollow")
		c.Next()
	}
}

func robots(indexable bool, base string) gin.HandlerFunc {
	body := "User-agent: *\nDisallow: /\n"
	if indexable {
		body = "User-agent: *\nDisallow: /api/\nDisallow: /admin\nDisallow: /me\nDisallow: /embed/\n"
		if base != "" {
			body += "\nSitemap: " + base + "/sitemap.xml\nAgentmap: " + base + ardPath + "\n"
		}
	}
	return func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=3600")
		c.String(http.StatusOK, body)
	}
}

func webHandler(dir, base string, catalog catalogFunc) gin.HandlerFunc {
	root, _ := filepath.Abs(dir)
	index := filepath.Join(root, "index.html")
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") || p == "/api" || strings.HasPrefix(p, "/downloads/") ||
			(c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) {
			writeError(c, http.StatusNotFound, "not_found", "找不到這個路徑")
			return
		}
		webHeaders(c, p)
		name := filepath.Join(root, filepath.FromSlash(path.Clean("/"+p)))
		if name != root && strings.HasPrefix(name, root+string(filepath.Separator)) {
			if st, err := os.Stat(name); err == nil && !st.IsDir() {
				if strings.HasPrefix(p, "/assets/") {
					c.Header("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					c.Header("Cache-Control", "public, max-age=3600")
				}
				http.ServeFile(c.Writer, c.Request, name)
				return
			}
		}
		if strings.HasPrefix(p, "/assets/") {
			c.Header("Cache-Control", "no-store")
			c.String(http.StatusNotFound, "not found")
			return
		}
		page, err := os.ReadFile(index)
		if err != nil {
			writeError(c, http.StatusNotFound, "not_found", "找不到這個路徑")
			return
		}
		var cat *component.Catalog
		if catalog != nil {
			cat = catalog(c.Request.Context())
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", renderIndex(page, lookupPage(cat, p), base, p))
	}
}

func webHeaders(c *gin.Context, p string) {
	h := c.Writer.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	if !strings.HasPrefix(p, "/embed/") {
		h.Set("X-Frame-Options", "SAMEORIGIN")
	}
}
