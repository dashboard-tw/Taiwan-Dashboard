package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/component"
	"dashboard.tw/backend/internal/opendata"
	"dashboard.tw/backend/internal/screen"
)

type Deps struct {
	DB                       Pinger
	Version                  string
	Screen                   *screen.Service
	ClientIPHeader           string
	RatePerMinute, RateBurst int
	Auth                     AuthDeps
	Admin                    AdminDeps
	OpenData                 *opendata.Service
	DownloadDir              string
	ImageDir                 string
	ImageProxy               *url.URL
	WebDir                   string
	Indexable                bool
}

func NewRouter(deps Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), clientInfo(strings.EqualFold(deps.ClientIPHeader, "CF-Connecting-IP")))
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1"})
	if deps.ClientIPHeader != "" {
		r.TrustedPlatform = deps.ClientIPHeader
	}

	if !deps.Indexable {
		r.Use(noIndex())
	}
	siteURL := ""
	if len(deps.Auth.Origins) > 0 {
		siteURL = deps.Auth.Origins[0]
	}
	getHead := func(path string, h gin.HandlerFunc) {
		r.GET(path, h)
		r.HEAD(path, h)
	}
	getHead("/robots.txt", robots(deps.Indexable, siteURL))
	if siteURL != "" {
		getHead(ardPath, ardManifest(siteURL))
		getHead("/.well-known/ai-catalog.json", ardManifest(siteURL))
	}
	if deps.Screen != nil {
		getHead("/sitemap.xml", sitemap(deps.Screen, siteURL))
		getHead("/llms.txt", llmsTxt(deps.Screen, siteURL))
	}

	v1 := r.Group("/api/v1")
	v1.GET("/health", health(deps.DB, deps.Version))

	if deps.Auth.Store != nil {
		deps.Auth.ImageDir = deps.ImageDir
		registerAuth(v1, deps.Auth)
		registerMe(v1, deps.Auth)
		registerAdmin(v1, deps.Auth, deps.Admin)
		v1.GET("/apps", publicApps(deps.Auth.Store.DB))
		if deps.Auth.MailWebhookToken != "" && deps.Auth.Notify.Enabled() {
			v1.POST("/hooks/brevo", newLimiter(600, 300).middleware(), brevoWebhook(deps.Auth.Notify, deps.Auth.MailWebhookToken))
		}
		if deps.ImageDir != "" {
			registerAppImages(v1, deps.ImageDir)
		}
		limits := &keyLimits{byTi: map[int]*limiter{}, notify: deps.Auth.Notify}
		if deps.OpenData != nil {
			free, partner := auth.Tiers[auth.TierFree], auth.Tiers[auth.TierPartner]
			spec := opendata.SpecOptions{
				Version: deps.Version, PerDay: free.PerDay, PerMinute: free.PerMinute,
				MBPerDay: int(free.BytesPerDay >> 20), TokenHours: int(auth.TokenTTL.Hours()),
				PartnerPerDay: partner.PerDay, PartnerPerMinute: partner.PerMinute,
				PartnerMBPerDay: int(partner.BytesPerDay >> 20),
				Downloads:       deps.DownloadDir != "",
			}
			registerOpenData(v1, deps.Auth.Store, deps.OpenData, limits, spec)
			if deps.Screen != nil {
				site := siteOf(deps.Auth.Origins)
				registerMCPOAuth(r, v1, deps.Auth, site)
				registerMCP(r, deps.Auth.Store, deps.OpenData, deps.Screen, limits, spec, site, deps.Version)
			}
		}
		if deps.DownloadDir != "" {
			registerDownloads(r, deps.Auth.Store, deps.DownloadDir, limits)
		}
	}

	if deps.Screen != nil {
		perMinute, burst := deps.RatePerMinute, deps.RateBurst
		if perMinute <= 0 {
			perMinute = 120
		}
		if burst <= 0 {
			burst = 60
		}
		screenGroup := v1.Group("", newLimiter(perMinute, burst).middleware())
		registerScreen(screenGroup, deps.Screen, siteURL)
		screenGroup.POST("/presence", newPresence(deps.Screen.App).handle)
		if deps.ImageDir != "" {
			registerImages(screenGroup, deps.ImageDir)
		} else if deps.ImageProxy != nil {
			registerImageProxy(screenGroup, deps.ImageProxy)
		}
	}

	if deps.WebDir != "" {
		var catalog catalogFunc
		if deps.Screen != nil {
			catalog = func(ctx context.Context) *component.Catalog {
				cat, _, err := deps.Screen.Catalog(ctx)
				if err != nil {
					return nil
				}
				return cat
			}
		}
		r.NoRoute(webHandler(deps.WebDir, siteURL, catalog))
	} else {
		r.NoRoute(func(c *gin.Context) {
			writeError(c, http.StatusNotFound, "not_found", "找不到這個路徑")
		})
	}
	return r
}
