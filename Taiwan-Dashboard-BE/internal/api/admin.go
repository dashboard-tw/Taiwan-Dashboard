package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/component"
	"dashboard.tw/backend/internal/hooks"
	"dashboard.tw/backend/internal/notify"
	"dashboard.tw/backend/internal/pipeline"
)

type AdminDeps struct {
	RunDataset func(ctx context.Context, id string) (pipeline.Result, error)
	RunOrder   []string
	Reader     component.QueryFunc
}

func registerAdmin(v1 *gin.RouterGroup, a AuthDeps, ad AdminDeps) {
	g := v1.Group("/admin", private(), a.sameOrigin(), a.session(), requireAdmin())
	h := adminHandlers{db: a.Store.DB, store: a.Store, run: ad.RunDataset, query: ad.Reader, order: ad.RunOrder,
		all: &runAll{}, imageDir: a.ImageDir, notify: a.Notify, hooks: a.Hooks, sealer: sealerOf(a.Store), site: siteOf(a.Origins)}
	g.GET("/overview", h.overview)
	g.GET("/mail-templates", h.mailTemplates)
	g.PUT("/mail-templates/:key", h.updateMailTemplate)
	g.POST("/mail-templates/:key/preview", h.previewMailTemplate)
	g.POST("/mail-templates/:key/test", h.testMailTemplate)
	g.GET("/mail-queue", h.mailQueue)
	g.POST("/mail-queue/:id/retry", h.retryMail)
	g.GET("/mail-suppressions", h.mailSuppressions)
	g.GET("/channels", h.channels)
	g.POST("/channels", h.createChannel)
	g.PUT("/channels/:id", h.updateChannel)
	g.DELETE("/channels/:id", h.deleteChannel)
	g.POST("/channels/:id/test", h.testChannel)
	g.DELETE("/mail-suppressions/:id", h.deleteSuppression)
	g.GET("/datasets", h.datasets)
	g.GET("/datasets/run-all", h.runAllStatus)
	g.POST("/datasets/run-all", h.startRunAll)
	g.POST("/datasets/:id/run", h.runDataset)
	g.GET("/pipeline-runs", h.pipelineRuns)
	g.GET("/ref/reservoirs", h.reservoirs)
	g.PUT("/ref/reservoirs/:id", h.updateReservoir)
	g.GET("/ref/power-units", h.powerUnits)
	g.PUT("/ref/power-units/:id", h.updatePowerUnit)
	g.GET("/api-survey", h.apiSurvey)
	g.GET("/api-usage", h.apiUsage)
	g.DELETE("/users/:id/api-key", h.revokeUserKey)
	g.POST("/users/search", newLimiter(30, 10).middleware(), h.searchUsers)
	g.GET("/users/:id", h.userDetail)
	g.GET("/apps", h.showcaseApps)
	g.POST("/apps/:id/review", h.reviewApp)
	g.PATCH("/users/:id", h.updateUser)
	g.GET("/components", h.components)
	g.POST("/components", h.createComponent)
	g.POST("/components/preview", h.previewComponent)
	g.GET("/components/:slug", h.componentSpec)
	g.PUT("/components/:slug", h.updateComponent)
	g.POST("/components/:slug/status", h.setStatus)
	g.GET("/components/:slug/versions", h.componentVersions)
	g.GET("/components/:slug/versions/:version", h.componentVersion)
	g.POST("/components/:slug/versions/:version/restore", h.restoreVersion)
}

type adminHandlers struct {
	db       *sql.DB
	store    *auth.Store
	run      func(ctx context.Context, id string) (pipeline.Result, error)
	query    component.QueryFunc
	order    []string
	all      *runAll
	imageDir string
	notify   *notify.Notifier
	hooks    *hooks.Dispatcher
	sealer   hooks.Sealer
	site     string
}

type datasetHealth struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Agency         string     `json:"agency"`
	URL            string     `json:"url"`
	Status         string     `json:"status"`
	UpdateFreqSec  int64      `json:"update_freq_seconds"`
	StaleAfterSec  int64      `json:"stale_after_seconds"`
	LastObservedAt *time.Time `json:"last_observed_at"`
	LastIngestedAt *time.Time `json:"last_ingested_at"`
	LastError      *string    `json:"last_error"`
	RunsToday      int        `json:"runs_today"`
	FailuresToday  int        `json:"failures_today"`
}

func (h adminHandlers) datasets(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT d.id, d.name, d.source_agency, d.source_url, `+component.DatasetStatusSQL("d")+`,
		       extract(epoch FROM d.update_freq)::bigint, extract(epoch FROM d.stale_after)::bigint,
		       d.last_observed_at, d.last_ingested_at, d.last_error,
		       count(r.id), count(r.id) FILTER (WHERE r.status = 'failed')
		  FROM app.dataset d
		  LEFT JOIN app.pipeline_run r
		    ON r.dataset_id = d.id AND r.started_at >= date_trunc('day', now() AT TIME ZONE 'Asia/Taipei') AT TIME ZONE 'Asia/Taipei'
		 GROUP BY d.id
		 ORDER BY d.id`)
	if err != nil {
		adminFail(c, err)
		return
	}
	defer rows.Close()
	out := []datasetHealth{}
	for rows.Next() {
		var d datasetHealth
		if err := rows.Scan(&d.ID, &d.Name, &d.Agency, &d.URL, &d.Status, &d.UpdateFreqSec, &d.StaleAfterSec,
			&d.LastObservedAt, &d.LastIngestedAt, &d.LastError, &d.RunsToday, &d.FailuresToday); err != nil {
			adminFail(c, err)
			return
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		adminFail(c, err)
		return
	}
	writeData(c, http.StatusOK, out, nil)
}

type pipelineRun struct {
	ID           int64           `json:"id"`
	Dataset      string          `json:"dataset"`
	StartedAt    time.Time       `json:"started_at"`
	FinishedAt   *time.Time      `json:"finished_at"`
	Status       string          `json:"status"`
	RowsFetched  int             `json:"rows_fetched"`
	RowsInserted int             `json:"rows_inserted"`
	Warnings     json.RawMessage `json:"warnings"`
	Error        *string         `json:"error"`
}

func (h adminHandlers) pipelineRuns(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	limit := 50
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeError(c, http.StatusBadRequest, "invalid_limit", "limit 必須是 1 到 500")
			return
		}
		limit = n
	}
	var dataset any
	if v := c.Query("dataset"); v != "" {
		dataset = v
	}
	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT id, dataset_id, started_at, finished_at, status, rows_fetched, rows_inserted, warnings, error
		  FROM app.pipeline_run
		 WHERE $1::text IS NULL OR dataset_id = $1
		 ORDER BY id DESC
		 LIMIT $2`, dataset, limit)
	if err != nil {
		adminFail(c, err)
		return
	}
	defer rows.Close()
	out := []pipelineRun{}
	for rows.Next() {
		var r pipelineRun
		var warnings []byte
		if err := rows.Scan(&r.ID, &r.Dataset, &r.StartedAt, &r.FinishedAt, &r.Status,
			&r.RowsFetched, &r.RowsInserted, &warnings, &r.Error); err != nil {
			adminFail(c, err)
			return
		}
		r.Warnings = warnings
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		adminFail(c, err)
		return
	}
	writeData(c, http.StatusOK, out, nil)
}

func adminFail(c *gin.Context, err error) {
	slog.Error("admin api", "path", c.Request.URL.Path, "error", err)
	writeError(c, http.StatusInternalServerError, "internal_error", "伺服器發生錯誤")
}
