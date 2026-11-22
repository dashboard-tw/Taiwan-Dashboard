package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const LinkTTL = 10 * time.Minute

type LinkRequired struct {
	UserID int64
}

func (e *LinkRequired) Error() string {
	return fmt.Sprintf("email belongs to user %d: confirm with the original sign-in", e.UserID)
}

var ErrIdentityTaken = errors.New("identity already belongs to an account")

var ErrProviderLinked = errors.New("account already has a sign-in with this provider")

var ErrLastIdentity = errors.New("the account's only sign-in method")

var ErrIdentityNotFound = errors.New("no such sign-in method")

type PendingLink struct {
	TokenHash []byte
	UserID    int64
	Provider  string
	Subject   string
	ExpiresAt time.Time
	Existing  []string
}

func (s *Store) findByEmail(ctx context.Context, email string) (id int64, status string, err error) {
	if s.Emails == nil || email == "" {
		return 0, "", nil
	}
	err = s.DB.QueryRowContext(ctx, `
		SELECT id, status FROM app.users WHERE email_lookup = $1 ORDER BY id LIMIT 1`,
		s.Emails.Lookup(email)).Scan(&id, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	return id, status, err
}

func (s *Store) SavePendingLink(ctx context.Context, l PendingLink) error {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM app.identity_link WHERE expires_at < $1`, s.now()); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO app.identity_link (token_hash, user_id, provider, subject, expires_at)
		VALUES ($1, $2, $3, $4, $5)`, l.TokenHash, l.UserID, l.Provider, l.Subject, l.ExpiresAt)
	return err
}

func (s *Store) PendingLinkInfo(ctx context.Context, tokenHash []byte) (*PendingLink, error) {
	l := PendingLink{TokenHash: tokenHash}
	var existing string
	err := s.DB.QueryRowContext(ctx, `
		SELECT l.user_id, l.provider, l.subject, l.expires_at,
		       COALESCE((SELECT string_agg(i.provider, ',' ORDER BY i.created_at)
		                   FROM app.user_identity i WHERE i.user_id = l.user_id), '')
		  FROM app.identity_link l
		 WHERE l.token_hash = $1 AND l.expires_at > $2`, tokenHash, s.now()).
		Scan(&l.UserID, &l.Provider, &l.Subject, &l.ExpiresAt, &existing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, err
	}
	l.Existing = strings.Split(existing, ",")
	if existing == "" {
		l.Existing = []string{}
	}
	return &l, nil
}

func (s *Store) TakePendingLink(ctx context.Context, tokenHash []byte) (*PendingLink, error) {
	l := PendingLink{TokenHash: tokenHash}
	err := s.DB.QueryRowContext(ctx, `
		DELETE FROM app.identity_link WHERE token_hash = $1
		RETURNING user_id, provider, subject, expires_at`, tokenHash).
		Scan(&l.UserID, &l.Provider, &l.Subject, &l.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, err
	}
	if !l.ExpiresAt.After(s.now()) {
		return nil, ErrNoSession
	}
	return &l, nil
}

func (s *Store) LinkIdentity(ctx context.Context, userID int64, provider, subject, ip string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var other bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM app.user_identity WHERE user_id = $1 AND provider = $2 AND subject <> $3)`,
		userID, provider, subject).Scan(&other); err != nil {
		return err
	}
	if other {
		return ErrProviderLinked
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO app.user_identity (user_id, provider, subject) VALUES ($1, $2, $3)
		ON CONFLICT (provider, subject) DO NOTHING`, userID, provider, subject)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var owner int64
		if err := tx.QueryRowContext(ctx, `
			SELECT user_id FROM app.user_identity WHERE provider = $1 AND subject = $2`, provider, subject).Scan(&owner); err != nil {
			return err
		}
		if owner != userID {
			return ErrIdentityTaken
		}
		return nil
	}
	if err := audit(ctx, tx, &userID, "identity.link", fmt.Sprintf("user:%d", userID), map[string]any{"provider": provider}, ip); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UnlinkIdentity(ctx context.Context, userID int64, provider, ip string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
		SELECT provider FROM app.user_identity WHERE user_id = $1 FOR UPDATE`, userID)
	if err != nil {
		return err
	}
	var have []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return err
		}
		have = append(have, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	found := false
	for _, p := range have {
		found = found || p == provider
	}
	if !found {
		return ErrIdentityNotFound
	}
	if len(have) < 2 {
		return ErrLastIdentity
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM app.user_identity WHERE user_id = $1 AND provider = $2`, userID, provider); err != nil {
		return err
	}
	if err := audit(ctx, tx, &userID, "identity.unlink", fmt.Sprintf("user:%d", userID), map[string]any{"provider": provider}, ip); err != nil {
		return err
	}
	return tx.Commit()
}
