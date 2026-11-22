package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"
)

type Job struct {
	Name       string
	Schedule   Schedule
	RunAtStart bool
	Run        func(ctx context.Context) error
	LastRun    func(ctx context.Context) (time.Time, error)
}

func PipelineJob(r *Runner, p Pipeline, s Schedule) Job {
	return Job{
		Name:       p.Dataset().ID,
		Schedule:   s,
		RunAtStart: true,
		Run: func(ctx context.Context) error {
			_, _ = r.Run(ctx, p)
			return nil
		},
		LastRun: func(ctx context.Context) (time.Time, error) {
			var t sql.NullTime
			err := r.DB.QueryRowContext(ctx, `
				SELECT max(started_at) FROM app.pipeline_run
				 WHERE dataset_id = $1 AND status IN ('success', 'partial')`, p.Dataset().ID).Scan(&t)
			return t.Time, err
		},
	}
}

type Worker struct {
	Jobs   []Job
	Now    func() time.Time
	Logger *slog.Logger
}

func (w *Worker) Start(ctx context.Context) {
	for _, j := range w.Jobs {
		if ctx.Err() != nil {
			return
		}
		if j.RunAtStart && !w.ranSinceLastSlot(ctx, j) {
			w.runOnce(ctx, j)
		}
	}

	var wg sync.WaitGroup
	for _, j := range w.Jobs {
		wg.Add(1)
		go func(j Job) {
			defer wg.Done()
			w.loop(ctx, j)
		}(j)
	}
	wg.Wait()
}

func (w *Worker) loop(ctx context.Context, j Job) {
	for {
		next := j.Schedule.Next(w.Now())
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			w.runOnce(ctx, j)
		}
	}
}

// 上一個排程時間之後已經成功跑過，重啟時就不必再抓一次
func (w *Worker) ranSinceLastSlot(ctx context.Context, j Job) bool {
	if j.LastRun == nil {
		return false
	}
	last, err := j.LastRun(ctx)
	if err != nil || last.IsZero() {
		return false
	}
	now := w.Now()
	prev := j.Schedule.Next(now).Add(-j.Schedule.Every)
	return !last.Before(prev) || now.Sub(last) < j.Schedule.Every
}

func (w *Worker) runOnce(ctx context.Context, j Job) {
	defer func() {
		if p := recover(); p != nil {
			w.Logger.Error("job panicked", "job", j.Name, "panic", p)
		}
	}()
	if err := j.Run(ctx); err != nil && !errors.Is(err, ErrAlreadyRunning) && ctx.Err() == nil {
		w.Logger.Error("job failed", "job", j.Name, "error", err)
	}
}
