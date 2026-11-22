package api

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"

	"github.com/gin-gonic/gin"
)

var imageName = regexp.MustCompile(`^\d{8}T\d{4}Z(-f\d{2})?\.(jpg|png)$`)

var imageProducts = map[string]bool{"radar": true, "satellite": true, "radar_layer": true, "cloud_layer": true, "wind_layer": true}

func registerImages(g *gin.RouterGroup, dir string) {
	g.GET("/images/:product/:file", allowParams(), func(c *gin.Context) {
		product, file := c.Param("product"), c.Param("file")
		if !imageProducts[product] || !imageName.MatchString(file) {
			writeError(c, http.StatusNotFound, "not_found", "找不到這張影像")
			return
		}
		path := filepath.Join(dir, product, file)
		if st, err := os.Stat(path); err != nil || !st.Mode().IsRegular() {
			writeError(c, http.StatusNotFound, "not_found", "找不到這張影像")
			return
		}
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Header("X-Content-Type-Options", "nosniff")
		c.File(path)
	})
}

func registerImageProxy(g *gin.RouterGroup, base *url.URL) {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(base)
			r.Out.URL.Path = path.Join("/", base.Path, r.In.Header.Get(imagePathHeader))
			r.Out.URL.RawPath = ""
			r.Out.Header.Del(imagePathHeader)
		},
		ModifyResponse: func(res *http.Response) error {
			if res.StatusCode == http.StatusOK {
				res.Header.Set("Cache-Control", "public, max-age=31536000, immutable")
				res.Header.Set("X-Content-Type-Options", "nosniff")
			} else {
				res.Header.Del("Cache-Control")
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			w.WriteHeader(http.StatusBadGateway)
		},
	}
	g.GET("/images/:product/:file", allowParams(), func(c *gin.Context) {
		product, file := c.Param("product"), c.Param("file")
		if !imageProducts[product] || !imageName.MatchString(file) {
			writeError(c, http.StatusNotFound, "not_found", "找不到這張影像")
			return
		}
		c.Request.Header.Set(imagePathHeader, product+"/"+file)
		proxy.ServeHTTP(c.Writer, c.Request)
	})
}

const imagePathHeader = "X-Dashboardtw-Image-Path"
