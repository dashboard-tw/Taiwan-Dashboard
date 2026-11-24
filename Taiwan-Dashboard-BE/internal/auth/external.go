package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const OAuthLoginTTL = 10 * time.Minute

type OAuthLogin struct {
	StateHash   []byte
	BrowserHash []byte
	Provider    string
	Verifier    string
	RedirectURI string
	ReturnTo    string
	ExpiresAt   time.Time
	LinkUserID  int64
}

var ErrDisabled = errors.New("account disabled")

func (s *Store) SaveOAuthLogin(ctx context.Context, l OAuthLogin) error {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM app.oauth_login WHERE expires_at < $1`, s.now()); err != nil {
		return err
	}
	var link any
	if l.LinkUserID != 0 {
		link = l.LinkUserID
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO app.oauth_login (state_hash, browser_hash, provider, code_verifier, redirect_uri, return_to, expires_at, link_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		l.StateHash, l.BrowserHash, l.Provider, l.Verifier, l.RedirectURI, l.ReturnTo, l.ExpiresAt, link)
	return err
}

func (s *Store) TakeOAuthLogin(ctx context.Context, stateHash []byte, provider string) (*OAuthLogin, error) {
	l := OAuthLogin{StateHash: stateHash, Provider: provider}
	var link sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `
		DELETE FROM app.oauth_login
		 WHERE state_hash = $1 AND provider = $2
		RETURNING browser_hash, code_verifier, redirect_uri, return_to, expires_at, link_user_id`, stateHash, provider).
		Scan(&l.BrowserHash, &l.Verifier, &l.RedirectURI, &l.ReturnTo, &l.ExpiresAt, &link)
	l.LinkUserID = link.Int64
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

func (s *Store) SignInExternal(ctx context.Context, provider string, id Identity) (u *User, created bool, err error) {
	for attempt := 0; attempt < 2; attempt++ {
		u, err := s.findExternal(ctx, provider, id)
		if err == nil || !errors.Is(err, ErrNoSession) {
			return u, false, err
		}
		owner, status, err := s.findByEmail(ctx, id.Email)
		if err != nil {
			return nil, false, err
		}
		if owner != 0 {
			if status != "active" {
				return nil, false, ErrDisabled
			}
			return nil, false, &LinkRequired{UserID: owner}
		}
		u, err = s.createExternal(ctx, provider, id)
		if err == nil {
			return u, true, nil
		}
		if !isUniqueViolation(err) {
			return nil, false, err
		}
	}
	return nil, false, errors.New("sign-in: could not find or create the user")
}

func (s *Store) findExternal(ctx context.Context, provider string, id Identity) (*User, error) {
	var u User
	var status string
	err := s.DB.QueryRowContext(ctx, `
		SELECT u.id, u.display_name, u.role, u.status
		  FROM app.user_identity i JOIN app.users u ON u.id = i.user_id
		 WHERE i.provider = $1 AND i.subject = $2`, provider, id.Subject).
		Scan(&u.ID, &u.DisplayName, &u.Role, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, ErrDisabled
	}
	if err := s.saveEmail(ctx, s.DB, u.ID, id.Email, true); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) saveEmail(ctx context.Context, db execer, userID int64, email string, onlyIfChanged bool) error {
	if s.Emails == nil || email == "" {
		return nil
	}
	enc, err := s.Emails.Seal(userID, email)
	if err != nil {
		return err
	}
	fp := s.Emails.Fingerprint(userID, email)
	q := `UPDATE app.users SET email_enc = $2, email_hash = $3, email_lookup = $4 WHERE id = $1`
	if onlyIfChanged {
		q += ` AND (email_hash IS DISTINCT FROM $3 OR email_lookup IS DISTINCT FROM $4)`
	}
	_, err = db.ExecContext(ctx, q, userID, enc, fp, s.Emails.Lookup(email))
	return err
}

func (s *Store) UserEmail(ctx context.Context, userID int64) (string, error) {
	if s.Emails == nil {
		return "", nil
	}
	var enc []byte
	err := s.DB.QueryRowContext(ctx, `SELECT email_enc FROM app.users WHERE id = $1`, userID).Scan(&enc)
	if err != nil || enc == nil {
		return "", err
	}
	return s.Emails.Open(userID, enc)
}

func (s *Store) EmailBelongsTo(ctx context.Context, userID int64, email string) (bool, error) {
	if s.Emails == nil {
		return false, nil
	}
	var fp []byte
	err := s.DB.QueryRowContext(ctx, `SELECT email_hash FROM app.users WHERE id = $1`, userID).Scan(&fp)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && fp == nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return s.Emails.Matches(userID, email, fp), nil
}

func (s *Store) createExternal(ctx context.Context, provider string, id Identity) (*User, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	name := id.Name
	if name == "" {
		name = "使用者"
	}
	u := User{DisplayName: name, Role: RoleUser}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO app.users (display_name, role) VALUES ($1, 'user') RETURNING id`,
		name).Scan(&u.ID); err != nil {
		return nil, err
	}
	if err := s.saveEmail(ctx, tx, u.ID, id.Email, false); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO app.user_identity (user_id, provider, subject) VALUES ($1, $2, $3)`,
		u.ID, provider, id.Subject); err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, &u.ID, "account.create", fmt.Sprintf("user:%d", u.ID), map[string]any{"provider": provider}, ""); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.emit(ctx, Event{Kind: EventSignup, UserID: u.ID, Provider: provider})
	return &u, nil
}

func isUniqueViolation(err error) bool {
	var coded interface{ SQLState() string }
	return errors.As(err, &coded) && coded.SQLState() == "23505"
}
