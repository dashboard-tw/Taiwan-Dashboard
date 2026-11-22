package db_test

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"dashboard.tw/backend/internal/db"
	"dashboard.tw/backend/internal/testdb"
)

func TestListenerSurvivesIdlePings(t *testing.T) {
	database := testdb.NewEmpty(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var connects atomic.Int32
	connected := make(chan struct{}, 10)
	got := make(chan string, 10)
	l := &db.Listener{
		DatabaseURL: testdb.URL(t, database),
		Channels:    []string{"test_channel"},
		OnNotify:    func(_, payload string) { got <- payload },
		OnConnect:   func() { connects.Add(1); connected <- struct{}{} },
		Logger:      slog.New(slog.DiscardHandler),
		PingEvery:   50 * time.Millisecond,
	}
	go l.Run(ctx)
	<-connected

	time.Sleep(300 * time.Millisecond) // several idle pings
	if _, err := database.Exec(`SELECT pg_notify('test_channel', 'hello')`); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-got:
		if p != "hello" {
			t.Errorf("payload = %q", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("notification not received after idle pings")
	}
	if n := connects.Load(); n != 1 {
		t.Errorf("connected %d times, want 1 (pings must not reconnect)", n)
	}
}
