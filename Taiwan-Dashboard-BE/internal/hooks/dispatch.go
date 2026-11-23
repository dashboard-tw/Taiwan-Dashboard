package hooks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const maxAttempts = 5

var backoff = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}

type Dispatcher struct {
	DB       *sql.DB
	Sealer   Sealer
	Client   *http.Client
	Interval time.Duration
	wake     chan struct{}
	wg       sync.WaitGroup
}

func NewDispatcher(db *sql.DB, s Sealer) *Dispatcher {
	return &Dispatcher{DB: db, Sealer: s, Client: SafeClient(), Interval: time.Second, wake: make(chan struct{}, 1)}
}

func (d *Dispatcher) Emit(ctx context.Context, event string, m Message) {
	if d == nil {
		return
	}
	if err := Emit(ctx, d.DB, event, m); err != nil {
		slog.Error("notify channels", "event", event, "error", err)
		return
	}
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (d *Dispatcher) Start(ctx context.Context) {
	if d == nil {
		return
	}
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		for {
			if _, err := d.DB.ExecContext(ctx, `
				UPDATE app.notify_outbox SET status = 'pending', locked_at = NULL
				 WHERE status = 'sending' AND locked_at < now() - interval '10 minutes'`); err != nil && ctx.Err() == nil {
				slog.Error("notify channels: release stale", "error", err)
			}
			worked, err := d.sendNext(ctx)
			if err != nil && ctx.Err() == nil {
				slog.Error("notify channels", "error", err)
			}
			wait := d.Interval
			if !worked {
				wait = 15 * time.Second
			}
			select {
			case <-ctx.Done():
				return
			case <-d.wake:
			case <-time.After(wait):
			}
		}
	}()
}

func (d *Dispatcher) Wait() {
	if d != nil {
		d.wg.Wait()
	}
}

func (d *Dispatcher) sendNext(ctx context.Context) (bool, error) {
	var (
		id, channelID int64
		event, kind   string
		raw, enc      []byte
		enabled       bool
		attempts      int
	)
	err := d.DB.QueryRowContext(ctx, `
		WITH next AS (
		  UPDATE app.notify_outbox SET status = 'sending', locked_at = now(), attempts = attempts + 1
		   WHERE id = (SELECT id FROM app.notify_outbox
		                WHERE status = 'pending' AND next_attempt_at <= now()
		                ORDER BY next_attempt_at, id LIMIT 1 FOR UPDATE SKIP LOCKED)
		  RETURNING id, channel_id, event, message, attempts)
		SELECT n.id, n.channel_id, n.event, n.message, n.attempts, c.kind, c.target_enc, c.enabled
		  FROM next n JOIN app.notify_channel c ON c.id = n.channel_id`).
		Scan(&id, &channelID, &event, &raw, &attempts, &kind, &enc, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !enabled {
		return true, d.finish(ctx, id, "failed", "頻道已停用")
	}
	var m Message
	if err := json.Unmarshal(raw, &m); err != nil {
		return true, d.finish(ctx, id, "failed", "訊息格式錯誤")
	}
	t, err := OpenTarget(d.Sealer, enc)
	if err != nil {
		return true, d.finish(ctx, id, "failed", "無法解開頻道設定："+err.Error())
	}
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	err = Send(sendCtx, d.Client, kind, t, event, m)
	if err == nil {
		return true, d.finish(ctx, id, "sent", "")
	}
	var perm *PermanentError
	if errors.As(err, &perm) || attempts >= maxAttempts {
		return true, d.finish(ctx, id, "failed", err.Error())
	}
	wait := backoff[min(attempts, len(backoff))-1]
	var retry *RetryError
	if errors.As(err, &retry) && retry.After > 0 {
		wait = retry.After + time.Second
	}
	_, err2 := d.DB.ExecContext(ctx, `
		UPDATE app.notify_outbox SET status = 'pending', locked_at = NULL, last_error = left($2, 500),
		       next_attempt_at = now() + $3 * interval '1 second'
		 WHERE id = $1`, id, err.Error(), wait.Seconds())
	return true, err2
}

func (d *Dispatcher) finish(ctx context.Context, id int64, status, reason string) error {
	_, err := d.DB.ExecContext(ctx, `
		UPDATE app.notify_outbox SET status = $2, last_error = left($3, 500), locked_at = NULL,
		       sent_at = CASE WHEN $2 = 'sent' THEN now() END
		 WHERE id = $1`, id, status, reason)
	return err
}
