package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/db"
	"dashboard.tw/backend/internal/hooks"
)

var appCategories = []string{"app", "web", "innovation"}

type appFields struct {
	Category    string   `json:"category"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	WebURL      string   `json:"web_url"`
	IOSURL      string   `json:"ios_url"`
	AndroidURL  string   `json:"android_url"`
	Topics      []string `json:"topics"`
	ShowAuthor  bool     `json:"show_author"`
}

type appContent struct {
	appFields
	Image      string `json:"image"`
	Screenshot string `json:"screenshot"`
}

type myApp struct {
	ID int64 `json:"id"`
	appContent
	Status      string      `json:"status"`
	ReviewNote  string      `json:"review_note"`
	Published   *appContent `json:"published"`
	PublishedAt *time.Time  `json:"published_at"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

const appColumns = `id, category, name, description, web_url, ios_url, android_url, image, screenshot, topics, show_author,
	status, review_note, published, published_at, created_at, updated_at`

func scanApp(row interface{ Scan(...any) error }, extra ...any) (*myApp, error) {
	var a myApp
	var published []byte
	m := pgtype.NewMap()
	dest := append([]any{&a.ID, &a.Category, &a.Name, &a.Description, &a.WebURL, &a.IOSURL, &a.AndroidURL, &a.Image, &a.Screenshot,
		m.SQLScanner(&a.Topics), &a.ShowAuthor, &a.Status, &a.ReviewNote, &published, &a.PublishedAt, &a.CreatedAt, &a.UpdatedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	if published != nil {
		a.Published = &appContent{}
		if err := json.Unmarshal(published, a.Published); err != nil {
			return nil, err
		}
	}
	return &a, nil
}

type appInput struct {
	appFields
	AgreeRules bool `json:"agree_rules"`
}

func visibleText(s string, max int, multiline bool) (string, bool) {
	s = strings.TrimSpace(s)
	n := 0
	for _, r := range s {
		if multiline && r == '\n' {
			n++
			continue
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == utf8.RuneError {
			return "", false
		}
		n++
	}
	return s, n >= 1 && n <= max
}

func (in *appInput) validate(c *gin.Context) bool {
	fail := func(code, msg string) bool {
		writeError(c, http.StatusBadRequest, code, msg)
		return false
	}
	var ok bool
	if !in.AgreeRules {
		return fail("rules_required", "請先同意規定")
	}
	if !slices.Contains(appCategories, in.Category) {
		return fail("invalid_category", "請選擇類別")
	}
	if in.Name, ok = visibleText(in.Name, 40, false); !ok {
		return fail("invalid_name", "應用名稱要 1 到 40 個字")
	}
	if in.Description, ok = visibleText(in.Description, 200, true); !ok {
		return fail("invalid_description", "介紹要 1 到 200 個字")
	}
	links := []struct {
		v    *string
		host string
		code string
		msg  string
	}{
		{&in.WebURL, "", "invalid_web_url", "網站網址格式錯誤，請以 https:// 開頭"},
		{&in.IOSURL, "apps.apple.com", "invalid_ios_url", "App Store 連結格式錯誤"},
		{&in.AndroidURL, "play.google.com", "invalid_android_url", "Google Play 連結格式錯誤"},
	}
	hasLink := false
	for _, l := range links {
		*l.v = strings.TrimSpace(*l.v)
		if *l.v == "" {
			continue
		}
		hasLink = true
		if !validLink(*l.v, l.host) {
			return fail(l.code, l.msg)
		}
	}
	if !hasLink {
		return fail("link_required", "請至少填一個連結")
	}
	if in.Category == "app" && in.IOSURL == "" && in.AndroidURL == "" {
		return fail("store_link_required", "請填 App Store 或 Google Play 連結")
	}
	if in.Category == "web" && in.WebURL == "" {
		return fail("web_link_required", "請填網站連結")
	}
	if len(in.Topics) == 0 || len(in.Topics) > 10 {
		return fail("invalid_topics", "請選擇 1 到 10 個用到的資料")
	}
	seen := map[string]bool{}
	for _, t := range in.Topics {
		if !slices.Contains(surveyTopics, t) || seen[t] {
			return fail("invalid_topics", "用到的資料有無法辨識的項目，請重新選擇")
		}
		seen[t] = true
	}
	return true
}

func validLink(raw, host string) bool {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 200 || u.Host == "" || u.User != nil {
		return false
	}
	if host != "" {
		return u.Scheme == "https" && strings.EqualFold(u.Hostname(), host)
	}
	return u.Scheme == "https" || u.Scheme == "http"
}

func appParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusNotFound, "not_found", "找不到這個應用")
		return 0, false
	}
	return id, true
}

func (h meHandlers) loadApps(c *gin.Context) ([]*myApp, error) {
	list := []*myApp{}
	err := h.asUser(c, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(c.Request.Context(), `SELECT `+appColumns+` FROM app.showcase_app ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanApp(rows)
			if err != nil {
				return err
			}
			list = append(list, a)
		}
		return rows.Err()
	})
	return list, err
}

func (h meHandlers) myApps(c *gin.Context) {
	list, err := h.loadApps(c)
	if err != nil {
		meFail(c, err)
		return
	}
	writeData(c, http.StatusOK, list, nil)
}

var (
	errNoClient = errors.New("no api credential")
)

func (h meHandlers) createApp(c *gin.Context) {
	var in appInput
	if !bindJSON(c, &in) || !in.validate(c) {
		return
	}
	u := h.user(c)
	ok, err := h.submit.run(c, func(ctx context.Context) error {
		return h.asUser(c, func(tx *sql.Tx) error {
			var hasClient bool
			if err := tx.QueryRowContext(ctx,
				`SELECT EXISTS (SELECT 1 FROM app.api_key WHERE revoked_at IS NULL)`).Scan(&hasClient); err != nil {
				return err
			}
			if !hasClient {
				return errNoClient
			}
			var id int64
			if err := tx.QueryRowContext(ctx, `
			INSERT INTO app.showcase_app (user_id, name, description, web_url, ios_url, android_url, topics, show_author, category)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
				u.ID, in.Name, in.Description, in.WebURL, in.IOSURL, in.AndroidURL, in.Topics, in.ShowAuthor, in.Category).Scan(&id); err != nil {
				return err
			}
			return auth.AuditTx(ctx, tx, &u.ID, "app.submit", fmt.Sprintf("app:%d", id), nil, c.ClientIP())
		})
	})
	if !ok {
		return
	}
	if errors.Is(err, errNoClient) {
		writeError(c, http.StatusConflict, "client_required", "請先建立 API 憑證")
		return
	}
	if err != nil {
		meFail(c, err)
		return
	}
	h.auth.Notify.AppSubmitted(in.Name, u.DisplayName, false)
	h.auth.Hooks.Emit(c.Request.Context(), hooks.EventAppSubmitted, hooks.Message{
		Title: "有新投稿：" + in.Name, Level: hooks.LevelInfo, URL: siteOf(h.auth.Origins) + "/admin/apps",
		Text: in.Description, Fields: []hooks.Field{{Name: "投稿者", Value: u.DisplayName}},
	})
	h.myApps(c)
}

func (h meHandlers) updateApp(c *gin.Context) {
	id, ok := appParam(c)
	if !ok {
		return
	}
	var in appInput
	if !bindJSON(c, &in) || !in.validate(c) {
		return
	}
	u := h.user(c)
	ok, err := h.submit.run(c, func(ctx context.Context) error {
		return h.asUser(c, func(tx *sql.Tx) error {
			res, err := tx.ExecContext(ctx, `
			UPDATE app.showcase_app
			   SET name = $2, description = $3, web_url = $4, ios_url = $5, android_url = $6,
			       topics = $7, show_author = $8, category = $9, status = 'pending', review_note = '', updated_at = now(),
			       screenshot = CASE WHEN $9 = 'web' THEN screenshot ELSE '' END
			 WHERE id = $1`, id, in.Name, in.Description, in.WebURL, in.IOSURL, in.AndroidURL, in.Topics, in.ShowAuthor, in.Category)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return sql.ErrNoRows
			}
			return auth.AuditTx(ctx, tx, &u.ID, "app.update", fmt.Sprintf("app:%d", id), nil, c.ClientIP())
		})
	})
	if !ok {
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "not_found", "找不到這個應用")
		return
	}
	if err != nil {
		meFail(c, err)
		return
	}
	h.auth.Notify.AppSubmitted(in.Name, u.DisplayName, true)
	h.auth.Hooks.Emit(c.Request.Context(), hooks.EventAppSubmitted, hooks.Message{
		Title: "應用修改待審查：" + in.Name, Level: hooks.LevelInfo, URL: siteOf(h.auth.Origins) + "/admin/apps",
		Fields: []hooks.Field{{Name: "投稿者", Value: u.DisplayName}},
	})
	h.myApps(c)
}

func (h meHandlers) deleteApp(c *gin.Context) {
	id, ok := appParam(c)
	if !ok {
		return
	}
	u := h.user(c)
	ctx := c.Request.Context()
	var images []string
	err := h.asUser(c, func(tx *sql.Tx) error {
		var draft, shot string
		var published, publishedShot sql.NullString
		if err := tx.QueryRowContext(ctx, `
			DELETE FROM app.showcase_app WHERE id = $1 RETURNING image, screenshot, published->>'image', published->>'screenshot'`, id).
			Scan(&draft, &shot, &published, &publishedShot); err != nil {
			return err
		}
		images = []string{draft, shot, published.String, publishedShot.String}
		return auth.AuditTx(ctx, tx, &u.ID, "app.delete", fmt.Sprintf("app:%d", id), nil, c.ClientIP())
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "not_found", "找不到這個應用")
		return
	}
	if err != nil {
		meFail(c, err)
		return
	}
	dropUnusedImages(c, h.db, h.auth.ImageDir, images...)
	c.Status(http.StatusNoContent)
}

var errNotWeb = errors.New("screenshot is only for web apps")

func (h meHandlers) uploadAppImage(c *gin.Context)      { h.uploadImage(c, logoImage) }
func (h meHandlers) uploadAppScreenshot(c *gin.Context) { h.uploadImage(c, shotImage) }

func (h meHandlers) uploadImage(c *gin.Context, k imageKind) {
	id, ok := appParam(c)
	if !ok {
		return
	}
	if h.auth.ImageDir == "" {
		writeError(c, http.StatusServiceUnavailable, "images_disabled", "目前無法上傳圖片")
		return
	}
	data, ok := readAppImage(c.Request.Body)
	if !ok {
		writeError(c, http.StatusRequestEntityTooLarge, "image_too_large", "圖片最大 2 MB")
		return
	}
	u := h.user(c)
	var old, name string
	ok, err := h.submit.run(c, func(ctx context.Context) error {
		out, n, err := encodeAppImage(data, k)
		if err != nil {
			return err
		}
		name = n
		if err := saveAppImage(h.auth.ImageDir, name, out); err != nil {
			return err
		}
		return h.asUser(c, func(tx *sql.Tx) error {
			var category string
			if err := tx.QueryRowContext(ctx, `SELECT `+k.column+`, category FROM app.showcase_app WHERE id = $1 FOR UPDATE`, id).
				Scan(&old, &category); err != nil {
				return err
			}
			if k == shotImage && category != "web" {
				return errNotWeb
			}
			if _, err := tx.ExecContext(ctx, `
			UPDATE app.showcase_app SET `+k.column+` = $2, status = 'pending', review_note = '', updated_at = now()
			 WHERE id = $1`, id, name); err != nil {
				return err
			}
			return auth.AuditTx(ctx, tx, &u.ID, "app."+k.column, fmt.Sprintf("app:%d", id), nil, c.ClientIP())
		})
	})
	if !ok {
		return
	}
	if errors.Is(err, errBadImage) {
		writeError(c, http.StatusBadRequest, "invalid_image", "只能上傳 PNG、JPEG 或 WebP 圖片")
		return
	}
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, errNotWeb) {
		dropUnusedImages(c, h.db, h.auth.ImageDir, name)
		if errors.Is(err, errNotWeb) {
			writeError(c, http.StatusConflict, "not_web", "只有網站可以上傳截圖")
			return
		}
		writeError(c, http.StatusNotFound, "not_found", "找不到這個應用")
		return
	}
	if err != nil {
		meFail(c, err)
		return
	}
	if old != name {
		dropUnusedImages(c, h.db, h.auth.ImageDir, old)
	}
	h.myApps(c)
}

func dropUnusedImages(c *gin.Context, dbh *sql.DB, dir string, names ...string) {
	if dir == "" {
		return
	}
	ctx := c.Request.Context()
	for _, name := range names {
		if !appImageName.MatchString(name) {
			continue
		}
		var used bool
		err := db.AsSystem(ctx, dbh, func(tx *sql.Tx) error {
			return tx.QueryRowContext(ctx, `
				SELECT EXISTS (SELECT 1 FROM app.showcase_app
				                WHERE image = $1 OR screenshot = $1 OR published->>'image' = $1 OR published->>'screenshot' = $1)`, name).Scan(&used)
		})
		if err == nil && !used {
			removeAppImage(dir, name)
		}
	}
}

func registerAppImages(v1 *gin.RouterGroup, dir string) {
	v1.GET("/app-images/:file", allowParams(), func(c *gin.Context) {
		file := c.Param("file")
		if !appImageName.MatchString(file) {
			writeError(c, http.StatusNotFound, "not_found", "找不到這張圖片")
			return
		}
		path := filepath.Join(dir, appImageSubdir, file)
		if st, err := os.Stat(path); err != nil || !st.Mode().IsRegular() {
			writeError(c, http.StatusNotFound, "not_found", "找不到這張圖片")
			return
		}
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Header("X-Content-Type-Options", "nosniff")
		c.File(path)
	})
}

type adminApp struct {
	myApp
	UserID      int64  `json:"user_id"`
	DisplayName string `json:"display_name"`
}

func (h adminHandlers) showcaseApps(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	status := c.Query("status")
	if status != "" && !slices.Contains([]string{"pending", "approved", "rejected", "removed"}, status) {
		writeError(c, http.StatusBadRequest, "invalid_status", "狀態只能是 pending、approved、rejected 或 removed")
		return
	}
	ctx := c.Request.Context()
	list := []adminApp{}
	err := db.AsSystem(ctx, h.db, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT `+prefixColumns("a.", appColumns)+`, u.id, u.display_name
			  FROM app.showcase_app a JOIN app.users u ON u.id = a.user_id
			 WHERE $1 = '' OR a.status = $1
			 ORDER BY (a.status = 'pending') DESC, a.updated_at DESC
			 LIMIT 200`, status)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r adminApp
			a, err := scanApp(rows, &r.UserID, &r.DisplayName)
			if err != nil {
				return err
			}
			r.myApp = *a
			list = append(list, r)
		}
		return rows.Err()
	})
	if err != nil {
		adminFail(c, err)
		return
	}
	writeData(c, http.StatusOK, list, nil)
}

func prefixColumns(prefix, cols string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = prefix + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

func (h adminHandlers) reviewApp(c *gin.Context) {
	me, ok := mustAdmin(c)
	if !ok {
		return
	}
	id, ok := appParam(c)
	if !ok {
		return
	}
	var in struct {
		Action string `json:"action"`
		Note   string `json:"note"`
	}
	if !bindJSON(c, &in) {
		return
	}
	in.Note = strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(in.Note) > 200 {
		writeError(c, http.StatusBadRequest, "invalid_note", "原因最多 200 個字")
		return
	}
	var query string
	switch in.Action {
	case "approve":
		query = `
			UPDATE app.showcase_app
			   SET status = 'approved', review_note = '', reviewed_at = now(),
			       published = jsonb_build_object('category', category, 'name', name, 'description', description,
			                                      'web_url', web_url, 'ios_url', ios_url, 'android_url', android_url,
			                                      'image', image, 'screenshot', screenshot, 'topics', to_jsonb(topics), 'show_author', show_author),
			       published_at = COALESCE(published_at, now())
			 WHERE id = $1 AND status = 'pending'`
	case "reject":
		if in.Note == "" {
			writeError(c, http.StatusBadRequest, "note_required", "請填寫原因")
			return
		}
		query = `
			UPDATE app.showcase_app SET status = 'rejected', review_note = $2, reviewed_at = now()
			 WHERE id = $1 AND status = 'pending'`
	case "remove":
		if in.Note == "" {
			writeError(c, http.StatusBadRequest, "note_required", "請填寫原因")
			return
		}
		query = `
			UPDATE app.showcase_app
			   SET status = 'removed', review_note = $2, reviewed_at = now(), published = NULL, published_at = NULL
			 WHERE id = $1 AND published IS NOT NULL`
	default:
		writeError(c, http.StatusBadRequest, "invalid_action", "動作只能是 approve、reject 或 remove")
		return
	}
	ctx := c.Request.Context()
	var oldImage, oldShot sql.NullString
	var owner int64
	var name string
	var firstPublish bool
	err := db.AsSystem(ctx, h.db, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
			SELECT published->>'image', published->>'screenshot', user_id, name, published_at IS NULL
			  FROM app.showcase_app WHERE id = $1 FOR UPDATE`, id).
			Scan(&oldImage, &oldShot, &owner, &name, &firstPublish); err != nil {
			return err
		}
		args := []any{id}
		if in.Action != "approve" {
			args = append(args, in.Note)
		}
		res, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return sql.ErrNoRows
		}
		var detail map[string]any
		if in.Note != "" {
			detail = map[string]any{"note": in.Note}
		}
		return auth.AuditTx(ctx, tx, &me.ID, "admin.app."+in.Action, fmt.Sprintf("app:%d", id), detail, c.ClientIP())
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusConflict, "state_changed", "狀態已變更，請重新整理")
		return
	}
	if err != nil {
		adminFail(c, err)
		return
	}
	dropUnusedImages(c, h.db, h.imageDir, oldImage.String, oldShot.String)
	h.notify.AppReviewed(owner, name, in.Action, in.Note)
	if in.Action == "approve" && firstPublish {
		h.announceApp(ctx, id)
	}
	c.Status(http.StatusNoContent)
}

type publicApp struct {
	ID          int64     `json:"id"`
	Category    string    `json:"category"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	WebURL      string    `json:"web_url"`
	IOSURL      string    `json:"ios_url"`
	AndroidURL  string    `json:"android_url"`
	ImageURL    *string   `json:"image_url"`
	ScreenURL   *string   `json:"screenshot_url"`
	Topics      []string  `json:"topics"`
	Author      *string   `json:"author"`
	PublishedAt time.Time `json:"published_at"`
}

func publicApps(dbh *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		list := []publicApp{}
		err := db.AsSystem(ctx, dbh, func(tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, `
				SELECT a.id, a.published, a.published_at, u.display_name
				  FROM app.showcase_app a JOIN app.users u ON u.id = a.user_id
				 WHERE a.published IS NOT NULL AND u.status = 'active'
				 ORDER BY a.published_at DESC`)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var p publicApp
				var raw []byte
				var author string
				if err := rows.Scan(&p.ID, &raw, &p.PublishedAt, &author); err != nil {
					return err
				}
				var content appContent
				if err := json.Unmarshal(raw, &content); err != nil {
					return err
				}
				p.Category, p.Name, p.Description, p.Topics = content.Category, content.Name, content.Description, content.Topics
				p.WebURL, p.IOSURL, p.AndroidURL = content.WebURL, content.IOSURL, content.AndroidURL
				if appImageName.MatchString(content.Image) {
					u := "/api/v1/app-images/" + content.Image
					p.ImageURL = &u
				}
				if content.Category == "web" && appImageName.MatchString(content.Screenshot) {
					u := "/api/v1/app-images/" + content.Screenshot
					p.ScreenURL = &u
				}
				if content.ShowAuthor {
					p.Author = &author
				}
				list = append(list, p)
			}
			return rows.Err()
		})
		if err != nil {
			adminFail(c, err)
			return
		}
		c.Header("Cache-Control", "public, max-age=300")
		writeData(c, http.StatusOK, list, nil)
	}
}

var categoryNames = map[string]string{"app": "App", "web": "網站", "innovation": "創新應用"}

func (h adminHandlers) announceApp(ctx context.Context, id int64) {
	var raw []byte
	var author sql.NullString
	var topics string
	err := db.AsSystem(ctx, h.db, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT a.published, CASE WHEN a.show_author THEN u.display_name END,
			       COALESCE((SELECT string_agg(COALESCE(d.name, t), '、' ORDER BY ord)
			                   FROM unnest(a.topics) WITH ORDINALITY AS x(t, ord)
			                   LEFT JOIN app.dashboard d ON d.slug = x.t), '')
			  FROM app.showcase_app a JOIN app.users u ON u.id = a.user_id
			 WHERE a.id = $1 AND a.published IS NOT NULL`, id).Scan(&raw, &author, &topics)
	})
	var p appContent
	if err == nil {
		err = json.Unmarshal(raw, &p)
	}
	if err != nil {
		slog.Error("announce app", "app", id, "error", err)
		return
	}
	h.hooks.Emit(ctx, hooks.EventAppPublished, appAnnouncement(h.site, p, author.String, topics))
}

func appAnnouncement(site string, p appContent, author, topics string) hooks.Message {
	fields := []hooks.Field{{Name: "類別", Value: categoryNames[p.Category]}}
	if author != "" {
		fields = append(fields, hooks.Field{Name: "作者", Value: author})
	}
	if topics != "" {
		fields = append(fields, hooks.Field{Name: "用到的資料", Value: topics})
	}
	m := hooks.Message{
		Title: "新應用上架：" + p.Name, Text: p.Description, Level: hooks.LevelOK,
		URL: site + "/showcase/" + p.Category, Fields: fields,
	}
	if appImageName.MatchString(p.Image) {
		m.Thumbnail = site + "/api/v1/app-images/" + p.Image
	}
	if p.Category == "web" && appImageName.MatchString(p.Screenshot) {
		m.Image = site + "/api/v1/app-images/" + p.Screenshot
	}
	webLabel := "前往網站"
	if p.Category == "innovation" {
		webLabel = "查看作品"
	}
	for _, l := range []hooks.Link{{Label: webLabel, URL: p.WebURL, Icon: "web"}, {Label: "App Store", URL: p.IOSURL, Icon: "ios"},
		{Label: "Google Play", URL: p.AndroidURL, Icon: "android"}} {
		if l.URL != "" {
			m.Links = append(m.Links, l)
		}
	}
	m.Links = append(m.Links, hooks.Link{Label: "在臺灣儀表板查看", URL: m.URL, Icon: "site"})
	return m
}
