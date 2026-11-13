package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCacheGetSetExpiry(t *testing.T) {
	now := time.Unix(1000, 0)
	c := New[string, int]()
	c.now = func() time.Time { return now }

	c.Set("k", 42, 10*time.Second)
	v, ok := c.Get("k")
	if !ok || v != 42 {
		t.Fatalf("expected (42, true), got (%d, %v)", v, ok)
	}

	now = now.Add(11 * time.Second)
	_, ok = c.Get("k")
	if ok {
		t.Fatalf("expected expired key to miss")
	}
}

func TestCacheDoSingleflight(t *testing.T) {
	c := New[string, string]()
	var calls atomic.Int32

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers)
	results := make([]string, workers)

	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			val, err := c.Do(context.Background(), "same-key", time.Minute, func(context.Context) (string, error) {
				calls.Add(1)
				time.Sleep(25 * time.Millisecond)
				return "ok", nil
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			results[idx] = val
		}(i)
	}
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected singleflight to invoke fn once, got %d", got)
	}
	for i, r := range results {
		if r != "ok" {
			t.Fatalf("worker %d got %q", i, r)
		}
	}
}
