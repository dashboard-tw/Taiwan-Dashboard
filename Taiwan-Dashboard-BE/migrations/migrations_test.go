package migrations_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/pressly/goose/v3"

	"dashboard.tw/backend/internal/testdb"
	"dashboard.tw/backend/migrations"
)

func TestUpDownUp(t *testing.T) {
	db := testdb.NewEmpty(t)
	ctx := context.Background()

	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("up: %v", err)
	}
	assertSchemas(t, db, 3)
	assertCount(t, db, `SELECT count(*) FROM ref.county`, 22)

	for {
		_, err := migrations.Down(ctx, db)
		if errors.Is(err, goose.ErrNoNextVersion) {
			break
		}
		if err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	assertSchemas(t, db, 0)

	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("second up: %v", err)
	}
	assertSchemas(t, db, 3)
}

func TestCountySeed(t *testing.T) {
	db := testdb.New(t)
	assertCount(t, db, `SELECT count(*) FROM ref.county WHERE NOT ST_IsValid(boundary)`, 0)
	assertCount(t, db, `SELECT count(*) FROM ref.county WHERE NOT ST_Contains(boundary, center)`, 0)
	assertCount(t, db, `SELECT count(DISTINCT sort_order) FROM ref.county`, 22)
}

func assertSchemas(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	assertCount(t, db, `SELECT count(*) FROM information_schema.schemata WHERE schema_name IN ('ref', 'data', 'app')`, want)
}

func assertCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	if got != want {
		t.Fatalf("%s = %d, want %d", query, got, want)
	}
}
