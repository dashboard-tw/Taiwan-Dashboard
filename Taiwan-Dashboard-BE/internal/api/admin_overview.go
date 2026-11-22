package api

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/component"
	"dashboard.tw/backend/internal/db"
)

type overviewDay struct {
	Day         string `json:"day"`
	NewUsers    int    `json:"new_users"`
	APIRequests int    `json:"api_requests"`
	Failures    int    `json:"pipeline_failures"`
}

type overviewProblem struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	LastIngestedAt *time.Time `json:"last_ingested_at"`
	LastError      *string    `json:"last_error"`
}

type overviewFailure struct {
	Dataset   string    `json:"dataset"`
	StartedAt time.Time `json:"started_at"`
	Error     *string   `json:"error"`
}

func (h adminHandlers) overview(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	ctx := c.Request.Context()
	out := gin.H{}
	err := db.AsSystem(ctx, h.db, func(tx *sql.Tx) error {
		var users struct {
			Total    int `json:"total"`
			Admins   int `json:"admins"`
			Disabled int `json:"disabled"`
			New7d    int `json:"new_7d"`
			Active7d int `json:"active_7d"`
			Sessions int `json:"sessions"`
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*), count(*) FILTER (WHERE role = 'admin'), count(*) FILTER (WHERE status = 'disabled'),
			       count(*) FILTER (WHERE created_at >= now() - interval '7 days'),
			       count(*) FILTER (WHERE last_login_at >= now() - interval '7 days'),
			       (SELECT count(*) FROM app.session WHERE revoked_at IS NULL AND expires_at > now())
			  FROM app.users`).
			Scan(&users.Total, &users.Admins, &users.Disabled, &users.New7d, &users.Active7d, &users.Sessions); err != nil {
			return err
		}
		out["users"] = users

		var api struct {
			Clients       int   `json:"clients"`
			Accounts      int   `json:"accounts"`
			RequestsToday int   `json:"requests_today"`
			BytesToday    int64 `json:"bytes_today"`
			RejectedToday int   `json:"rejected_today"`
			Requests7d    int   `json:"requests_7d"`
		}
		if err := tx.QueryRowContext(ctx, `
			WITH today AS (SELECT (now() AT TIME ZONE 'Asia/Taipei')::date AS d)
			SELECT (SELECT count(*) FROM app.api_key WHERE revoked_at IS NULL),
			       (SELECT count(DISTINCT user_id) FROM app.api_key WHERE revoked_at IS NULL AND disabled_at IS NULL),
			       coalesce(sum(u.requests) FILTER (WHERE u.day = today.d), 0),
			       coalesce(sum(u.bytes) FILTER (WHERE u.day = today.d), 0),
			       coalesce(sum(u.rejected) FILTER (WHERE u.day = today.d), 0),
			       coalesce(sum(u.requests) FILTER (WHERE u.day > today.d - 7), 0)
			  FROM today LEFT JOIN app.api_usage_daily u ON u.day > today.d - 7
			 GROUP BY today.d`).
			Scan(&api.Clients, &api.Accounts, &api.RequestsToday, &api.BytesToday, &api.RejectedToday, &api.Requests7d); err != nil {
			return err
		}
		out["api"] = api

		datasets := map[string]int{"ok": 0, "stale": 0, "error": 0, "pending": 0}
		rows, err := tx.QueryContext(ctx, `
			SELECT s, count(*) FROM (SELECT `+component.DatasetStatusSQL("d")+` AS s FROM app.dataset d) x GROUP BY s`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var s string
			var n int
			if err := rows.Scan(&s, &n); err != nil {
				rows.Close()
				return err
			}
			datasets[s] = n
		}
		if err := rows.Close(); err != nil {
			return err
		}
		out["datasets"] = datasets

		problems := []overviewProblem{}
		rows, err = tx.QueryContext(ctx, `
			SELECT id, name, s, last_ingested_at, last_error FROM (
			  SELECT d.id, d.name, `+component.DatasetStatusSQL("d")+` AS s, d.last_ingested_at, d.last_error
			    FROM app.dataset d) x
			 WHERE s IN ('stale', 'error')
			 ORDER BY s, id LIMIT 20`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var p overviewProblem
			if err := rows.Scan(&p.ID, &p.Name, &p.Status, &p.LastIngestedAt, &p.LastError); err != nil {
				rows.Close()
				return err
			}
			problems = append(problems, p)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		out["problems"] = problems

		var runs struct {
			Total  int `json:"total"`
			Failed int `json:"failed"`
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*), count(*) FILTER (WHERE status = 'failed')
			  FROM app.pipeline_run WHERE started_at >= now() - interval '24 hours'`).
			Scan(&runs.Total, &runs.Failed); err != nil {
			return err
		}
		out["runs_24h"] = runs

		failures := []overviewFailure{}
		rows, err = tx.QueryContext(ctx, `
			SELECT dataset_id, started_at, left(error, 300) FROM app.pipeline_run
			 WHERE status = 'failed' ORDER BY started_at DESC LIMIT 8`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var f overviewFailure
			if err := rows.Scan(&f.Dataset, &f.StartedAt, &f.Error); err != nil {
				rows.Close()
				return err
			}
			failures = append(failures, f)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		out["failures"] = failures

		var comps struct {
			Published  int `json:"published"`
			Draft      int `json:"draft"`
			Dashboards int `json:"dashboards"`
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*) FILTER (WHERE status = 'published'), count(*) FILTER (WHERE status <> 'published'),
			       (SELECT count(*) FROM app.dashboard)
			  FROM app.component`).Scan(&comps.Published, &comps.Draft, &comps.Dashboards); err != nil {
			return err
		}
		out["components"] = comps

		days := []overviewDay{}
		rows, err = tx.QueryContext(ctx, `
			WITH days AS (
			  SELECT generate_series((now() AT TIME ZONE 'Asia/Taipei')::date - 13,
			                         (now() AT TIME ZONE 'Asia/Taipei')::date, interval '1 day')::date AS d)
			SELECT to_char(days.d, 'YYYY-MM-DD'),
			       (SELECT count(*) FROM app.users u WHERE (u.created_at AT TIME ZONE 'Asia/Taipei')::date = days.d),
			       (SELECT coalesce(sum(a.requests), 0) FROM app.api_usage_daily a WHERE a.day = days.d),
			       (SELECT count(*) FROM app.pipeline_run r
			         WHERE r.status = 'failed' AND (r.started_at AT TIME ZONE 'Asia/Taipei')::date = days.d)
			  FROM days ORDER BY days.d`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var d overviewDay
			if err := rows.Scan(&d.Day, &d.NewUsers, &d.APIRequests, &d.Failures); err != nil {
				rows.Close()
				return err
			}
			days = append(days, d)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		out["days"] = days
		return nil
	})
	if err != nil {
		adminFail(c, err)
		return
	}
	writeData(c, http.StatusOK, out, nil)
}
