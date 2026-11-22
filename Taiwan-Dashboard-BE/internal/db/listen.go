package db

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

type Listener struct {
	DatabaseURL string
	Channels    []string
	OnNotify    func(channel, payload string)
	OnConnect   func()
	Logger      *slog.Logger
	PingEvery   time.Duration
}

func (l *Listener) Run(ctx context.Context) {
	backoff := time.Second
	for {
		err := l.listen(ctx, func() { backoff = time.Second })
		if ctx.Err() != nil {
			return
		}
		l.Logger.Warn("notification listener disconnected", "error", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Minute)
	}
}

func (l *Listener) listen(ctx context.Context, connected func()) error {
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	conn, err := pgx.Connect(connectCtx, l.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())

	for _, ch := range l.Channels {
		if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{ch}.Sanitize()); err != nil {
			return err
		}
	}
	connected()
	if l.OnConnect != nil {
		l.OnConnect()
	}
	l.Logger.Info("listening for notifications", "channels", l.Channels)

	for {
		every := l.PingEvery
		if every <= 0 {
			every = time.Minute
		}
		waitCtx, cancel := context.WithTimeout(ctx, every)
		n, err := conn.WaitForNotification(waitCtx)
		cancel()
		if err != nil {
			if ctx.Err() == nil && waitCtx.Err() != nil {
				if err := conn.Ping(ctx); err != nil {
					return err
				}
				continue
			}
			return err
		}
		l.OnNotify(n.Channel, n.Payload)
	}
}
