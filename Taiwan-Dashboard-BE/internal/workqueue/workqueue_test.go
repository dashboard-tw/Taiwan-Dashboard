package workqueue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueueLimitsConcurrency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := New(2, 100)
	q.Start(ctx)
	var running, peak atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := q.Do(ctx, func(context.Context) error {
				n := running.Add(1)
				for {
					p := peak.Load()
					if n <= p || peak.CompareAndSwap(p, n) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				running.Add(-1)
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if peak.Load() > 2 {
		t.Errorf("peak = %d, want at most 2 at once", peak.Load())
	}
}

func TestQueueFullAndCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := New(1, 1)
	block := make(chan struct{})
	started := make(chan struct{})
	go func() { _ = q.Do(ctx, func(context.Context) error { close(started); <-block; return nil }) }()
	q.Start(ctx)
	<-started

	reqCtx, reqCancel := context.WithCancel(ctx)
	ran := atomic.Bool{}
	waitErr := make(chan error, 1)
	go func() {
		waitErr <- q.Do(reqCtx, func(context.Context) error { ran.Store(true); return nil })
	}()
	for q.Waiting() == 0 {
		time.Sleep(time.Millisecond)
	}
	if err := q.Do(ctx, func(context.Context) error { return nil }); !errors.Is(err, ErrBusy) {
		t.Errorf("full queue: err = %v, want ErrBusy", err)
	}
	reqCancel()
	if err := <-waitErr; !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled request: err = %v", err)
	}
	close(block)
	time.Sleep(20 * time.Millisecond)
	if ran.Load() {
		t.Error("a request cancelled while queued still ran")
	}
}
