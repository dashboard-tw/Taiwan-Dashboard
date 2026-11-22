package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentMissLoadsOnce(t *testing.T) {
	c := New[int](10)
	var loads atomic.Int32
	release := make(chan struct{})
	load := func(context.Context) (int, error) {
		loads.Add(1)
		<-release
		return 42, nil
	}

	var wg sync.WaitGroup
	results := make([]int, 1000)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, _, err := c.Get(context.Background(), "k", nil, time.Minute, load)
			if err != nil {
				t.Error(err)
			}
			results[i] = v
		}(i)
	}
	time.Sleep(50 * time.Millisecond) // let every goroutine reach the cache
	close(release)
	wg.Wait()

	if n := loads.Load(); n != 1 {
		t.Errorf("1000 concurrent requests made %d loads, want 1", n)
	}
	for _, v := range results {
		if v != 42 {
			t.Fatalf("got %d, want 42", v)
		}
	}
}

func TestInvalidateRefreshesTaggedEntries(t *testing.T) {
	c := New[int](10)
	n := 0
	load := func(context.Context) (int, error) { n++; return n, nil }
	get := func(key string, tags ...string) int {
		v, _, _ := c.Get(context.Background(), key, tags, time.Hour, load)
		return v
	}
	if get("aqi", "aqi_obs") != 1 || get("power", "power_gen") != 2 {
		t.Fatal("initial loads")
	}
	if get("aqi", "aqi_obs") != 1 {
		t.Error("cached value should be served")
	}
	if c.Invalidate("aqi_obs") != 1 {
		t.Error("one entry carries the aqi_obs tag")
	}
	if get("aqi", "aqi_obs") != 3 {
		t.Error("invalidated entry should reload")
	}
	if get("power", "power_gen") != 2 {
		t.Error("other entries stay cached")
	}
	c.InvalidateAll()
	if get("power", "power_gen") != 4 {
		t.Error("InvalidateAll should refresh every entry")
	}
}

func TestExpiry(t *testing.T) {
	c := New[int](10)
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	n := 0
	load := func(context.Context) (int, error) { n++; return n, nil }

	c.Get(context.Background(), "k", nil, time.Minute, load)
	now = now.Add(59 * time.Second)
	if v, _, _ := c.Get(context.Background(), "k", nil, time.Minute, load); v != 1 {
		t.Error("value should still be fresh")
	}
	now = now.Add(2 * time.Second)
	if v, _, _ := c.Get(context.Background(), "k", nil, time.Minute, load); v != 2 {
		t.Error("expired value should reload")
	}
}

func TestServesStaleWhenLoadFails(t *testing.T) {
	c := New[string](10)
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	fail := false
	var loads int
	load := func(context.Context) (string, error) {
		loads++
		if fail {
			return "", errors.New("database down")
		}
		return "fresh", nil
	}

	fail = true
	if _, _, err := c.Get(context.Background(), "k", []string{"d"}, time.Minute, load); err == nil {
		t.Fatal("first load failure must be an error")
	}

	fail = false
	c.Get(context.Background(), "k", []string{"d"}, time.Minute, load)
	fail = true
	c.Invalidate("d")
	v, info, err := c.Get(context.Background(), "k", []string{"d"}, time.Minute, load)
	if err != nil || v != "fresh" || !info.Stale {
		t.Fatalf("got %q stale=%v err=%v, want the old value marked stale", v, info.Stale, err)
	}

	before := loads
	c.Get(context.Background(), "k", []string{"d"}, time.Minute, load)
	if loads != before {
		t.Error("should not retry within the back-off window")
	}
	now = now.Add(retryAfterError + time.Second)
	fail = false
	v, info, _ = c.Get(context.Background(), "k", []string{"d"}, time.Minute, load)
	if v != "fresh" || info.Stale || loads != before+1 {
		t.Errorf("after recovery got %q stale=%v loads=%d", v, info.Stale, loads-before)
	}
}

func TestInvalidateDuringLoad(t *testing.T) {
	c := New[int](10)
	started := make(chan struct{})
	release := make(chan struct{})
	n := 0
	load := func(context.Context) (int, error) {
		n++
		if n == 1 {
			close(started)
			<-release
		}
		return n, nil
	}
	done := make(chan struct{})
	go func() {
		c.Get(context.Background(), "k", []string{"d"}, time.Hour, load)
		close(done)
	}()
	<-started
	c.Invalidate("d")
	close(release)
	<-done

	if v, _, _ := c.Get(context.Background(), "k", []string{"d"}, time.Hour, load); v != 2 {
		t.Errorf("got %d, want a reload after the overlapping invalidation", v)
	}
}

func TestEvictsOldest(t *testing.T) {
	c := New[int](2)
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { now = now.Add(time.Second); return now }
	load := func(context.Context) (int, error) { return 1, nil }
	for _, k := range []string{"a", "b", "c"} {
		c.Get(context.Background(), k, nil, time.Hour, load)
	}
	if c.Len() != 2 {
		t.Errorf("len = %d, want 2", c.Len())
	}
	if _, ok := c.entries["a"]; ok {
		t.Error("the oldest entry should be evicted")
	}
}
