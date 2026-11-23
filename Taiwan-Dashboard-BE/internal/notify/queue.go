package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"dashboard.tw/backend/internal/db"
	"dashboard.tw/backend/internal/hooks"
	"dashboard.tw/backend/internal/mail"
)

const (
	maxAttempts = 6
	mergeDelay  = 2 * time.Minute
	staleAfter  = 10 * time.Minute
)

var backoff = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour}

type job struct {
	id       int64
	template string
	userID   int64
	emailEnc []byte
	data     map[string]string
	mergeKey string
	delay    time.Duration
}

func (n *Notifier) enqueue(ctx context.Context, j job) error {
	data, err := json.Marshal(j.data)
	if err != nil {
		return err
	}
	var mergeKey any
	if j.mergeKey != "" {
		mergeKey = j.mergeKey
	}
	err = n.system(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
		INSERT INTO app.mail_queue (template, user_id, email_enc, data, merge_key, next_attempt_at)
		VALUES ($1, $2, $3, $4, $5, now() + $6 * interval '1 second')
		ON CONFLICT (merge_key) WHERE status = 'pending' AND merge_key IS NOT NULL DO UPDATE
		   SET data = app.mail_queue.data || jsonb_build_object(
		         'App', concat_ws('」、「', app.mail_queue.data->>'App', EXCLUDED.data->>'App'),
		         'Author', CASE WHEN position(EXCLUDED.data->>'Author' IN app.mail_queue.data->>'Author') > 0
		                        THEN app.mail_queue.data->>'Author'
		                        ELSE concat_ws('、', app.mail_queue.data->>'Author', EXCLUDED.data->>'Author') END)`,
			j.template, j.userID, j.emailEnc, data, mergeKey, j.delay.Seconds())
		return err
	})
	if err != nil {
		return fmt.Errorf("enqueue %s: %w", j.template, err)
	}
	select {
	case n.wake <- struct{}{}:
	default:
	}
	return nil
}

func (n *Notifier) queue(j job) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := n.enqueue(ctx, j); err != nil {
		slog.Error("mail queue", "error", err)
	}
}

func (n *Notifier) Start(ctx context.Context) {
	if !n.enabled() {
		return
	}
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		n.run(ctx)
	}()
	if p, ok := n.Sender.(pinger); ok {
		n.wg.Add(1)
		go func() {
			defer n.wg.Done()
			n.keepAlive(ctx, p)
		}()
	}
}

type pinger interface {
	Ping(ctx context.Context) error
}

const KeepAliveEvery = 30 * 24 * time.Hour

func (n *Notifier) keepAlive(ctx context.Context, p pinger) {
	wait := time.Minute
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = KeepAliveEvery
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := p.Ping(pctx)
		cancel()
		if err == nil {
			slog.Info("mail: keep-alive ok")
			continue
		}
		if ctx.Err() != nil {
			return
		}
		slog.Error("mail: keep-alive failed", "error", err)
		n.Hooks.Emit(ctx, hooks.EventMailFailed, hooks.Message{
			Title: "Brevo 金鑰無法使用", Level: hooks.LevelError, URL: n.Site + "/admin/mail",
			Text: cut(err.Error(), 300),
		})
		wait = 6 * time.Hour
	}
}

func (n *Notifier) run(ctx context.Context) {
	interval := n.Interval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	capped := false
	for {
		if err := n.release(ctx); err != nil && ctx.Err() == nil {
			slog.Error("mail queue: release stale", "error", err)
		}
		over, err := n.overLimit(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Error("mail queue: daily count", "error", err)
		}
		if over != capped {
			capped = over
			if over {
				slog.Warn("mail queue: daily limit reached, waiting until tomorrow", "limit", n.DailyLimit)
				n.Hooks.Emit(ctx, hooks.EventMailFailed, hooks.Message{
					Title: "今天的寄信額度用完了", Level: hooks.LevelWarn, URL: n.Site + "/admin/mail",
					Text: fmt.Sprintf("已經寄了 %d 封，剩下的信明天再寄。", n.DailyLimit),
				})
			}
		}
		worked := false
		if !over && err == nil {
			worked, err = n.sendNext(ctx)
			if err != nil && ctx.Err() == nil {
				slog.Error("mail queue", "error", err)
			}
		}
		wait := interval
		if !worked {
			wait = 30 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-n.wake:
			if !worked {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		case <-time.After(wait):
		}
	}
}

func (n *Notifier) release(ctx context.Context) error {
	return n.exec(ctx, `
		UPDATE app.mail_queue SET status = 'pending', locked_at = NULL
		 WHERE status = 'sending' AND locked_at < now() - $1 * interval '1 second'`, staleAfter.Seconds())
}

func (n *Notifier) system(ctx context.Context, fn func(*sql.Tx) error) error {
	return db.AsSystem(ctx, n.Store.DB, fn)
}

func (n *Notifier) exec(ctx context.Context, q string, args ...any) error {
	return n.system(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, args...)
		return err
	})
}

func (n *Notifier) overLimit(ctx context.Context) (bool, error) {
	if n.DailyLimit <= 0 {
		return false, nil
	}
	sent, err := n.SentToday(ctx)
	return sent >= n.DailyLimit, err
}

func (n *Notifier) SentToday(ctx context.Context) (int, error) {
	now := time.Now().In(taipei)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, taipei)
	var c int
	err := n.system(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT count(*) FROM app.mail_queue WHERE status = 'sent' AND sent_at >= $1`, midnight).Scan(&c)
	})
	return c, err
}

func (n *Notifier) sendNext(ctx context.Context) (bool, error) {
	var (
		j        job
		raw      []byte
		attempts int
	)
	err := n.system(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
		UPDATE app.mail_queue SET status = 'sending', locked_at = now(), attempts = attempts + 1
		 WHERE id = (SELECT id FROM app.mail_queue
		              WHERE status = 'pending' AND next_attempt_at <= now()
		              ORDER BY next_attempt_at, id LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING id, template, user_id, email_enc, data, attempts`).
			Scan(&j.id, &j.template, &j.userID, &j.emailEnc, &raw, &attempts)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(raw, &j.data); err != nil {
		return true, n.finish(ctx, j.id, "failed", "資料格式錯誤："+err.Error())
	}
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	status, reason := n.deliver(sendCtx, j)
	if status == "retry" {
		if attempts >= maxAttempts {
			n.Hooks.Emit(ctx, hooks.EventMailFailed, hooks.Message{
				Title: "信寄不出去", Level: hooks.LevelError, URL: n.Site + "/admin/mail",
				Text:   cut(reason, 300),
				Fields: []hooks.Field{{Name: "信件", Value: n.templateName(ctx, j.template)}, {Name: "嘗試", Value: fmt.Sprintf("%d 次", attempts)}},
			})
			return true, n.finish(ctx, j.id, "failed", reason)
		}
		err := n.exec(ctx, `
			UPDATE app.mail_queue SET status = 'pending', locked_at = NULL, last_error = left($2, 500),
			       next_attempt_at = now() + $3 * interval '1 second'
			 WHERE id = $1`, j.id, reason, backoff[min(attempts, len(backoff))-1].Seconds())
		return true, err
	}
	return true, n.finish(ctx, j.id, status, reason)
}

func (n *Notifier) finish(ctx context.Context, id int64, status, reason string) error {
	return n.exec(ctx, `
		UPDATE app.mail_queue
		   SET status = $2, last_error = left($3, 500), locked_at = NULL, email_enc = NULL,
		       sent_at = CASE WHEN $2 = 'sent' THEN now() END
		 WHERE id = $1`, id, status, reason)
}

func (n *Notifier) deliver(ctx context.Context, j job) (status, reason string) {
	var email string
	var err error
	if j.emailEnc != nil {
		if n.Store.Emails == nil {
			return "skipped", "沒有設定 EMAIL_KEY，無法解開信箱"
		}
		email, err = n.Store.Emails.Open(j.userID, j.emailEnc)
	} else {
		var name string
		email, name, err = n.userInfo(ctx, j.userID)
		if errors.Is(err, sql.ErrNoRows) {
			return "skipped", "帳號已刪除"
		}
		j.data["Name"] = name
	}
	if err != nil {
		return "retry", "查詢收件人：" + err.Error()
	}
	if email == "" {
		return "skipped", "帳號沒有信箱"
	}
	if why, err := n.suppressed(ctx, email); err != nil {
		return "retry", "查詢退信名單：" + err.Error()
	} else if why != "" {
		return "skipped", "這個信箱曾經" + suppressLabel[why] + "，不再寄送"
	}
	j.data["Site"] = n.Site
	subject, body, err := Render(ctx, n.Store.DB, j.template, j.data)
	if errors.Is(err, ErrDisabled) {
		return "skipped", "這封信已停用"
	}
	if err != nil {
		return "retry", err.Error()
	}
	if err := n.Sender.Send(ctx, mail.Message{To: email, Subject: subject, Text: body, Tag: j.template, ID: fmt.Sprintf("mail_queue:%d", j.id)}); err != nil {
		return "retry", err.Error()
	}
	if n.Sandbox {
		return "sent", "沙盒模式，Brevo 沒有實際寄出"
	}
	return "sent", ""
}

var suppressLabel = map[string]string{"hard_bounce": "退信", "spam": "被檢舉為垃圾信", "invalid_email": "被判定為無效信箱", "blocked": "被 Brevo 封鎖"}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func (n *Notifier) templateName(ctx context.Context, key string) string {
	var name string
	if err := n.Store.DB.QueryRowContext(ctx, `SELECT name FROM app.mail_template WHERE key = $1`, key).Scan(&name); err != nil {
		return key
	}
	return name
}
