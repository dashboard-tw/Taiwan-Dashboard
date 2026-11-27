package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"dashboard.tw/backend/internal/db"
)

type Tier struct {
	PerDay      int
	PerMinute   int
	BytesPerDay int64
	Unlimited   bool // 開發帳號：不限次數、流量與每分鐘次數
}

var Tiers = map[string]Tier{
	TierFree:    {PerDay: 600, PerMinute: 30, BytesPerDay: 300 << 20},
	TierPartner: {PerDay: 3000, PerMinute: 60, BytesPerDay: 1536 << 20},
	TierDev:     {Unlimited: true},
}

const (
	TierFree    = "free"
	TierPartner = "partner"
	TierDev     = "dev"
)

func tierSQL(user string) string {
	return `CASE WHEN EXISTS (SELECT 1 FROM app.users d WHERE d.id = ` + user + ` AND d.is_dev) THEN '` + TierDev +
		`' WHEN EXISTS (SELECT 1 FROM app.showcase_app a WHERE a.user_id = ` + user +
		` AND a.published IS NOT NULL) THEN '` + TierPartner + `' ELSE '` + TierFree + `' END`
}

// DevKeysSQL lists the API keys of dev accounts, so admin statistics can leave them out.
const DevKeysSQL = `(SELECT dk.id FROM app.api_key dk JOIN app.users du ON du.id = dk.user_id WHERE du.is_dev)`

func (s *Store) TierOf(ctx context.Context, userID int64) (string, error) {
	var tier string
	err := db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT `+tierSQL("$1"), userID).Scan(&tier)
	})
	return tier, err
}

const (
	MaxClients = 3
	TokenTTL   = 24 * time.Hour
)

var (
	ErrNoKey          = errors.New("invalid access token")
	ErrInvalidClient  = errors.New("invalid client credentials")
	ErrClientNotFound = errors.New("credential not found")
	ErrTooManyClients = errors.New("too many credentials")
)

type APIClient struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	ClientID   string     `json:"client_id"`
	SecretHint string     `json:"secret_hint"`
	Enabled    bool       `json:"enabled"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

type KeyOwner struct {
	KeyID   int64
	UserID  int64
	Tier    Tier
	Dev     bool // 開發帳號，不算進後台統計
	OAuth   bool
	Expires time.Time
}

var idEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func dashed(s string, n int) string {
	var parts []string
	for len(s) > n {
		parts = append(parts, s[:n])
		s = s[n:]
	}
	return strings.Join(append(parts, s), "-")
}

func userHash(userID int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("dashboard.tw client user:%d", userID)))
	return hex.EncodeToString(sum[:4])
}

func newClientID(userID int64) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	random := strings.ToLower(idEncoding.EncodeToString(b))[:12]
	return userHash(userID) + "-" + dashed(random, 4), nil
}

func newClientSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return dashed(hex.EncodeToString(b), 8), nil
}

func secretHint(secret string) string {
	if len(secret) < 24 {
		return ""
	}
	return secret[:8] + "-************-" + secret[len(secret)-8:]
}

func hashOf(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

var taipei = time.FixedZone("Asia/Taipei", 8*60*60)

func (s *Store) taipeiDay() string {
	return s.now().In(taipei).Format(time.DateOnly)
}

func (s *Store) CreateClient(ctx context.Context, userID int64, name, ip string) (*APIClient, string, error) {
	clientID, err := newClientID(userID)
	if err != nil {
		return nil, "", err
	}
	secret, err := newClientSecret()
	if err != nil {
		return nil, "", err
	}
	c := APIClient{Name: name, ClientID: clientID, SecretHint: secretHint(secret), Enabled: true}
	err = db.AsUser(ctx, s.DB, userID, func(tx *sql.Tx) error {
		var dev bool
		if err := tx.QueryRowContext(ctx, `SELECT is_dev FROM app.users WHERE id = $1 FOR UPDATE`, userID).Scan(&dev); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM app.api_key WHERE revoked_at IS NULL AND kind = 'client'`).Scan(&n); err != nil {
			return err
		}
		if !dev && n >= MaxClients {
			return ErrTooManyClients
		}
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO app.api_key (user_id, name, prefix, key_hash, secret_hint, tier) VALUES ($1, $2, $3, $4, $5, 'free')
			RETURNING id, created_at`, userID, name, clientID, hashOf(secret), c.SecretHint).Scan(&c.ID, &c.CreatedAt); err != nil {
			return err
		}
		return audit(ctx, tx, &userID, "client.create", fmt.Sprintf("api_key:%d", c.ID), map[string]any{"client_id": clientID}, ip)
	})
	if err != nil {
		return nil, "", err
	}
	s.emit(ctx, Event{Kind: EventAPIKeyCreated, UserID: userID, IP: ip, Key: name, ClientID: clientID})
	return &c, secret, nil
}

func (s *Store) Clients(ctx context.Context, userID int64) ([]APIClient, error) {
	out := []APIClient{}
	err := db.AsUser(ctx, s.DB, userID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT id, name, prefix, coalesce(secret_hint, ''), disabled_at IS NULL, created_at, last_used_at
			  FROM app.api_key WHERE revoked_at IS NULL AND kind = 'client' ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c APIClient
			if err := rows.Scan(&c.ID, &c.Name, &c.ClientID, &c.SecretHint, &c.Enabled, &c.CreatedAt, &c.LastUsedAt); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) changeClient(ctx context.Context, userID, id int64, action, ip string, fn func(tx *sql.Tx) error) error {
	_, err := s.changeClientNamed(ctx, userID, id, action, ip, fn)
	return err
}

func (s *Store) changeClientNamed(ctx context.Context, userID, id int64, action, ip string, fn func(tx *sql.Tx) error) (name [2]string, err error) {
	err = db.AsUser(ctx, s.DB, userID, func(tx *sql.Tx) error {
		var found int64
		err := tx.QueryRowContext(ctx, `SELECT id, name, prefix FROM app.api_key WHERE id = $1 AND revoked_at IS NULL AND kind = 'client' FOR UPDATE`, id).
			Scan(&found, &name[0], &name[1])
		if errors.Is(err, sql.ErrNoRows) {
			return ErrClientNotFound
		}
		if err != nil {
			return err
		}
		if err := fn(tx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM app.api_token WHERE api_key_id = $1`, id); err != nil {
			return err
		}
		return audit(ctx, tx, &userID, action, fmt.Sprintf("api_key:%d", id), nil, ip)
	})
	return name, err
}

func (s *Store) RotateClientSecret(ctx context.Context, userID, id int64, ip string) (string, error) {
	secret, err := newClientSecret()
	if err != nil {
		return "", err
	}
	name, err := s.changeClientNamed(ctx, userID, id, "client.rotate", ip, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE app.api_key SET key_hash = $2, secret_hint = $3 WHERE id = $1`,
			id, hashOf(secret), secretHint(secret))
		return err
	})
	if err == nil {
		s.emit(ctx, Event{Kind: EventAPIKeyRotated, UserID: userID, IP: ip, Key: name[0], ClientID: name[1]})
	}
	return secret, err
}

func (s *Store) SetClientEnabled(ctx context.Context, userID, id int64, enabled bool, ip string) error {
	action := "client.disable"
	if enabled {
		action = "client.enable"
	}
	name, err := s.changeClientNamed(ctx, userID, id, action, ip, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			UPDATE app.api_key SET disabled_at = CASE WHEN $2 THEN NULL ELSE coalesce(disabled_at, now()) END
			 WHERE id = $1`, id, enabled)
		return err
	})
	if err == nil && !enabled {
		s.emit(ctx, Event{Kind: EventAPIKeyDisabled, UserID: userID, IP: ip, Key: name[0], ClientID: name[1]})
	}
	return err
}

func (s *Store) DeleteClient(ctx context.Context, userID, id int64, ip string) error {
	name, err := s.changeClientNamed(ctx, userID, id, "client.delete", ip, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE app.api_key SET revoked_at = now() WHERE id = $1`, id)
		return err
	})
	if err == nil {
		s.emit(ctx, Event{Kind: EventAPIKeyDeleted, UserID: userID, IP: ip, Key: name[0], ClientID: name[1]})
	}
	return err
}

func (s *Store) IssueToken(ctx context.Context, clientID, secret string) (string, time.Time, error) {
	if clientID == "" || secret == "" || len(clientID) > 100 || len(secret) > 200 {
		return "", time.Time{}, ErrInvalidClient
	}
	token, err := randomString(32)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := s.now().Add(TokenTTL)
	err = db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		var id int64
		var stored []byte
		err := tx.QueryRowContext(ctx, `
			SELECT k.id, k.key_hash
			  FROM app.api_key k JOIN app.users u ON u.id = k.user_id
			 WHERE k.prefix = $1 AND k.kind = 'client' AND k.revoked_at IS NULL AND k.disabled_at IS NULL AND u.status = 'active'`,
			clientID).Scan(&id, &stored)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidClient
		}
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare(stored, hashOf(secret)) != 1 {
			return ErrInvalidClient
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM app.api_token WHERE api_key_id = $1 AND expires_at < $2`, id, s.now()); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO app.api_token (token_hash, api_key_id, expires_at) VALUES ($1, $2, $3)`,
			hashOf(token), id, expires)
		return err
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (s *Store) LookupToken(ctx context.Context, token string) (*KeyOwner, error) {
	return s.lookupToken(ctx, token, false)
}

func (s *Store) LookupMCPToken(ctx context.Context, token string) (*KeyOwner, error) {
	return s.lookupToken(ctx, token, true)
}

func (s *Store) lookupToken(ctx context.Context, token string, oauth bool) (*KeyOwner, error) {
	if token == "" || len(token) > 100 {
		return nil, ErrNoKey
	}
	var o KeyOwner
	var tier string
	err := db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT k.id, k.user_id, `+tierSQL("k.user_id")+`, k.kind = 'oauth', t.expires_at
			  FROM app.api_token t
			  JOIN app.api_key k ON k.id = t.api_key_id
			  JOIN app.users u ON u.id = k.user_id
			 WHERE t.token_hash = $1 AND t.expires_at > $2 AND (k.kind = 'client' OR $3)
			   AND k.revoked_at IS NULL AND k.disabled_at IS NULL AND u.status = 'active'`,
			hashOf(token), s.now(), oauth).Scan(&o.KeyID, &o.UserID, &tier, &o.OAuth, &o.Expires)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoKey
	}
	if err != nil {
		return nil, err
	}
	t, ok := Tiers[tier]
	if !ok {
		t = Tiers[TierFree]
	}
	o.Tier = t
	o.Dev = tier == TierDev
	return &o, nil
}

type Usage struct {
	Requests int   `json:"requests"`
	Bytes    int64 `json:"bytes"`
}

func (s *Store) UsedToday(ctx context.Context, userID int64) (Usage, error) {
	var u Usage
	err := db.AsUser(ctx, s.DB, userID, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT coalesce(sum(requests), 0)::int, coalesce(sum(bytes), 0)::bigint
			  FROM app.api_usage_daily WHERE day = $1`, s.taipeiDay()).Scan(&u.Requests, &u.Bytes)
	})
	return u, err
}

type DayUsage struct {
	Day      string `json:"day"`
	Requests int    `json:"requests"`
	Bytes    int64  `json:"bytes"`
}

func (s *Store) APIKeyUsage(ctx context.Context, userID int64, days int) ([]DayUsage, error) {
	out := []DayUsage{}
	err := db.AsUser(ctx, s.DB, userID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT to_char(d, 'YYYY-MM-DD'), coalesce(sum(u.requests), 0)::int, coalesce(sum(u.bytes), 0)::bigint
			  FROM generate_series($1::date - ($2::int - 1), $1::date, interval '1 day') d
			  LEFT JOIN app.api_usage_daily u ON u.day = d::date
			 GROUP BY d ORDER BY d`, s.taipeiDay(), days)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var u DayUsage
			if err := rows.Scan(&u.Day, &u.Requests, &u.Bytes); err != nil {
				return err
			}
			out = append(out, u)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) UseAPIKey(ctx context.Context, o *KeyOwner) (used Usage, ok bool, err error) {
	day := s.taipeiDay()
	err = db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `SELECT 1 FROM app.users WHERE id = $1 FOR UPDATE`, o.UserID); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT coalesce(sum(u.requests), 0)::int, coalesce(sum(u.bytes), 0)::bigint
			  FROM app.api_usage_daily u JOIN app.api_key k ON k.id = u.api_key_id
			 WHERE k.user_id = $1 AND u.day = $2`, o.UserID, day).Scan(&used.Requests, &used.Bytes); err != nil {
			return err
		}
		if !o.Tier.Unlimited && (used.Requests >= o.Tier.PerDay || used.Bytes >= o.Tier.BytesPerDay) {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app.api_usage_daily AS u (api_key_id, day, requests) VALUES ($1, $2, 1)
			ON CONFLICT (api_key_id, day) DO UPDATE SET requests = u.requests + 1`, o.KeyID, day); err != nil {
			return err
		}
		used.Requests++
		ok = true
		_, err := tx.ExecContext(ctx, `UPDATE app.api_key SET last_used_at = now() WHERE id = $1`, o.KeyID)
		return err
	})
	return used, ok, err
}

func (s *Store) AddTraffic(ctx context.Context, o *KeyOwner, bytes int64) error {
	if bytes <= 0 {
		return nil
	}
	return db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO app.api_usage_daily AS u (api_key_id, day, bytes) VALUES ($1, $2, $3)
			ON CONFLICT (api_key_id, day) DO UPDATE SET bytes = u.bytes + EXCLUDED.bytes`, o.KeyID, s.taipeiDay(), bytes)
		return err
	})
}

func (s *Store) MarkUsageWarned(ctx context.Context, userID int64) (bool, error) {
	var n int64
	err := db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO app.api_usage_warning (user_id, day) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			userID, s.taipeiDay())
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	})
	return n == 1, err
}

func (s *Store) RejectAPIKey(ctx context.Context, o *KeyOwner) error {
	return db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO app.api_usage_daily AS u (api_key_id, day, requests, rejected) VALUES ($1, $2, 0, 1)
			ON CONFLICT (api_key_id, day) DO UPDATE SET rejected = u.rejected + 1`, o.KeyID, s.taipeiDay())
		return err
	})
}

func (s *Store) CountMCPCall(ctx context.Context, userID int64, tool string) error {
	return db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO app.mcp_usage_daily AS u (user_id, day, tool, calls) VALUES ($1, $2, left($3, 60), 1)
			ON CONFLICT (user_id, day, tool) DO UPDATE SET calls = u.calls + 1`, userID, s.taipeiDay(), tool)
		return err
	})
}

func (s *Store) LookupClient(ctx context.Context, clientID, secret string) (*KeyOwner, error) {
	if clientID == "" || secret == "" || len(clientID) > 100 || len(secret) > 200 {
		return nil, ErrInvalidClient
	}
	var o KeyOwner
	var stored []byte
	var tier string
	err := db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT k.id, k.user_id, k.key_hash, `+tierSQL("k.user_id")+`
			  FROM app.api_key k JOIN app.users u ON u.id = k.user_id
			 WHERE k.prefix = $1 AND k.kind = 'client' AND k.revoked_at IS NULL AND k.disabled_at IS NULL AND u.status = 'active'`,
			clientID).Scan(&o.KeyID, &o.UserID, &stored, &tier)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidClient
	}
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare(stored, hashOf(secret)) != 1 {
		return nil, ErrInvalidClient
	}
	t, ok := Tiers[tier]
	if !ok {
		t = Tiers[TierFree]
	}
	o.Tier = t
	o.Dev = tier == TierDev
	return &o, nil
}

func (s *Store) TouchKey(ctx context.Context, keyID int64) error {
	return db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			UPDATE app.api_key SET last_used_at = now()
			 WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '5 minutes')`, keyID)
		return err
	})
}

func (s *Store) MCPCallsToday(ctx context.Context, userID int64) (int, error) {
	var n int
	err := db.AsUser(ctx, s.DB, userID, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT COALESCE(sum(calls), 0)::int FROM app.mcp_usage_daily WHERE user_id = $1 AND day = $2`,
			userID, s.taipeiDay()).Scan(&n)
	})
	return n, err
}

// SetDev turns the dev flag of the account with this email on or off and returns the user id.
func (s *Store) SetDev(ctx context.Context, email string, on bool) (int64, error) {
	if s.Emails == nil {
		return 0, errors.New("EMAIL_KEY is not set")
	}
	var id int64
	err := db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
			UPDATE app.users SET is_dev = $2
			 WHERE id = (SELECT id FROM app.users WHERE email_lookup = $1 ORDER BY id LIMIT 1)
			RETURNING id`, s.Emails.Lookup(email), on).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("no account uses this email")
		}
		if err != nil {
			return err
		}
		return audit(ctx, tx, nil, "user.dev", fmt.Sprintf("user:%d", id), map[string]any{"on": on}, "")
	})
	return id, err
}
