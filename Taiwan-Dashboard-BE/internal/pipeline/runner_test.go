package pipeline_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
	"dashboard.tw/backend/internal/testdb"
)

type fakePipeline struct {
	res pipeline.Result
	err error
}

func (f *fakePipeline) Dataset() pipeline.Dataset {
	return pipeline.Dataset{
		ID: "fake", Name: "測試", SourceAgency: "測試機關", SourceURL: "https://example.test",
		UpdateFreq: time.Hour, StaleAfter: 3 * time.Hour,
	}
}

func (f *fakePipeline) Run(context.Context, pipeline.Env) (pipeline.Result, error) {
	return f.res, f.err
}

func newRunner(t *testing.T) *pipeline.Runner {
	t.Helper()
	db := testdb.New(t)
	return &pipeline.Runner{
		DB: db, Fetch: &pipeline.Fetcher{}, Now: time.Now,
		Logger: slog.New(slog.DiscardHandler), Timeout: time.Minute,
	}
}

func datasetStatus(t *testing.T, r *pipeline.Runner) (status string, lastErr *string) {
	t.Helper()
	if err := r.DB.QueryRow(`SELECT status, last_error FROM app.dataset WHERE id = 'fake'`).Scan(&status, &lastErr); err != nil {
		t.Fatal(err)
	}
	return status, lastErr
}

func lastRunStatus(t *testing.T, r *pipeline.Runner) string {
	t.Helper()
	var s string
	if err := r.DB.QueryRow(`SELECT status FROM app.pipeline_run ORDER BY id DESC LIMIT 1`).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRunnerRecordsOutcomes(t *testing.T) {
	r := newRunner(t)
	ctx := context.Background()
	p := &fakePipeline{}
	if err := r.RegisterDatasets(ctx, []pipeline.Pipeline{p}); err != nil {
		t.Fatal(err)
	}
	if s, _ := datasetStatus(t, r); s != "pending" {
		t.Errorf("new dataset status = %s, want pending", s)
	}

	p.res = pipeline.Result{RowsFetched: 2, RowsInserted: 2, LastObservedAt: time.Now().Add(-10 * time.Minute)}
	if _, err := r.Run(ctx, p); err != nil {
		t.Fatal(err)
	}
	if s, _ := datasetStatus(t, r); s != "ok" || lastRunStatus(t, r) != "success" {
		t.Errorf("after fresh run: dataset %s, run %s", s, lastRunStatus(t, r))
	}

	p.res.Warnings = []string{"一筆資料對不到縣市"}
	if _, err := r.Run(ctx, p); err != nil {
		t.Fatal(err)
	}
	if lastRunStatus(t, r) != "partial" {
		t.Errorf("run with warnings = %s, want partial", lastRunStatus(t, r))
	}

	p.err = errors.New("HTTP 503")
	if _, err := r.Run(ctx, p); err == nil {
		t.Fatal("expected the run error")
	}
	if s, msg := datasetStatus(t, r); s != "error" || msg == nil || *msg != "HTTP 503" {
		t.Errorf("after failure: status %s, error %v", s, msg)
	}
	if lastRunStatus(t, r) != "failed" {
		t.Errorf("run = %s, want failed", lastRunStatus(t, r))
	}

	p.err = nil
	p.res = pipeline.Result{LastObservedAt: time.Now().Add(-5 * time.Hour)}
	if _, err := r.Run(ctx, p); err != nil {
		t.Fatal(err)
	}
	if s, msg := datasetStatus(t, r); s != "ok" || msg != nil {
		t.Errorf("after recovery: status %s, error %v", s, msg)
	}
}

func TestRunnerMarksOldDataStale(t *testing.T) {
	r := newRunner(t)
	ctx := context.Background()
	p := &fakePipeline{res: pipeline.Result{LastObservedAt: time.Now().Add(-5 * time.Hour)}}
	if err := r.RegisterDatasets(ctx, []pipeline.Pipeline{p}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, p); err != nil {
		t.Fatal(err)
	}
	if s, _ := datasetStatus(t, r); s != "stale" {
		t.Errorf("status = %s, want stale (data older than stale_after)", s)
	}
}

func TestMarkStale(t *testing.T) {
	r := newRunner(t)
	ctx := context.Background()
	p := &fakePipeline{res: pipeline.Result{LastObservedAt: time.Now()}}
	if err := r.RegisterDatasets(ctx, []pipeline.Pipeline{p}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DB.Exec(`UPDATE app.dataset SET last_observed_at = now() - interval '4 hours'`); err != nil {
		t.Fatal(err)
	}
	n, err := pipeline.MarkStale(ctx, r.DB)
	if err != nil || n != 1 {
		t.Fatalf("MarkStale = %d, %v", n, err)
	}
	if s, _ := datasetStatus(t, r); s != "stale" {
		t.Errorf("status = %s, want stale", s)
	}
}

func TestRunnerSkipsWhenLocked(t *testing.T) {
	r := newRunner(t)
	ctx := context.Background()
	p := &fakePipeline{}
	if err := r.RegisterDatasets(ctx, []pipeline.Pipeline{p}); err != nil {
		t.Fatal(err)
	}

	conn, err := r.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext('pipeline:fake'))`); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Run(ctx, p); !errors.Is(err, pipeline.ErrAlreadyRunning) {
		t.Fatalf("err = %v, want ErrAlreadyRunning", err)
	}
	if n := pipelinetest.Count(t, r.DB, `SELECT count(*) FROM app.pipeline_run`); n != 0 {
		t.Errorf("a skipped run must not be recorded, found %d", n)
	}
}

func TestRunnerNotifiesAfterNewRows(t *testing.T) {
	r := newRunner(t)
	ctx := context.Background()
	p := &fakePipeline{}
	if err := r.RegisterDatasets(ctx, []pipeline.Pipeline{p}); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, testdb.URL(t, r.DB))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "LISTEN "+pipeline.DatasetChannel); err != nil {
		t.Fatal(err)
	}
	wait := func() (string, bool) {
		waitCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		n, err := conn.WaitForNotification(waitCtx)
		if err != nil {
			return "", false
		}
		return n.Payload, true
	}

	p.res = pipeline.Result{RowsFetched: 3, RowsInserted: 0, LastObservedAt: time.Now()}
	if _, err := r.Run(ctx, p); err != nil {
		t.Fatal(err)
	}
	if payload, ok := wait(); ok {
		t.Errorf("a run without new rows notified %q", payload)
	}

	p.res.RowsInserted = 3
	if _, err := r.Run(ctx, p); err != nil {
		t.Fatal(err)
	}
	if payload, ok := wait(); !ok || payload != "fake" {
		t.Errorf("notification = %q, %v; want the dataset id", payload, ok)
	}
}

func TestReleaseOrphanRuns(t *testing.T) {
	r := newRunner(t)
	ctx := context.Background()
	p := &fakePipeline{}
	if err := r.RegisterDatasets(ctx, []pipeline.Pipeline{p}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DB.Exec(`
		INSERT INTO app.pipeline_run (dataset_id, started_at) VALUES
		  ('fake', now() - interval '20 minutes'), ('fake', now() - interval '1 minute')`); err != nil {
		t.Fatal(err)
	}

	conn, err := r.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext('pipeline:fake'))`); err != nil {
		t.Fatal(err)
	}
	if n, err := pipeline.ReleaseOrphanRuns(ctx, r.DB); err != nil || n != 0 {
		t.Fatalf("a run that is still going was released: %d %v", n, err)
	}
	conn.Close()

	n, err := pipeline.ReleaseOrphanRuns(ctx, r.DB)
	if err != nil || n != 1 {
		t.Fatalf("ReleaseOrphanRuns = %d, %v", n, err)
	}
	if c := pipelinetest.Count(t, r.DB, `SELECT count(*) FROM app.pipeline_run WHERE status = 'running'`); c != 1 {
		t.Errorf("the recent run should stay running, %d left", c)
	}
}
