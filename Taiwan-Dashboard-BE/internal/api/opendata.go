package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/notify"
	"dashboard.tw/backend/internal/opendata"
)

const ownerKey = "dtw.key"

var (
	siteRe   = regexp.MustCompile(`^[A-Za-z0-9-]{1,40}$`)
	countyRe = regexp.MustCompile(`^\d{5}$`)
)

type keyLimits struct {
	mu     sync.Mutex
	byTi   map[int]*limiter
	notify *notify.Notifier
}

func (k *keyLimits) allow(owner *auth.KeyOwner) (bool, time.Duration) {
	k.mu.Lock()
	l := k.byTi[owner.Tier.PerMinute]
	if l == nil {
		l = newLimiter(owner.Tier.PerMinute, owner.Tier.PerMinute)
		k.byTi[owner.Tier.PerMinute] = l
	}
	k.mu.Unlock()
	return l.allow(strconv.FormatInt(owner.UserID, 10))
}

func openCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Authorization, If-None-Match")
		c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Header("Access-Control-Expose-Headers", "X-RateLimit-Limit, X-RateLimit-Remaining, X-Traffic-Limit, X-Traffic-Used, X-Next-Cursor, Retry-After, ETag")
		if c.Request.Method == http.MethodOptions {
			c.Header("Access-Control-Max-Age", "86400")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func requireKey(store *auth.Store, limits *keyLimits) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || token == "" {
			c.Header("WWW-Authenticate", `Bearer realm="dashboard.tw"`)
			writeError(c, http.StatusUnauthorized, "token_required", "缺少 Access Token，請先用 Client Id 與 Client Secret 到 /api/v1/auth/token 換取")
			return
		}
		ctx := c.Request.Context()
		owner, err := store.LookupToken(ctx, strings.TrimSpace(token))
		if errors.Is(err, auth.ErrNoKey) {
			c.Header("WWW-Authenticate", `Bearer realm="dashboard.tw", error="invalid_token"`)
			writeError(c, http.StatusUnauthorized, "invalid_token", "Access Token 無效或已過期，請重新取得")
			return
		}
		if err != nil {
			openFail(c, err)
			return
		}
		if ok, wait := limits.allow(owner); !ok {
			countRejected(c, store, owner)
			c.Header("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			writeError(c, http.StatusTooManyRequests, "rate_limited",
				fmt.Sprintf("每分鐘最多 %d 次，請稍後再試", owner.Tier.PerMinute))
			return
		}
		used, ok, err := store.UseAPIKey(ctx, owner)
		if err != nil {
			openFail(c, err)
			return
		}
		c.Header("X-RateLimit-Limit", strconv.Itoa(owner.Tier.PerDay))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(max(0, owner.Tier.PerDay-used.Requests)))
		c.Header("X-Traffic-Limit", strconv.FormatInt(owner.Tier.BytesPerDay, 10))
		c.Header("X-Traffic-Used", strconv.FormatInt(used.Bytes, 10))
		if !ok {
			countRejected(c, store, owner)
			c.Header("Retry-After", strconv.Itoa(secondsToTaipeiMidnight(time.Now())))
			if used.Bytes >= owner.Tier.BytesPerDay {
				writeError(c, http.StatusTooManyRequests, "traffic_exceeded",
					fmt.Sprintf("今天的 %d MB 流量已經用完，臺灣時間午夜重新計算", owner.Tier.BytesPerDay>>20))
				return
			}
			writeError(c, http.StatusTooManyRequests, "quota_exceeded",
				fmt.Sprintf("今天的 %d 次額度已經用完，臺灣時間午夜重新計算", owner.Tier.PerDay))
			return
		}
		c.Set(ownerKey, owner)
		c.Next()
		n := max(c.Writer.Size(), 0)
		if n > 0 {
			if err := store.AddTraffic(context.WithoutCancel(ctx), owner, int64(n)); err != nil {
				slog.Error("count api traffic", "error", err)
			}
		}
		limits.notify.UsageHigh(owner.UserID, auth.Usage{Requests: used.Requests, Bytes: used.Bytes + int64(n)}, owner.Tier)
	}
}

func revalidate() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-cache")
		c.Next()
	}
}

type bufferedWriter struct {
	gin.ResponseWriter
	buf    bytes.Buffer
	status int
}

func (w *bufferedWriter) WriteHeader(code int) { w.status = code }
func (w *bufferedWriter) WriteHeaderNow()      {}
func (w *bufferedWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.buf.Write(b)
}
func (w *bufferedWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *bufferedWriter) Written() bool                     { return w.status != 0 }
func (w *bufferedWriter) Size() int                         { return w.buf.Len() }
func (w *bufferedWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func etagMatches(header, etag string) bool {
	weak := strings.TrimPrefix(etag, "W/")
	for _, t := range strings.Split(header, ",") {
		t = strings.TrimSpace(t)
		if t == "*" || t == etag || strings.TrimPrefix(t, "W/") == weak {
			return true
		}
	}
	return false
}

func conditionalGet() gin.HandlerFunc {
	return func(c *gin.Context) {
		bw := &bufferedWriter{ResponseWriter: c.Writer}
		c.Writer = bw
		c.Next()
		c.Writer = bw.ResponseWriter
		status := bw.Status()
		if status != http.StatusOK {
			c.Writer.WriteHeader(status)
			_, _ = c.Writer.Write(bw.buf.Bytes())
			return
		}
		sum := sha256.Sum256(bw.buf.Bytes())
		etag := `W/"` + hex.EncodeToString(sum[:16]) + `"`
		c.Header("ETag", etag)
		if inm := c.GetHeader("If-None-Match"); inm != "" && etagMatches(inm, etag) {
			c.Writer.Header().Del("Content-Type")
			c.Writer.WriteHeader(http.StatusNotModified)
			c.Writer.WriteHeaderNow()
			return
		}
		c.Writer.WriteHeader(http.StatusOK)
		_, _ = c.Writer.Write(bw.buf.Bytes())
	}
}

func countRejected(c *gin.Context, store *auth.Store, owner *auth.KeyOwner) {
	if err := store.RejectAPIKey(context.WithoutCancel(c.Request.Context()), owner); err != nil {
		slog.Error("count rejected api request", "error", err)
	}
}

func countDataset(svc *opendata.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Writer.Status() >= 400 {
			return
		}
		id := c.Param("id")
		if id == "" {
			id = "_list"
		} else if opendata.Find(id) == nil {
			return
		}
		if err := svc.CountUse(context.WithoutCancel(c.Request.Context()), id); err != nil {
			slog.Error("count dataset use", "dataset", id, "error", err)
		}
	}
}

func secondsToTaipeiMidnight(now time.Time) int {
	t := now.In(taipeiZone)
	next := time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, taipeiZone)
	return int(next.Sub(t).Seconds()) + 1
}

var taipeiZone = time.FixedZone("Asia/Taipei", 8*60*60)

func openFail(c *gin.Context, err error) {
	slog.Error("open data api", "path", c.FullPath(), "error", err)
	writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
}

func registerOpenData(v1 *gin.RouterGroup, store *auth.Store, svc *opendata.Service, limits *keyLimits, spec opendata.SpecOptions) {
	h := openHandlers{svc: svc, spec: spec}
	v1.GET("/openapi.json", openCORS(), newLimiter(60, 30).middleware(), allowParams(), h.openAPI)
	v1.GET("/catalog", openCORS(), newLimiter(60, 30).middleware(), allowParams(), h.catalog)
	v1.POST("/auth/token", noStoreMiddleware(), newLimiter(20, 10).middleware(), tokenEndpoint(store))
	g := v1.Group("/datasets", openCORS(), revalidate(), newLimiter(60, 30).middleware(),
		requireKey(store, limits), countDataset(svc), conditionalGet())
	g.GET("", allowParams(), h.list)
	g.GET("/:id", allowParams(), h.describe)
	g.GET("/:id/sites", allowParams("county", "format"), h.sites)
	g.GET("/:id/latest", allowParams("county", "site", "cursor", "format"), h.latest)
	g.GET("/:id/history", allowParams("county", "site", "from", "to", "cursor", "format"), h.history)
	for _, p := range []string{"", "/:id", "/:id/:part"} {
		v1.OPTIONS("/datasets"+p, openCORS())
	}
}

func noStoreMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Next()
	}
}

type openHandlers struct {
	svc  *opendata.Service
	spec opendata.SpecOptions
}

func (h openHandlers) openAPI(c *gin.Context) {
	o := h.spec
	o.Names = map[string]string{}
	if list, err := h.svc.List(c.Request.Context()); err == nil {
		for _, s := range list {
			o.Names[s.ID] = s.Name
		}
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, opendata.OpenAPI(o))
}

func (h openHandlers) catalog(c *gin.Context) {
	cat, err := h.svc.Catalog(c.Request.Context())
	if err != nil {
		openFail(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	writeData(c, http.StatusOK, cat, nil)
}

func (h openHandlers) list(c *gin.Context) {
	list, err := h.svc.List(c.Request.Context())
	if err != nil {
		openFail(c, err)
		return
	}
	writeData(c, http.StatusOK, list, nil)
}

func (h openHandlers) dataset(c *gin.Context) (*opendata.Dataset, *opendata.Summary, bool) {
	d := opendata.Find(c.Param("id"))
	if d == nil {
		writeError(c, http.StatusNotFound, "dataset_not_found", "找不到這個資料集："+c.Param("id"))
		return nil, nil, false
	}
	s, err := h.svc.Get(c.Request.Context(), d.ID)
	if err != nil {
		openFail(c, err)
		return nil, nil, false
	}
	if s == nil {
		writeError(c, http.StatusNotFound, "dataset_not_found", "這個資料集還沒有資料："+d.ID)
		return nil, nil, false
	}
	return d, s, true
}

func (h openHandlers) describe(c *gin.Context) {
	d, s, ok := h.dataset(c)
	if !ok {
		return
	}
	fields := d.SiteFields
	if fields == nil {
		fields = []opendata.Field{}
	}
	out := gin.H{
		"dataset": s, "fields": d.Fields, "latest_fields": d.LatestRowFields(), "site_fields": fields,
		"latest_paged": d.LatestPaged, "has_history": d.HasHistory(),
		"history": gin.H{"page_size": opendata.PageSize, "max_range_days": int(opendata.MaxRange.Hours() / 24)},
	}
	if d.Months != "" {
		out["downloads"] = "/downloads/" + d.ID + "/" + opendata.IndexFile
	}
	writeData(c, http.StatusOK, out, nil)
}

func (h openHandlers) filters(c *gin.Context, d *opendata.Dataset) (county, site *string, ok bool) {
	if v, has := c.GetQuery("county"); has {
		if !d.HasCounty {
			writeError(c, http.StatusBadRequest, "unsupported_parameter", "這個資料集不能用 county 篩選")
			return nil, nil, false
		}
		if !countyRe.MatchString(v) {
			writeError(c, http.StatusBadRequest, "invalid_county", "縣市代碼格式錯誤，例如 63000")
			return nil, nil, false
		}
		county = &v
	}
	if v, has := c.GetQuery("site"); has {
		if !d.HasSite {
			writeError(c, http.StatusBadRequest, "unsupported_parameter", "這個資料集沒有觀測點")
			return nil, nil, false
		}
		if !siteRe.MatchString(v) {
			writeError(c, http.StatusBadRequest, "invalid_site", "觀測對象代碼格式錯誤")
			return nil, nil, false
		}
		site = &v
	}
	return county, site, true
}

func format(c *gin.Context) (string, bool) {
	f := c.DefaultQuery("format", "json")
	if f != "json" && f != "csv" {
		writeError(c, http.StatusBadRequest, "invalid_format", "format 只能是 json 或 csv")
		return "", false
	}
	return f, true
}

func writeTable(c *gin.Context, f, name string, t opendata.Table, meta gin.H) {
	if f == "csv" {
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, name))
		c.Data(http.StatusOK, "text/csv; charset=utf-8", toCSV(t.CSVRows()))
		return
	}
	writeData(c, http.StatusOK, t, meta)
}

func sourceMeta(s *opendata.Summary) gin.H {
	return gin.H{"dataset": s.ID, "source": gin.H{
		"agency": s.Agency, "license": s.License, "url": s.SourceURL,
	}, "status": s.Status, "last_observed_at": s.LastObservedAt}
}

func (h openHandlers) sites(c *gin.Context) {
	d, s, ok := h.dataset(c)
	if !ok {
		return
	}
	if !d.HasSite {
		writeError(c, http.StatusNotFound, "no_sites", "這個資料集沒有觀測點")
		return
	}
	county, _, ok := h.filters(c, d)
	if !ok {
		return
	}
	f, ok := format(c)
	if !ok {
		return
	}
	t, err := h.svc.Sites(c.Request.Context(), d, county)
	if err != nil {
		openFail(c, err)
		return
	}
	writeTable(c, f, d.ID+"-sites", t, sourceMeta(s))
}

func (h openHandlers) latest(c *gin.Context) {
	d, s, ok := h.dataset(c)
	if !ok {
		return
	}
	county, site, ok := h.filters(c, d)
	if !ok {
		return
	}
	f, ok := format(c)
	if !ok {
		return
	}
	t, next, err := h.svc.Latest(c.Request.Context(), d, county, site, c.Query("cursor"))
	if errors.Is(err, opendata.ErrBadCursor) {
		writeError(c, http.StatusBadRequest, "invalid_cursor", "cursor 無效，請使用回應中的 next_cursor")
		return
	}
	if err != nil {
		openFail(c, err)
		return
	}
	meta := sourceMeta(s)
	if next != "" {
		meta["next_cursor"] = next
		c.Header("X-Next-Cursor", next)
	}
	writeTable(c, f, d.ID+"-latest", t, meta)
}

func (h openHandlers) history(c *gin.Context) {
	d, s, ok := h.dataset(c)
	if !ok {
		return
	}
	if !d.HasHistory() {
		writeError(c, http.StatusNotFound, "no_history", "這個資料集沒有歷史資料，請改用 latest")
		return
	}
	county, site, ok := h.filters(c, d)
	if !ok {
		return
	}
	f, ok := format(c)
	if !ok {
		return
	}
	to := time.Now()
	if v := c.Query("to"); v != "" {
		t, err := parseTime(v)
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_time", "to 的時間格式錯誤")
			return
		}
		to = *t
	}
	from := to.Add(-opendata.DefaultRange)
	if v := c.Query("from"); v != "" {
		t, err := parseTime(v)
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_time", "from 的時間格式錯誤")
			return
		}
		from = *t
	}
	if !from.Before(to) {
		writeError(c, http.StatusBadRequest, "invalid_range", "from 必須早於 to")
		return
	}
	if to.Sub(from) > opendata.MaxRange {
		writeError(c, http.StatusBadRequest, "range_too_long", "每次最多查詢 31 天，更長的範圍請改用大量下載")
		return
	}
	t, next, err := h.svc.History(c.Request.Context(), d, opendata.HistoryQuery{
		County: county, Site: site, From: from, To: to, Cursor: c.Query("cursor"),
	})
	if errors.Is(err, opendata.ErrBadCursor) {
		writeError(c, http.StatusBadRequest, "invalid_cursor", "cursor 無效，請使用回應中的 next_cursor")
		return
	}
	if err != nil {
		openFail(c, err)
		return
	}
	meta := sourceMeta(s)
	meta["from"], meta["to"] = from.In(taipeiZone), to.In(taipeiZone)
	if next != "" {
		meta["next_cursor"] = next
		c.Header("X-Next-Cursor", next)
	}
	writeTable(c, f, d.ID+"-history", t, meta)
}

func registerDownloads(r *gin.Engine, store *auth.Store, dir string, limits *keyLimits) {
	g := r.Group("/downloads", openCORS(), noStoreMiddleware(), newLimiter(60, 30).middleware(), requireKey(store, limits))
	g.GET("/:dataset/:file", func(c *gin.Context) { serveDownload(c, dir) })
	r.OPTIONS("/downloads/:dataset/:file", openCORS())
}

func serveDownload(c *gin.Context, dir string) {
	id, file := c.Param("dataset"), c.Param("file")
	m := opendata.MonthFile.FindStringSubmatch(file)
	if opendata.Find(id) == nil || (file != opendata.IndexFile && m == nil) {
		writeError(c, http.StatusNotFound, "not_found", "找不到這個檔案")
		return
	}
	f, err := os.Open(filepath.Join(dir, id, file))
	if errors.Is(err, fs.ErrNotExist) {
		msg := "找不到這個檔案"
		if file == opendata.IndexFile {
			msg = "這個資料集還沒有大量下載檔案"
		}
		writeError(c, http.StatusNotFound, "not_found", msg)
		return
	}
	if err != nil {
		openFail(c, err)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		openFail(c, err)
		return
	}
	if file == opendata.IndexFile {
		c.Header("Content-Type", "application/json; charset=utf-8")
	} else {
		c.Header("Content-Type", "application/gzip")
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s"`, id, file))
	}
	http.ServeContent(c.Writer, c.Request, file, st.ModTime(), f)
}

func tokenEndpoint(store *auth.Store) gin.HandlerFunc {
	oauthError := func(c *gin.Context, status int, code, desc string) {
		c.Header("Pragma", "no-cache")
		c.AbortWithStatusJSON(status, gin.H{"error": code, "error_description": desc})
	}
	return func(c *gin.Context) {
		c.Header("Pragma", "no-cache")
		if err := c.Request.ParseForm(); err != nil || !strings.HasPrefix(c.ContentType(), "application/x-www-form-urlencoded") {
			oauthError(c, http.StatusBadRequest, "invalid_request", "請以 application/x-www-form-urlencoded 送出")
			return
		}
		form := c.Request.PostForm
		if form.Get("grant_type") != "client_credentials" {
			oauthError(c, http.StatusBadRequest, "unsupported_grant_type", "grant_type 必須是 client_credentials")
			return
		}
		id, secret, basic := c.Request.BasicAuth()
		if basic {
			id, _ = url.QueryUnescape(id)
			secret, _ = url.QueryUnescape(secret)
		} else {
			id, secret = form.Get("client_id"), form.Get("client_secret")
		}
		token, expires, err := store.IssueToken(c.Request.Context(), id, secret)
		if errors.Is(err, auth.ErrInvalidClient) {
			if basic {
				c.Header("WWW-Authenticate", `Basic realm="dashboard.tw"`)
			}
			oauthError(c, http.StatusUnauthorized, "invalid_client", "Client Id 或 Client Secret 錯誤，或憑證已停用")
			return
		}
		if err != nil {
			slog.Error("issue token", "error", err)
			oauthError(c, http.StatusInternalServerError, "server_error", "伺服器發生錯誤")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"access_token": token,
			"token_type":   "Bearer",
			"expires_in":   int(time.Until(expires).Round(time.Second).Seconds()),
		})
	}
}
