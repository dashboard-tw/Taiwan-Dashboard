package workqueue

import (
	"context"
	"errors"
	"sync/atomic"
)

var ErrBusy = errors.New("work queue is full")

type task struct {
	ctx  context.Context
	fn   func(context.Context) error
	done chan error
}

type Queue struct {
	tasks   chan task
	workers int
	waiting atomic.Int64
}

func New(workers, capacity int) *Queue {
	return &Queue{tasks: make(chan task, capacity), workers: max(workers, 1)}
}

func (q *Queue) Start(ctx context.Context) {
	for range q.workers {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case t := <-q.tasks:
					q.waiting.Add(-1)
					if err := t.ctx.Err(); err != nil {
						t.done <- err
						continue
					}
					t.done <- t.fn(t.ctx)
				}
			}
		}()
	}
}

func (q *Queue) Do(ctx context.Context, fn func(context.Context) error) error {
	t := task{ctx: ctx, fn: fn, done: make(chan error, 1)}
	select {
	case q.tasks <- t:
		q.waiting.Add(1)
	default:
		return ErrBusy
	}
	select {
	case err := <-t.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *Queue) Waiting() int { return int(q.waiting.Load()) }
