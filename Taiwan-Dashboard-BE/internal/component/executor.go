package component

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

const DefaultTimeout = 5 * time.Second

var ErrTimeout = errors.New("component query timed out")

type Executor struct {
	DB      *sql.DB
	Timeout time.Duration
}

func (e *Executor) Query(ctx context.Context, q Compiled, p Params) (Rows, error) {
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout+time.Second)
	defer cancel()

	tx, err := e.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Rows{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", timeout.Milliseconds())); err != nil {
		return Rows{}, err
	}
	if !p.AsOf.IsZero() {
		if _, err := tx.ExecContext(ctx, `SELECT set_config($1, $2, true)`, AsOfSetting, p.AsOf.Format(time.RFC3339Nano)); err != nil {
			return Rows{}, err
		}
	}

	return queryAll(ctx, tx, q, p)
}

func queryAll(ctx context.Context, tx *sql.Tx, q Compiled, p Params) (Rows, error) {
	rows, err := tx.QueryContext(ctx, q.SQL, q.Args(p)...)
	if err != nil {
		return Rows{}, classify(err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return Rows{}, err
	}
	out := Rows{Columns: cols}
	if types, err := rows.ColumnTypes(); err == nil {
		for _, t := range types {
			out.Types = append(out.Types, t.DatabaseTypeName())
		}
	}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return Rows{}, err
		}
		out.Values = append(out.Values, vals)
	}
	if err := rows.Err(); err != nil {
		return Rows{}, classify(err)
	}
	return out, nil
}

func VerifyReader(ctx context.Context, db *sql.DB) error {
	var user string
	var appUsage, superuser, canWrite bool
	err := db.QueryRowContext(ctx, `
		SELECT current_user,
		       has_schema_privilege('app', 'USAGE'),
		       (SELECT rolsuper FROM pg_roles WHERE rolname = current_user),
		       EXISTS (SELECT 1 FROM information_schema.table_privileges
		                WHERE grantee = current_user AND privilege_type IN ('INSERT', 'UPDATE', 'DELETE', 'TRUNCATE'))`).
		Scan(&user, &appUsage, &superuser, &canWrite)
	if err != nil {
		return fmt.Errorf("check reader role: %w", err)
	}
	switch {
	case superuser:
		return fmt.Errorf("%w: connected as superuser %s", ErrUnsafeReader, user)
	case appUsage:
		return fmt.Errorf("%w: role %s can read the app schema", ErrUnsafeReader, user)
	case canWrite:
		return fmt.Errorf("%w: role %s can modify tables", ErrUnsafeReader, user)
	}
	return nil
}

var ErrUnsafeReader = errors.New("component queries need the dashboard_reader role")

type Verified struct {
	Exec *Executor

	mu sync.Mutex
	ok bool
}

func (v *Verified) Query(ctx context.Context, q Compiled, p Params) (Rows, error) {
	v.mu.Lock()
	if !v.ok {
		if err := VerifyReader(ctx, v.Exec.DB); err != nil {
			v.mu.Unlock()
			return Rows{}, err
		}
		v.ok = true
	}
	v.mu.Unlock()
	return v.Exec.Query(ctx, q, p)
}

func classify(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}
	var coded interface{ SQLState() string }
	if errors.As(err, &coded) && coded.SQLState() == "57014" { // query_canceled
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}
	return err
}
