package cache

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const retryAfterError = 10 * time.Second

type Cache[V any] struct {
	mu         sync.Mutex
	entries    map[string]*entry[V]
	group      singleflight.Group
	maxEntries int
	now        func() time.Time
	gen        uint64
}

type entry[V any] struct {
	value      V
	loadedAt   time.Time
	expires    time.Time
	dirty      bool // invalidated: refresh before serving, but keep for errors
	retryAfter time.Time
	tags       []string
}

func New[V any](maxEntries int) *Cache[V] {
	return &Cache[V]{entries: map[string]*entry[V]{}, maxEntries: maxEntries, now: time.Now}
}

type Info struct {
	Stale    bool
	LoadedAt time.Time
}

func (c *Cache[V]) Get(ctx context.Context, key string, tags []string, ttl time.Duration,
	load func(context.Context) (V, error)) (V, Info, error) {
	now := c.now()
	c.mu.Lock()
	e := c.entries[key]
	if e != nil && !e.dirty && now.Before(e.expires) {
		v, info := e.value, Info{LoadedAt: e.loadedAt}
		c.mu.Unlock()
		return v, info, nil
	}
	if e != nil && now.Before(e.retryAfter) {
		v, info := e.value, Info{Stale: true, LoadedAt: e.loadedAt}
		c.mu.Unlock()
		return v, info, nil
	}
	gen := c.gen
	c.mu.Unlock()

	loadCtx := context.WithoutCancel(ctx)
	res, err, _ := c.group.Do(key, func() (any, error) {
		v, err := load(loadCtx)
		if err != nil {
			return nil, err
		}
		return v, nil
	})
	if err == nil {
		v := res.(V)
		return v, Info{LoadedAt: c.store(key, v, tags, ttl, gen)}, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.entries[key]; e != nil {
		e.retryAfter = c.now().Add(retryAfterError)
		return e.value, Info{Stale: true, LoadedAt: e.loadedAt}, nil
	}
	var zero V
	return zero, Info{}, err
}

func (c *Cache[V]) store(key string, v V, tags []string, ttl time.Duration, gen uint64) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.maxEntries {
		c.evictOldest()
	}
	c.entries[key] = &entry[V]{value: v, loadedAt: now, expires: now.Add(ttl), tags: tags, dirty: gen != c.gen}
	return now
}

func (c *Cache[V]) evictOldest() {
	var oldestKey string
	var oldest time.Time
	for k, e := range c.entries {
		if oldestKey == "" || e.loadedAt.Before(oldest) {
			oldestKey, oldest = k, e.loadedAt
		}
	}
	delete(c.entries, oldestKey)
}

func (c *Cache[V]) Invalidate(tag string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	n := 0
	for _, e := range c.entries {
		for _, t := range e.tags {
			if t == tag {
				e.dirty, e.retryAfter = true, time.Time{}
				n++
				break
			}
		}
	}
	return n
}

func (c *Cache[V]) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	for _, e := range c.entries {
		e.dirty, e.retryAfter = true, time.Time{}
	}
}

func (c *Cache[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
