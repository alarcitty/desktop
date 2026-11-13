package cache

import (
	"context"
	"sync"
	"time"
)

type entry[V any] struct {
	value     V
	expiresAt time.Time
}

type flight[V any] struct {
	done chan struct{}
	val  V
	err  error
}

// Cache stores values in memory with per-entry TTL and singleflight deduplication.
type Cache[K comparable, V any] struct {
	mu       sync.RWMutex
	items    map[K]entry[V]
	inflight map[K]*flight[V]
	now      func() time.Time
}

// New creates a new thread-safe TTL cache.
func New[K comparable, V any]() *Cache[K, V] {
	return &Cache[K, V]{
		items:    make(map[K]entry[V]),
		inflight: make(map[K]*flight[V]),
		now:      time.Now,
	}
}

// Get returns a cached value if present and not expired.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()

	if !ok {
		var zero V
		return zero, false
	}
	if c.now().After(item.expiresAt) {
		c.mu.Lock()
		if cur, still := c.items[key]; still && c.now().After(cur.expiresAt) {
			delete(c.items, key)
		}
		c.mu.Unlock()
		var zero V
		return zero, false
	}
	return item.value, true
}

// Set stores a value with the given TTL. Non-positive TTLs are ignored.
func (c *Cache[K, V]) Set(key K, val V, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	c.items[key] = entry[V]{
		value:     val,
		expiresAt: c.now().Add(ttl),
	}
	c.mu.Unlock()
}

// Delete removes a single entry from the cache.
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// Do returns a cached value or executes fn once for concurrent callers on the same key.
func (c *Cache[K, V]) Do(ctx context.Context, key K, ttl time.Duration, fn func(context.Context) (V, error)) (V, error) {
	if v, ok := c.Get(key); ok {
		return v, nil
	}

	c.mu.Lock()
	// Re-check under write lock in case another goroutine populated the entry.
	if item, ok := c.items[key]; ok && !c.now().After(item.expiresAt) {
		c.mu.Unlock()
		return item.value, nil
	}
	if fl, running := c.inflight[key]; running {
		c.mu.Unlock()
		select {
		case <-fl.done:
			return fl.val, fl.err
		case <-ctx.Done():
			var zero V
			return zero, ctx.Err()
		}
	}

	fl := &flight[V]{done: make(chan struct{})}
	c.inflight[key] = fl
	c.mu.Unlock()

	val, err := fn(ctx)
	if err == nil {
		c.Set(key, val, ttl)
	}

	c.mu.Lock()
	fl.val = val
	fl.err = err
	delete(c.inflight, key)
	close(fl.done)
	c.mu.Unlock()

	return val, err
}

// Prune removes all expired entries and returns how many were evicted.
func (c *Cache[K, V]) Prune() int {
	now := c.now()
	removed := 0
	c.mu.Lock()
	for k, item := range c.items {
		if now.After(item.expiresAt) {
			delete(c.items, k)
			removed++
		}
	}
	c.mu.Unlock()
	return removed
}
