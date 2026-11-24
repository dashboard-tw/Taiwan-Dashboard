package pipeline

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const DatasetChannel = "dataset_updated"

var ErrAlreadyRunning = errors.New("pipeline already running")

type Runner struct {
	DB       *sql.DB
	Fetch    *Fetcher
	Now      func() time.Time
	Logger   *slog.Logger
	Timeout  time.Duration // per run
	OnFinish func(ctx context.Context, d Dataset, runErr error)
}

func (r *Runner) RegisterDatasets(ctx context.Context, pipelines []Pipeline) error {
	for _, p := range pipelines {
		d := p.Dataset()
		_, err := r.DB.ExecContext(ctx, `
			INSERT INTO app.dataset (id, name, source_agency, source_url, license, update_freq, stale_after)
			VALUES ($1, $2, $3, $4, $5, $6::interval, $7::interval)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				source_agency = EXCLUDED.source_agency,
				source_url = EXCLUDED.source_url,
				license = EXCLUDED.license,
				update_freq = EXCLUDED.update_freq,
				stale_after = EXCLUDED.stale_after,
				updated_at = now()`,
			d.ID, d.Name, d.SourceAgency, d.SourceURL, cmp.Or(d.License, License),
			intervalString(d.UpdateFreq), intervalString(d.StaleAfter))
		if err != nil {
			return fmt.Errorf("register dataset %s: %w", d.ID, err)
		}
	}
	return nil
}

func (r *Runner) Run(ctx context.Context, p Pipeline) (Result, error) {
	d := p.Dataset()
	log := r.Logger.With("dataset", d.ID)

	conn, err := r.DB.Conn(ctx)
	if err != nil {
		return Result{}, err
	}
	defer conn.Close()

	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, "pipeline:"+d.ID).Scan(&locked); err != nil {
		return Result{}, fmt.Errorf("acquire lock: %w", err)
	}
	if !locked {
		log.Warn("skipped: already running elsewhere")
		return Result{}, ErrAlreadyRunning
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtext($1))`, "pipeline:"+d.ID)
	}()

	var runID int64
	if err := r.DB.QueryRowContext(ctx,
		`INSERT INTO app.pipeline_run (dataset_id) VALUES ($1) RETURNING id`, d.ID).Scan(&runID); err != nil {
		return Result{}, fmt.Errorf("record run start: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	started := r.Now()
	res, runErr := p.Run(runCtx, Env{DB: r.DB, Fetch: r.Fetch, Now: r.Now, Logger: log})
	elapsed := r.Now().Sub(started).Round(time.Millisecond)

	saveCtx, saveCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer saveCancel()
	if err := r.finish(saveCtx, d, runID, res, runErr); err != nil {
		return res, errors.Join(runErr, fmt.Errorf("record run result: %w", err))
	}
	if r.OnFinish != nil {
		r.OnFinish(saveCtx, d, runErr)
	}

	if runErr != nil {
		log.Error("run failed", "error", runErr, "elapsed", elapsed)
		return res, runErr
	}
	log.Info("run finished", "fetched", res.RowsFetched, "inserted", res.RowsInserted,
		"warnings", len(res.Warnings), "observed", res.LastObservedAt.In(Taipei).Format(time.DateTime), "elapsed", elapsed)
	return res, nil
}

func (r *Runner) finish(ctx context.Context, d Dataset, runID int64, res Result, runErr error) error {
	warnings := res.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	warningsJSON, err := json.Marshal(warnings)
	if err != nil {
		return err
	}

	status := "success"
	var errText *string
	switch {
	case runErr != nil:
		status = "failed"
		s := Truncate(runErr.Error(), 1000)
		errText = &s
	case len(warnings) > 0:
		status = "partial"
	}

	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		UPDATE app.pipeline_run
		   SET finished_at = now(), status = $2, rows_fetched = $3, rows_inserted = $4,
		       warnings = $5::jsonb, error = $6
		 WHERE id = $1`,
		runID, status, res.RowsFetched, res.RowsInserted, string(warningsJSON), errText); err != nil {
		return err
	}

	if runErr != nil {
		_, err = tx.ExecContext(ctx, `
			UPDATE app.dataset SET status = 'error', last_error = $2, updated_at = now() WHERE id = $1`,
			d.ID, *errText)
	} else {
		var observed *time.Time
		if !res.LastObservedAt.IsZero() {
			observed = &res.LastObservedAt
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE app.dataset
			   SET last_ingested_at = now(),
			       last_observed_at = GREATEST(last_observed_at, $2),
			       last_error = NULL,
			       status = CASE
			           WHEN GREATEST(last_observed_at, $2) IS NULL THEN 'pending'
			           WHEN GREATEST(last_observed_at, $2) < now() - stale_after THEN 'stale'
			           ELSE 'ok' END,
			       updated_at = now()
			 WHERE id = $1`,
			d.ID, observed)
		if err == nil && res.RowsInserted > 0 {
			_, err = tx.ExecContext(ctx, `SELECT pg_notify($1, $2)`, DatasetChannel, d.ID)
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func MarkStale(ctx context.Context, db *sql.DB) (int64, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE app.dataset
		   SET status = 'stale', updated_at = now()
		 WHERE status = 'ok' AND last_observed_at < now() - stale_after`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

const OrphanAfter = 10 * time.Minute

func ReleaseOrphanRuns(ctx context.Context, db *sql.DB) (int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, dataset_id FROM app.pipeline_run
		 WHERE status = 'running' AND started_at < now() - $1::interval`, intervalString(OrphanAfter))
	if err != nil {
		return 0, err
	}
	type run struct {
		id      int64
		dataset string
	}
	var runs []run
	for rows.Next() {
		var r run
		if err := rows.Scan(&r.id, &r.dataset); err != nil {
			rows.Close()
			return 0, err
		}
		runs = append(runs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(runs) == 0 {
		return 0, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	released := 0
	for _, r := range runs {
		var locked bool
		if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, "pipeline:"+r.dataset).Scan(&locked); err != nil {
			return released, err
		}
		if !locked {
			continue
		}
		_, err := conn.ExecContext(ctx, `
			UPDATE app.pipeline_run SET status = 'failed', finished_at = now(), error = '程式重新啟動時中斷'
			 WHERE id = $1 AND status = 'running'`, r.id)
		_, _ = conn.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(hashtext($1))`, "pipeline:"+r.dataset)
		if err != nil {
			return released, err
		}
		released++
	}
	return released, nil
}

func intervalString(d time.Duration) string {
	return fmt.Sprintf("%d seconds", int64(d/time.Second))
}
