package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"dashboard.tw/backend/internal/db"
)

const (
	OAuthScope      = "mcp"
	OAuthAccessTTL  = time.Hour
	OAuthRefreshTTL = 90 * 24 * time.Hour
	OAuthCodeTTL    = 5 * time.Minute
	MetadataDocTTL  = 24 * time.Hour
)

var (
	ErrOAuthClient = errors.New("unknown client")
	ErrOAuthGrant  = errors.New("invalid grant")
	ErrNoOAuthApp  = errors.New("authorization not found")
)

type OAuthClient struct {
	ID           string
	Name         string
	ClientURI    string
	RedirectURIs []string
	AuthMethod   string
	MetadataDoc  bool
	FetchedAt    *time.Time
	secretHash   []byte
}

func (c *OAuthClient) CheckSecret(secret string) bool {
	if c.AuthMethod == "none" {
		return true
	}
	return secret != "" && c.secretHash != nil && subtle.ConstantTimeCompare(c.secretHash, hashOf(secret)) == 1
}

type OAuthTokens struct {
	Access  string
	Refresh string
	Expires time.Time
	Scope   string
}

type OAuthApp struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	ClientURI  string     `json:"client_uri"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func (s *Store) RegisterOAuthClient(ctx context.Context, c OAuthClient) (*OAuthClient, string, error) {
	id, err := randomString(18)
	if err != nil {
		return nil, "", err
	}
	c.ID = "mcp-" + id
	c.Name = clip(c.Name, 100)
	c.MetadataDoc = false
	secret := ""
	if c.AuthMethod != "none" {
		if secret, err = randomString(32); err != nil {
			return nil, "", err
		}
		c.secretHash = hashOf(secret)
	}
	err = db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO app.oauth_client (id, name, client_uri, redirect_uris, auth_method, secret_hash)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			c.ID, c.Name, c.ClientURI, c.RedirectURIs, c.AuthMethod, c.secretHash)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return &c, secret, nil
}

func (s *Store) SaveMetadataClient(ctx context.Context, c OAuthClient) error {
	c.Name = clip(c.Name, 100)
	return db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO app.oauth_client (id, name, client_uri, redirect_uris, auth_method, metadata_doc, fetched_at)
			VALUES ($1, $2, $3, $4, 'none', true, $5)
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, client_uri = EXCLUDED.client_uri,
			       redirect_uris = EXCLUDED.redirect_uris, fetched_at = EXCLUDED.fetched_at
			 WHERE app.oauth_client.metadata_doc`,
			c.ID, c.Name, c.ClientURI, c.RedirectURIs, s.now())
		return err
	})
}

func (s *Store) OAuthClientByID(ctx context.Context, id string) (*OAuthClient, error) {
	if id == "" || len(id) > 500 {
		return nil, ErrOAuthClient
	}
	var c OAuthClient
	err := db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT id, name, client_uri, redirect_uris, auth_method, metadata_doc, fetched_at, secret_hash
			  FROM app.oauth_client WHERE id = $1`, id).
			Scan(&c.ID, &c.Name, &c.ClientURI, pgtype.NewMap().SQLScanner(&c.RedirectURIs), &c.AuthMethod, &c.MetadataDoc, &c.FetchedAt, &c.secretHash)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOAuthClient
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

type OAuthRequest struct {
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	Scope         string
	Resource      string
}

func (s *Store) CreateOAuthCode(ctx context.Context, userID int64, r OAuthRequest) (string, error) {
	code, err := randomString(32)
	if err != nil {
		return "", err
	}
	err = db.AsUser(ctx, s.DB, userID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO app.oauth_code (code_hash, client_id, user_id, redirect_uri, code_challenge, scope, resource, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			hashOf(code), r.ClientID, userID, r.RedirectURI, r.CodeChallenge, r.Scope, r.Resource, s.now().Add(OAuthCodeTTL))
		return err
	})
	if err != nil {
		return "", err
	}
	return code, nil
}

func pkceS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func validVerifier(v string) bool {
	if len(v) < 43 || len(v) > 128 {
		return false
	}
	for _, r := range v {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == '_' || r == '~') {
			return false
		}
	}
	return true
}

func (s *Store) ExchangeOAuthCode(ctx context.Context, c *OAuthClient, code, redirectURI, verifier, resource, ip string) (*OAuthTokens, error) {
	if code == "" || len(code) > 100 || !validVerifier(verifier) {
		return nil, ErrOAuthGrant
	}
	var clientID, redirect, challenge, scope, res string
	var userID int64
	var expires time.Time
	err := db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			DELETE FROM app.oauth_code WHERE code_hash = $1
			RETURNING client_id, user_id, redirect_uri, code_challenge, scope, resource, expires_at`, hashOf(code)).
			Scan(&clientID, &userID, &redirect, &challenge, &scope, &res, &expires)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOAuthGrant
	}
	if err != nil {
		return nil, err
	}
	if clientID != c.ID || redirect != redirectURI || !expires.After(s.now()) ||
		subtle.ConstantTimeCompare([]byte(challenge), []byte(pkceS256(verifier))) != 1 ||
		(resource != "" && resource != res) {
		return nil, ErrOAuthGrant
	}
	var out *OAuthTokens
	err = db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		var active bool
		if err := tx.QueryRowContext(ctx, `SELECT status = 'active' FROM app.users WHERE id = $1 FOR UPDATE`, userID).Scan(&active); err != nil || !active {
			return ErrOAuthGrant
		}
		keyID, err := s.oauthGrant(ctx, tx, userID, c, ip)
		if err != nil {
			return err
		}
		out, err = s.issueOAuth(ctx, tx, keyID, scope)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.touchOAuthClient(ctx, c.ID)
	return out, nil
}

func (s *Store) oauthGrant(ctx context.Context, tx *sql.Tx, userID int64, c *OAuthClient, ip string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM app.api_key
		 WHERE user_id = $1 AND oauth_client_id = $2 AND kind = 'oauth' AND revoked_at IS NULL`, userID, c.ID).Scan(&id)
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE app.api_key SET name = $2, disabled_at = NULL WHERE id = $1`, id, clip(c.Name, 50))
		return id, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	prefix, err := randomString(12)
	if err != nil {
		return 0, err
	}
	filler, err := randomString(32)
	if err != nil {
		return 0, err
	}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO app.api_key (user_id, name, prefix, key_hash, tier, kind, oauth_client_id)
		VALUES ($1, $2, $3, $4, 'free', 'oauth', $5) RETURNING id`,
		userID, clip(c.Name, 50), "oauth-"+prefix, hashOf(filler), c.ID).Scan(&id); err != nil {
		return 0, err
	}
	return id, audit(ctx, tx, &userID, "oauth.grant", fmt.Sprintf("api_key:%d", id), map[string]any{"client": c.Name}, ip)
}

func (s *Store) issueOAuth(ctx context.Context, tx *sql.Tx, keyID int64, scope string) (*OAuthTokens, error) {
	access, err := randomString(32)
	if err != nil {
		return nil, err
	}
	refresh, err := randomString(32)
	if err != nil {
		return nil, err
	}
	now := s.now()
	t := &OAuthTokens{Access: access, Refresh: refresh, Expires: now.Add(OAuthAccessTTL), Scope: scope}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app.api_token (token_hash, api_key_id, expires_at) VALUES ($1, $2, $3)`,
		hashOf(access), keyID, t.Expires); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app.oauth_refresh (token_hash, api_key_id, expires_at) VALUES ($1, $2, $3)`,
		hashOf(refresh), keyID, now.Add(OAuthRefreshTTL)); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) RefreshOAuth(ctx context.Context, c *OAuthClient, refresh string) (*OAuthTokens, error) {
	if refresh == "" || len(refresh) > 100 {
		return nil, ErrOAuthGrant
	}
	var out *OAuthTokens
	err := db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		var keyID int64
		var expires time.Time
		err := tx.QueryRowContext(ctx, `DELETE FROM app.oauth_refresh WHERE token_hash = $1 RETURNING api_key_id, expires_at`,
			hashOf(refresh)).Scan(&keyID, &expires)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrOAuthGrant
		}
		if err != nil {
			return err
		}
		var ok bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM app.api_key k JOIN app.users u ON u.id = k.user_id
			                WHERE k.id = $1 AND k.kind = 'oauth' AND k.oauth_client_id = $2
			                  AND k.revoked_at IS NULL AND k.disabled_at IS NULL AND u.status = 'active')`,
			keyID, c.ID).Scan(&ok); err != nil {
			return err
		}
		if !ok || !expires.After(s.now()) {
			return ErrOAuthGrant
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM app.api_token WHERE api_key_id = $1 AND expires_at < $2`, keyID, s.now()); err != nil {
			return err
		}
		out, err = s.issueOAuth(ctx, tx, keyID, OAuthScope)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.touchOAuthClient(ctx, c.ID)
	return out, nil
}

func (s *Store) RevokeOAuthToken(ctx context.Context, c *OAuthClient, token string) error {
	if token == "" || len(token) > 100 {
		return nil
	}
	h := hashOf(token)
	return db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		var keyID int64
		err := tx.QueryRowContext(ctx, `
			SELECT r.api_key_id FROM app.oauth_refresh r JOIN app.api_key k ON k.id = r.api_key_id
			 WHERE r.token_hash = $1 AND k.oauth_client_id = $2`, h, c.ID).Scan(&keyID)
		if err == nil {
			return revokeGrant(ctx, tx, keyID)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			DELETE FROM app.api_token t USING app.api_key k
			 WHERE t.token_hash = $1 AND k.id = t.api_key_id AND k.oauth_client_id = $2`, h, c.ID)
		return err
	})
}

func revokeGrant(ctx context.Context, tx *sql.Tx, keyID int64) error {
	if _, err := tx.ExecContext(ctx, `UPDATE app.api_key SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, keyID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app.oauth_refresh WHERE api_key_id = $1`, keyID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM app.api_token WHERE api_key_id = $1`, keyID)
	return err
}

func (s *Store) touchOAuthClient(ctx context.Context, id string) {
	_ = db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			UPDATE app.oauth_client SET last_used_at = now()
			 WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 hour')`, id)
		return err
	})
}

func (s *Store) OAuthApps(ctx context.Context, userID int64) ([]OAuthApp, error) {
	out := []OAuthApp{}
	err := db.AsUser(ctx, s.DB, userID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT id, name, created_at, last_used_at FROM app.api_key
			 WHERE kind = 'oauth' AND revoked_at IS NULL ORDER BY created_at`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a OAuthApp
			if err := rows.Scan(&a.ID, &a.Name, &a.CreatedAt, &a.LastUsedAt); err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	if err != nil || len(out) == 0 {
		return out, err
	}
	err = db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		for i := range out {
			if err := tx.QueryRowContext(ctx, `
				SELECT c.client_uri FROM app.api_key k JOIN app.oauth_client c ON c.id = k.oauth_client_id
				 WHERE k.id = $1`, out[i].ID).Scan(&out[i].ClientURI); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		return nil
	})
	return out, err
}

func (s *Store) RevokeOAuthApp(ctx context.Context, userID, id int64, ip string) error {
	return db.AsUser(ctx, s.DB, userID, func(tx *sql.Tx) error {
		var found int64
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM app.api_key WHERE id = $1 AND kind = 'oauth' AND revoked_at IS NULL FOR UPDATE`, id).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoOAuthApp
		}
		if err != nil {
			return err
		}
		if err := revokeGrant(ctx, tx, id); err != nil {
			return err
		}
		return audit(ctx, tx, &userID, "oauth.revoke", fmt.Sprintf("api_key:%d", id), nil, ip)
	})
}
