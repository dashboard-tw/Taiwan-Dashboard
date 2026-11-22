package tdx

import (
	"context"
	"strings"
	"testing"
	"time"

	"dashboard.tw/backend/internal/pipeline"
	"dashboard.tw/backend/internal/pipeline/pipelinetest"
)

func TestLimiter(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, pipeline.Taipei)
	l := NewLimiter(2, time.Minute)
	l.now = func() time.Time { return now }
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if l.Wait(ctx) != nil || l.Wait(ctx) != nil {
		t.Fatal("the first two calls go through")
	}
	if err := l.Wait(ctx); err == nil {
		t.Error("a third call within the minute must wait")
	}
	now = now.Add(time.Minute + 2*time.Second)
	if err := l.Wait(context.Background()); err != nil {
		t.Errorf("after the window: %v", err)
	}
	l.Block(time.Hour)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	if err := l.Wait(ctx2); err == nil {
		t.Error("a blocked limiter must wait")
	}
}

func TestClient(t *testing.T) {
	srv := pipelinetest.NewServer(t, map[string]string{
		"/token":    "testdata/token.json",
		"/api/v2/x": "testdata/ok.json",
	})
	env := srv.Env(nil, time.Now())
	c := &Client{ClientID: "id", ClientSecret: "secret", TokenURL: srv.URL + "/token", API: srv.URL + "/api/",
		Limiter: NewLimiter(100, 50*time.Millisecond)}
	body, err := c.Get(context.Background(), env, "v2/x", nil)
	if err != nil || len(body) == 0 {
		t.Fatalf("get: %q, %v", body, err)
	}
	if c.token != "test-token" {
		t.Errorf("token %q", c.token)
	}
	srv.Fail("/api/v2/x", 429)
	start := time.Now()
	if _, err := c.Get(context.Background(), env, "v2/x", nil); err == nil {
		t.Error("a second 429 must fail")
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Error("a 429 must wait for the window")
	}
	if _, err := (&Client{Limiter: NewLimiter(1, time.Second)}).Get(context.Background(), env, "v2/x", nil); err == nil {
		t.Error("missing credentials must fail")
	}
}

func TestTokenFailureWaitsBeforeRetrying(t *testing.T) {
	srv := pipelinetest.NewServer(t, map[string]string{
		"/token":    "testdata/token.json",
		"/api/v2/x": "testdata/ok.json",
	})
	now := time.Date(2026, 11, 21, 20, 0, 0, 0, pipeline.Taipei)
	c := &Client{ClientID: "id", ClientSecret: "secret", TokenURL: srv.URL + "/token", API: srv.URL + "/api/",
		Limiter: NewLimiter(100, 50*time.Millisecond)}

	srv.Fail("/token", 400)
	_, err := c.Get(context.Background(), srv.Env(nil, now), "v2/x", nil)
	if err == nil || !strings.Contains(err.Error(), "額度") {
		t.Fatalf("first failure: %v", err)
	}
	srv.Fail("/token", 0)
	_, err = c.Get(context.Background(), srv.Env(nil, now.Add(10*time.Minute)), "v2/x", nil)
	if err == nil || !strings.Contains(err.Error(), "20:30 後再試") {
		t.Fatalf("within the wait: %v", err)
	}
	if _, err := c.Get(context.Background(), srv.Env(nil, now.Add(31*time.Minute)), "v2/x", nil); err != nil {
		t.Fatalf("after the wait: %v", err)
	}
	if !c.failUntil.IsZero() || c.failDelay != 0 {
		t.Error("a success must clear the wait")
	}
}

func TestTokenWaitGrows(t *testing.T) {
	srv := pipelinetest.NewServer(t, map[string]string{"/token": "testdata/token.json"})
	srv.Fail("/token", 429)
	now := time.Date(2026, 11, 21, 20, 0, 0, 0, pipeline.Taipei)
	c := &Client{ClientID: "id", ClientSecret: "secret", TokenURL: srv.URL + "/token"}
	want := []time.Duration{30 * time.Minute, time.Hour, 2 * time.Hour, 4 * time.Hour, 6 * time.Hour, 6 * time.Hour}
	for i, w := range want {
		if _, err := c.accessToken(context.Background(), srv.Env(nil, now), false); err == nil {
			t.Fatal("a failing token endpoint must fail")
		}
		if c.failDelay != w {
			t.Errorf("failure %d: wait %v, want %v", i+1, c.failDelay, w)
		}
		now = c.failUntil
	}
}
