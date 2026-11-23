package notify

import (
	"context"
	"database/sql"
	"errors"
)

var SuppressReasons = map[string]bool{"hard_bounce": true, "spam": true, "invalid_email": true, "blocked": true}

func (n *Notifier) Suppress(ctx context.Context, email, reason, detail string) error {
	if n == nil || n.Store == nil || n.Store.Emails == nil || !SuppressReasons[reason] || email == "" {
		return nil
	}
	_, err := n.Store.DB.ExecContext(ctx, `
		INSERT INTO app.mail_suppression (email_lookup, reason, detail) VALUES ($1, $2, left($3, 300))
		ON CONFLICT (email_lookup) DO UPDATE SET reason = EXCLUDED.reason, detail = EXCLUDED.detail, created_at = now()`,
		n.Store.Emails.Lookup(email), reason, detail)
	return err
}

func (n *Notifier) suppressed(ctx context.Context, email string) (string, error) {
	if n.Store.Emails == nil {
		return "", nil
	}
	var reason string
	err := n.Store.DB.QueryRowContext(ctx, `SELECT reason FROM app.mail_suppression WHERE email_lookup = $1`,
		n.Store.Emails.Lookup(email)).Scan(&reason)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return reason, err
}
