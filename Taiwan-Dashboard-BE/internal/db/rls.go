package db

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
)

func AsUser(ctx context.Context, db *sql.DB, userID int64, fn func(*sql.Tx) error) error {
	if userID <= 0 {
		return errors.New("AsUser needs a user id")
	}
	return inTx(ctx, db, `SELECT set_config('app.user_id', $1, true)`, strconv.FormatInt(userID, 10), fn)
}

func AsSystem(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	return inTx(ctx, db, `SELECT set_config('app.rls_bypass', $1, true)`, "on", fn)
}

func inTx(ctx context.Context, db *sql.DB, setup, value string, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // a no-op after Commit
	if _, err := tx.ExecContext(ctx, setup, value); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
