package db_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"dashboard.tw/backend/internal/db"
	"dashboard.tw/backend/internal/testdb"
)

var rlsTables = []string{
	"session", "api_key", "api_usage_daily",
}

func seedTwoUsers(t *testing.T, admin *sql.DB) {
	t.Helper()
	if _, err := admin.Exec(`
		INSERT INTO app.users (id, display_name) VALUES (1, 'A'), (2, 'B');
		INSERT INTO app.session (id_hash, user_id, expires_at)
		VALUES (decode(repeat('aa', 32), 'hex'), 1, now() + interval '1 day'),
		       (decode(repeat('bb', 32), 'hex'), 2, now() + interval '1 day');
		INSERT INTO app.api_key (id, user_id, name, prefix, key_hash)
		VALUES (100, 1, 'a', 'dtw_a', decode(repeat('01', 32), 'hex')),
		       (200, 2, 'b', 'dtw_b', decode(repeat('02', 32), 'hex'));
		INSERT INTO app.api_usage_daily (api_key_id, day, requests) VALUES (100, current_date, 1), (200, current_date, 1)`); err != nil {
		t.Fatal(err)
	}
}

func counts(t *testing.T, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, table := range rlsTables {
		var n int
		if err := q.QueryRowContext(context.Background(), `SELECT count(*) FROM app.`+table).Scan(&n); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		out[table] = n
	}
	return out
}

func TestRowLevelSecurity(t *testing.T) {
	admin := testdb.New(t)
	seedTwoUsers(t, admin)
	app := testdb.App(t, admin)
	app.SetMaxOpenConns(1) // the same connection every time, to catch settings leaking
	ctx := context.Background()

	for table, n := range counts(t, app) {
		if n != 0 {
			t.Errorf("no user: %s shows %d rows, want 0", table, n)
		}
	}

	err := db.AsUser(ctx, app, 1, func(tx *sql.Tx) error {
		for table, n := range counts(t, tx) {
			if n != 1 {
				t.Errorf("user 1: %s shows %d rows, want 1", table, n)
			}
		}
		for _, q := range []string{
			`UPDATE app.api_key SET revoked_at = now() WHERE id = 200`,
			`DELETE FROM app.api_key WHERE user_id = 2`,
			`UPDATE app.session SET revoked_at = now() WHERE user_id = 2`,
			`UPDATE app.api_usage_daily SET requests = 0 WHERE api_key_id = 200`,
		} {
			r, err := tx.ExecContext(ctx, q)
			if err != nil {
				return err
			}
			if n, _ := r.RowsAffected(); n != 0 {
				t.Errorf("user 1 changed %d of user 2's rows: %s", n, q)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO app.api_key (user_id, name, prefix, key_hash) VALUES (2, 'planted', 'dtw_p', decode(repeat('03', 32), 'hex'))`,
		`INSERT INTO app.session (id_hash, user_id, expires_at) VALUES (decode(repeat('cc', 32), 'hex'), 2, now() + interval '1 day')`,
		`INSERT INTO app.api_usage_daily (api_key_id, day, requests) VALUES (200, current_date - 1, 1)`,
	} {
		err := db.AsUser(ctx, app, 1, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, q)
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "row-level security") {
			t.Errorf("user 1 wrote a row for user 2 (%s): err = %v", q, err)
		}
	}

	for table, n := range counts(t, app) {
		if n != 0 {
			t.Errorf("after AsUser: %s shows %d rows, want 0", table, n)
		}
	}

	err = db.AsSystem(ctx, app, func(tx *sql.Tx) error {
		for table, n := range counts(t, tx) {
			if n != 2 {
				t.Errorf("system: %s shows %d rows, want 2", table, n)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var untouched int
	if err := admin.QueryRow(`SELECT count(*) FROM app.api_key WHERE id = 200 AND revoked_at IS NULL`).Scan(&untouched); err != nil || untouched != 1 {
		t.Errorf("user 2's API key was changed (count %d, err %v)", untouched, err)
	}

	if err := db.AsUser(ctx, app, 0, func(*sql.Tx) error { return nil }); err == nil {
		t.Error("AsUser without a user id should fail")
	}
}

func TestUserTablesHaveForcedRLS(t *testing.T) {
	admin := testdb.New(t)
	notLimited := map[string]string{
		"users":         "the account itself; /me reads it by the session's user id",
		"user_identity": "looked up by provider and subject at sign-in, before the user is known",
		"identity_link": "a pending link, looked up by the hash of a secret token at sign-in, before the user is known",
		"audit_log":     "written for every user, read by administrators",
	}
	rows, err := admin.Query(`
		SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity
		  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = 'app' AND c.relkind = 'r'
		   AND (c.relname = 'users' OR EXISTS (
		       SELECT 1 FROM information_schema.columns
		        WHERE table_schema = 'app' AND table_name = c.relname
		          AND column_name IN ('user_id', 'api_key_id')))`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var name string
		var enabled, forced bool
		if err := rows.Scan(&name, &enabled, &forced); err != nil {
			t.Fatal(err)
		}
		seen++
		if _, ok := notLimited[name]; ok {
			continue
		}
		if !enabled || !forced {
			t.Errorf("app.%s holds user data but RLS enabled=%v forced=%v", name, enabled, forced)
		}
	}
	if seen < len(rlsTables) {
		t.Errorf("found %d user tables, want at least %d", seen, len(rlsTables))
	}
}
