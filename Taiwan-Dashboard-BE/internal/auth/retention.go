package auth

import (
	"context"
	"database/sql"
	"time"

	"dashboard.tw/backend/internal/db"
)

const (
	SessionRetention       = 30 * 24 * time.Hour
	AuditIPRetention       = 90 * 24 * time.Hour
	AuditRetention         = 365 * 24 * time.Hour
	RunRetention           = 90 * 24 * time.Hour
	DeletedClientRetention = 90 * 24 * time.Hour
)

type Purged struct {
	Sessions, AuditIPs, OAuthLogins, Audit, Runs, Clients int64
}

func Purge(ctx context.Context, sqlDB *sql.DB, now time.Time) (Purged, error) {
	var p Purged
	err := db.AsSystem(ctx, sqlDB, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			DELETE FROM app.session WHERE LEAST(expires_at, COALESCE(revoked_at, expires_at)) < $1`,
			now.Add(-SessionRetention))
		if err != nil {
			return err
		}
		p.Sessions, _ = res.RowsAffected()
		res, err = tx.ExecContext(ctx, `
			UPDATE app.audit_log SET ip = NULL WHERE ip IS NOT NULL AND created_at < $1`,
			now.Add(-AuditIPRetention))
		if err != nil {
			return err
		}
		p.AuditIPs, _ = res.RowsAffected()
		res, err = tx.ExecContext(ctx, `DELETE FROM app.audit_log WHERE created_at < $1`, now.Add(-AuditRetention))
		if err != nil {
			return err
		}
		p.Audit, _ = res.RowsAffected()
		res, err = tx.ExecContext(ctx, `
			DELETE FROM app.pipeline_run WHERE started_at < $1 AND status <> 'running'`, now.Add(-RunRetention))
		if err != nil {
			return err
		}
		p.Runs, _ = res.RowsAffected()
		res, err = tx.ExecContext(ctx, `
			DELETE FROM app.api_key WHERE revoked_at < $1`, now.Add(-DeletedClientRetention))
		if err != nil {
			return err
		}
		p.Clients, _ = res.RowsAffected()
		res, err = tx.ExecContext(ctx, `DELETE FROM app.oauth_login WHERE expires_at < $1`, now)
		if err != nil {
			return err
		}
		p.OAuthLogins, _ = res.RowsAffected()
		if _, err = tx.ExecContext(ctx, `DELETE FROM app.identity_link WHERE expires_at < $1`, now); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM app.api_token WHERE expires_at < $1`, now)
		return err
	})
	return p, err
}
