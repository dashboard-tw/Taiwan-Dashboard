package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"dashboard.tw/backend/internal/auth"
	"dashboard.tw/backend/internal/db"
)

const idleAfter = 30 * 24 * time.Hour

type usageDay struct {
	Day      string `json:"day"`
	Requests int    `json:"requests"`
	Rejected int    `json:"rejected"`
	Accounts int    `json:"accounts"`
}

type usageAccount struct {
	UserID      int64      `json:"user_id"`
	DisplayName string     `json:"display_name"`
	Prefix      *string    `json:"prefix"`
	Status      string     `json:"status"` // active、idle、revoked
	Requests    int        `json:"requests"`
	Rejected    int        `json:"rejected"`
	DaysUsed    int        `json:"days_used"`
	LastUsedAt  *time.Time `json:"last_used_at"`
}

const accountStatus = `
	WITH acct AS (
	  SELECT u.id, u.display_name, k.prefix, k.last_used_at,
	         CASE WHEN k.id IS NULL THEN 'revoked'
	              WHEN k.last_used_at >= now() - $1::interval THEN 'active'
	              ELSE 'idle' END AS status
	    FROM app.users u
	    LEFT JOIN app.api_key k ON k.user_id = u.id AND k.revoked_at IS NULL
	   WHERE EXISTS (SELECT 1 FROM app.api_key x WHERE x.user_id = u.id))`

func (h adminHandlers) apiUsage(c *gin.Context) {
	if _, ok := mustAdmin(c); !ok {
		return
	}
	days, err := strconv.Atoi(c.DefaultQuery("days", "30"))
	if err != nil || (days != 7 && days != 30 && days != 90) {
		writeError(c, http.StatusBadRequest, "invalid_days", "days 只能是 7、30 或 90")
		return
	}
	ctx := c.Request.Context()
	idle := fmt.Sprintf("%d seconds", int(idleAfter.Seconds()))
	limit := auth.Tiers["free"].PerDay
	out := gin.H{"days": days, "idle_days": int(idleAfter.Hours() / 24), "daily_limit": limit}

	err = db.AsSystem(ctx, h.db, func(tx *sql.Tx) error {
		status := map[string]int{"active": 0, "idle": 0, "revoked": 0}
		var neverUsed int
		rows, err := tx.QueryContext(ctx, accountStatus+`
			SELECT status, count(*), count(*) FILTER (WHERE status = 'idle' AND last_used_at IS NULL)
			  FROM acct GROUP BY status`, idle)
		if err != nil {
			return err
		}
		for rows.Next() {
			var s string
			var n, never int
			if err := rows.Scan(&s, &n, &never); err != nil {
				rows.Close()
				return err
			}
			status[s], neverUsed = n, neverUsed+never
		}
		if err := closeRows(rows); err != nil {
			return err
		}
		out["accounts"] = gin.H{"active": status["active"], "idle": status["idle"], "revoked": status["revoked"], "never_used": neverUsed}

		daily := []usageDay{}
		rows, err = tx.QueryContext(ctx, `
			SELECT to_char(d, 'YYYY-MM-DD'), coalesce(sum(u.requests), 0)::int, coalesce(sum(u.rejected), 0)::int,
			       count(DISTINCT k.user_id) FILTER (WHERE u.requests > 0)::int
			  FROM generate_series((now() AT TIME ZONE 'Asia/Taipei')::date - ($1::int - 1),
			                       (now() AT TIME ZONE 'Asia/Taipei')::date, interval '1 day') d
			  LEFT JOIN app.api_usage_daily u ON u.day = d::date
			  LEFT JOIN app.api_key k ON k.id = u.api_key_id
			 GROUP BY d ORDER BY d`, days)
		if err != nil {
			return err
		}
		for rows.Next() {
			var d usageDay
			if err := rows.Scan(&d.Day, &d.Requests, &d.Rejected, &d.Accounts); err != nil {
				rows.Close()
				return err
			}
			daily = append(daily, d)
		}
		if err := closeRows(rows); err != nil {
			return err
		}
		out["daily"] = daily

		var total, rejected, accounts, quotaHits int
		if err := tx.QueryRowContext(ctx, `
			SELECT coalesce(sum(u.requests), 0)::int, coalesce(sum(u.rejected), 0)::int,
			       count(DISTINCT k.user_id) FILTER (WHERE u.requests > 0)::int,
			       count(*) FILTER (WHERE u.requests >= $2)::int
			  FROM app.api_usage_daily u JOIN app.api_key k ON k.id = u.api_key_id
			 WHERE u.day > (now() AT TIME ZONE 'Asia/Taipei')::date - $1::int`, days, limit).
			Scan(&total, &rejected, &accounts, &quotaHits); err != nil {
			return err
		}
		avgAccount := 0.0
		if accounts > 0 {
			avgAccount = float64(total) / float64(accounts)
		}
		out["totals"] = gin.H{
			"requests": total, "rejected": rejected, "accounts": accounts, "quota_hits": quotaHits,
			"avg_per_day": float64(total) / float64(days), "avg_per_account": avgAccount,
		}

		type datasetUse struct {
			Dataset  string `json:"dataset"`
			Requests int    `json:"requests"`
		}
		datasets := []datasetUse{}
		rows, err = tx.QueryContext(ctx, `
			SELECT dataset, sum(requests)::int FROM app.api_usage_dataset_daily
			 WHERE day > (now() AT TIME ZONE 'Asia/Taipei')::date - $1::int
			 GROUP BY dataset ORDER BY 2 DESC, 1`, days)
		if err != nil {
			return err
		}
		for rows.Next() {
			var d datasetUse
			if err := rows.Scan(&d.Dataset, &d.Requests); err != nil {
				rows.Close()
				return err
			}
			datasets = append(datasets, d)
		}
		if err := closeRows(rows); err != nil {
			return err
		}
		out["datasets"] = datasets

		list := []usageAccount{}
		rows, err = tx.QueryContext(ctx, accountStatus+`, use AS (
			  SELECT k.user_id, sum(u.requests)::int AS requests, sum(u.rejected)::int AS rejected,
			         count(*) FILTER (WHERE u.requests > 0)::int AS days_used
			    FROM app.api_usage_daily u JOIN app.api_key k ON k.id = u.api_key_id
			   WHERE u.day > (now() AT TIME ZONE 'Asia/Taipei')::date - $2::int
			   GROUP BY k.user_id)
			SELECT a.id, a.display_name, a.prefix, a.status, coalesce(x.requests, 0), coalesce(x.rejected, 0),
			       coalesce(x.days_used, 0),
			       (SELECT max(last_used_at) FROM app.api_key k WHERE k.user_id = a.id)
			  FROM acct a
			  LEFT JOIN use x ON x.user_id = a.id
			 ORDER BY coalesce(x.requests, 0) DESC, a.id
			 LIMIT 200`, idle, days)
		if err != nil {
			return err
		}
		for rows.Next() {
			var a usageAccount
			if err := rows.Scan(&a.UserID, &a.DisplayName, &a.Prefix, &a.Status, &a.Requests, &a.Rejected,
				&a.DaysUsed, &a.LastUsedAt); err != nil {
				rows.Close()
				return err
			}
			list = append(list, a)
		}
		if err := closeRows(rows); err != nil {
			return err
		}
		out["top"] = list
		return nil
	})
	if err != nil {
		adminFail(c, err)
		return
	}
	writeData(c, http.StatusOK, out, nil)
}

func closeRows(rows *sql.Rows) error {
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	return rows.Close()
}

func (h adminHandlers) revokeUserKey(c *gin.Context) {
	u, ok := mustAdmin(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusNotFound, "not_found", "這個帳號沒有使用中的 API 憑證")
		return
	}
	ctx := c.Request.Context()
	var names []string
	err = db.AsSystem(ctx, h.db, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			UPDATE app.api_key SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL RETURNING id, name`, id)
		if err != nil {
			return err
		}
		var ids []int64
		for rows.Next() {
			var keyID int64
			var name string
			if err := rows.Scan(&keyID, &name); err != nil {
				rows.Close()
				return err
			}
			ids, names = append(ids, keyID), append(names, name)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(ids) == 0 {
			return sql.ErrNoRows
		}
		for _, keyID := range ids {
			if err := auth.AuditTx(ctx, tx, &u.ID, "admin.apikey.revoke", fmt.Sprintf("api_key:%d", keyID),
				map[string]any{"user": id}, c.ClientIP()); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "not_found", "這個帳號沒有使用中的 API 憑證")
		return
	}
	if err != nil {
		adminFail(c, err)
		return
	}
	h.notify.APIKeysRevoked(id, names)
	c.Status(http.StatusNoContent)
}
