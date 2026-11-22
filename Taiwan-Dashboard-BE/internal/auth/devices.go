package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"dashboard.tw/backend/internal/db"
)

const LastSeenInterval = 5 * time.Minute

var ErrDeviceNotFound = errors.New("device not found")

type Device struct {
	ID         int64      `json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	IP         string     `json:"ip"`
	UserAgent  string     `json:"user_agent"`
	Current    bool       `json:"current"`
}

func (s *Store) Devices(ctx context.Context, userID int64, currentToken string) ([]Device, error) {
	out := []Device{}
	err := db.AsUser(ctx, s.DB, userID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT id, created_at, last_seen_at, expires_at, COALESCE(host(ip), ''), COALESCE(user_agent, ''), id_hash = $3
			  FROM app.session
			 WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > $2
			 ORDER BY id_hash = $3 DESC, COALESCE(last_seen_at, created_at) DESC, id DESC`,
			userID, s.now(), HashToken(currentToken))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d Device
			if err := rows.Scan(&d.ID, &d.CreatedAt, &d.LastSeenAt, &d.ExpiresAt, &d.IP, &d.UserAgent, &d.Current); err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) SignOutDevice(ctx context.Context, userID, deviceID int64, currentToken, ip string) (current bool, err error) {
	err = db.AsUser(ctx, s.DB, userID, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
			UPDATE app.session SET revoked_at = now()
			 WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL AND expires_at > $3
			RETURNING id_hash = $4`, deviceID, userID, s.now(), HashToken(currentToken)).Scan(&current)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDeviceNotFound
		}
		if err != nil {
			return err
		}
		return audit(ctx, tx, &userID, "session.revoke", "", map[string]any{"device": deviceID}, ip)
	})
	return current, err
}

func (s *Store) SignOutOtherDevices(ctx context.Context, userID int64, currentToken, ip string) (n int64, err error) {
	err = db.AsUser(ctx, s.DB, userID, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE app.session SET revoked_at = now()
			 WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > $2 AND id_hash <> $3`,
			userID, s.now(), HashToken(currentToken))
		if err != nil {
			return err
		}
		if n, _ = res.RowsAffected(); n == 0 {
			return nil
		}
		return audit(ctx, tx, &userID, "session.revoke_others", "", map[string]any{"count": n}, ip)
	})
	return n, err
}
