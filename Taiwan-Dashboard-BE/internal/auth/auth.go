package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"dashboard.tw/backend/internal/db"
)

const SessionTTL = 7 * 24 * time.Hour

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

var ErrNoSession = errors.New("no valid session")

type User struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

func (u *User) IsAdmin() bool { return u != nil && u.Role == RoleAdmin }

type Store struct {
	DB     *sql.DB
	Now    func() time.Time
	Emails *EmailVault
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func (s *Store) CreateSession(ctx context.Context, userID int64, ip, userAgent string) (string, time.Time, error) {
	token, err := NewToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := s.now().Add(SessionTTL)
	err = db.AsUser(ctx, s.DB, userID, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app.session (id_hash, user_id, expires_at, ip, user_agent, last_seen_at)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			HashToken(token), userID, expires, ipOrNil(ip), truncate(userAgent, 300), s.now()); err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		_, err := tx.ExecContext(ctx, `UPDATE app.users SET last_login_at = now() WHERE id = $1`, userID)
		return err
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (s *Store) Lookup(ctx context.Context, token string) (*User, error) {
	if token == "" || len(token) > 100 {
		return nil, ErrNoSession
	}
	var u User
	hash, now := HashToken(token), s.now()
	err := db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		var lastSeen sql.NullTime
		if err := tx.QueryRowContext(ctx, `
			SELECT u.id, u.display_name, u.role, s.last_seen_at
			  FROM app.session s
			  JOIN app.users u ON u.id = s.user_id
			 WHERE s.id_hash = $1
			   AND s.revoked_at IS NULL
			   AND s.expires_at > $2
			   AND u.status = 'active'`, hash, now).Scan(&u.ID, &u.DisplayName, &u.Role, &lastSeen); err != nil {
			return err
		}
		if lastSeen.Valid && now.Sub(lastSeen.Time) < LastSeenInterval {
			return nil
		}
		_, err := tx.ExecContext(ctx, `UPDATE app.session SET last_seen_at = $2 WHERE id_hash = $1`, hash, now)
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) Revoke(ctx context.Context, token string) error {
	return db.AsSystem(ctx, s.DB, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			UPDATE app.session SET revoked_at = now() WHERE id_hash = $1 AND revoked_at IS NULL`, HashToken(token))
		return err
	})
}

func (s *Store) FindIdentity(ctx context.Context, provider, subject string) (*User, error) {
	var u User
	err := s.DB.QueryRowContext(ctx, `
		SELECT u.id, u.display_name, u.role
		  FROM app.user_identity i JOIN app.users u ON u.id = i.user_id
		 WHERE i.provider = $1 AND i.subject = $2 AND u.status = 'active'`,
		provider, subject).Scan(&u.ID, &u.DisplayName, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSession
	}
	return &u, err
}

const DevSubject = "dev-admin"

const DevUserSubject = "dev-user"

func (s *Store) EnsureDevUser(ctx context.Context) (*User, error) {
	if u, err := s.FindIdentity(ctx, "dev", DevUserSubject); !errors.Is(err, ErrNoSession) {
		return u, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	u := User{DisplayName: "測試使用者", Role: RoleUser}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO app.users (display_name, role) VALUES ($1, 'user') RETURNING id`, u.DisplayName).Scan(&u.ID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO app.user_identity (user_id, provider, subject) VALUES ($1, 'dev', $2)`, u.ID, DevUserSubject); err != nil {
		return nil, err
	}
	return &u, tx.Commit()
}

func (s *Store) CreateAdmin(ctx context.Context, name string) (*User, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM app.user_identity WHERE provider = 'dev' AND subject = $1)`,
		DevSubject).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("the development administrator already exists")
	}
	u := User{DisplayName: name, Role: RoleAdmin}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO app.users (display_name, role) VALUES ($1, 'admin') RETURNING id`, name).Scan(&u.ID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO app.user_identity (user_id, provider, subject) VALUES ($1, 'dev', $2)`, u.ID, DevSubject); err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, &u.ID, "admin.create", fmt.Sprintf("user:%d", u.ID), map[string]any{"via": "cli"}, ""); err != nil {
		return nil, err
	}
	return &u, tx.Commit()
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func AuditTx(ctx context.Context, tx *sql.Tx, userID *int64, action, target string, detail map[string]any, ip string) error {
	return audit(ctx, tx, userID, action, target, detail, ip)
}

func (s *Store) Audit(ctx context.Context, userID *int64, action, target string, detail map[string]any, ip string) error {
	return audit(ctx, s.DB, userID, action, target, detail, ip)
}

func audit(ctx context.Context, db execer, userID *int64, action, target string, detail map[string]any, ip string) error {
	if detail == nil {
		detail = map[string]any{}
	}
	b, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO app.audit_log (user_id, action, target, detail, ip) VALUES ($1, $2, $3, $4::jsonb, $5)`,
		userID, action, target, string(b), ipOrNil(ip))
	return err
}

func ipOrNil(ip string) any {
	if a, err := netip.ParseAddr(ip); err == nil {
		return a.String()
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}
