package cache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestCacheLRUEvictionByMaxEntriesAndRecency(t *testing.T) {
	c := New[string, string](
		WithMaxEntries[string, string](100),
		WithMaxBytes[string, string](0),
	)

	// Insert 1200 entries (verifying protocol scale >1000 records cleanly enforces cap)
	for i := 0; i < 1200; i++ {
		if i == 1105 {
			// Touch key "k1100" so it moves to MRU before "k1101" is evicted
			if v, ok := c.Get("k1100"); !ok || v != "v1100" {
				t.Fatalf("expected k1100 hit before subsequent writes")
			}
		}
		c.Set(fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i), time.Hour)
	}

	st := c.Stats()
	if st.Entries != 100 {
		t.Fatalf("expected 100 entries capped by LRU, got %d", st.Entries)
	}
	if st.Evictions < 1100 {
		t.Fatalf("expected at least 1100 LRU evictions, got %d", st.Evictions)
	}

	// Oldest keys must be evicted; touched MRU window must still be present
	if _, ok := c.Get("k0"); ok {
		t.Fatalf("expected k0 to be evicted")
	}
	if _, ok := c.Get("k1099"); ok {
		t.Fatalf("expected k1099 to be evicted")
	}
	if v, ok := c.Get("k1199"); !ok || v != "v1199" {
		t.Fatalf("expected newest key k1199 to hit, got (%q, %v)", v, ok)
	}
}

func TestCacheLRUEvictionByMaxBytes(t *testing.T) {
	c := New[string, string](
		WithMaxEntries[string, string](0),
		WithMaxBytes[string, string](25),
	)

	c.Set("a", strings.Repeat("a", 10), time.Hour)
	c.Set("b", strings.Repeat("b", 10), time.Hour)

	// Promote "a" to MRU so "b" is evicted when inserting "c"
	if _, ok := c.Get("a"); !ok {
		t.Fatalf("expected 'a' to be present")
	}
	c.Set("c", strings.Repeat("c", 10), time.Hour)

	if _, ok := c.Get("b"); ok {
		t.Fatalf("expected LRU entry 'b' to be evicted by byte budget")
	}
	if _, ok := c.Get("a"); !ok {
		t.Fatalf("expected MRU entry 'a' to stay after byte eviction")
	}
	if _, ok := c.Get("c"); !ok {
		t.Fatalf("expected new entry 'c' to stay")
	}
	if st := c.Stats(); st.Bytes > 25 {
		t.Fatalf("expected L1 bytes <= 25, got %d", st.Bytes)
	}

	// Oversized single entry (> maxBytes) must not evict healthy cache entries
	c.Set("huge", strings.Repeat("x", 100), time.Hour)
	if _, ok := c.Get("huge"); ok {
		t.Fatalf("expected oversized entry not to fit in L1 budget")
	}
	if _, ok := c.Get("a"); !ok {
		t.Fatalf("expected existing entry 'a' to remain untouched after oversized Set")
	}
}

func TestCacheL2DiskPersistenceAndPresetTTLs(t *testing.T) {
	if TTLMetadata != 24*time.Hour || TTLSchedule != 15*time.Minute || TTLSearch != 5*time.Minute {
		t.Fatalf("unexpected TTL presets: meta=%v sched=%v search=%v", TTLMetadata, TTLSchedule, TTLSearch)
	}

	dir := t.TempDir()
	now := time.Unix(1_700_000_000, 0).UTC()
	clock := func() time.Time { return now }

	c1 := New[string, map[string]any](
		WithMaxEntries[string, map[string]any](10),
		WithDiskDir[string, map[string]any](dir),
		WithClock[string, map[string]any](clock),
	)

	c1.Set("meta:63098", map[string]any{"title": "Psyren"}, TTLMetadata)
	c1.Set("sched:now", map[string]any{"count": 4}, TTLSchedule)
	c1.Set("search:psyren/1?q=test", map[string]any{"hits": 2}, TTLSearch)

	// Recreate cache cleanly from empty L1 memory, advancing simulated time by 6 minutes:
	// search (5m) should be expired on disk, schedule (15m) and metadata (24h) must restore from L2.
	now = now.Add(6 * time.Minute)
	c2 := New[string, map[string]any](
		WithMaxEntries[string, map[string]any](10),
		WithDiskDir[string, map[string]any](dir),
		WithClock[string, map[string]any](clock),
	)

	if _, ok := c2.Get("search:psyren/1?q=test"); ok {
		t.Fatalf("expected 5m search entry to expire after 6m across restart")
	}

	sched, ok := c2.Get("sched:now")
	if !ok || sched["count"] != float64(4) {
		t.Fatalf("expected 15m schedule entry recovered from L2 disk, got (%+v, %v)", sched, ok)
	}

	meta, ok := c2.Get("meta:63098")
	if !ok || meta["title"] != "Psyren" {
		t.Fatalf("expected 24h metadata entry recovered from L2 disk, got (%+v, %v)", meta, ok)
	}

	// Second read of meta:63098 must hit L1 (promoted from L2) without hitting disk again
	beforeReads := c2.Stats().DiskReads
	if _, ok := c2.Get("meta:63098"); !ok {
		t.Fatalf("expected L1 hit after L2 promotion")
	}
	st := c2.Stats()
	if st.HitsL2 != 2 || st.HitsL1 != 1 || st.DiskReads != beforeReads {
		t.Fatalf("unexpected L1/L2 promotion stats: %+v (beforeDiskReads=%d)", st, beforeReads)
	}

	// Advance past 15m schedule TTL, before 24h metadata TTL
	now = now.Add(10 * time.Minute)
	c3 := New[string, map[string]any](
		WithDiskDir[string, map[string]any](dir),
		WithClock[string, map[string]any](clock),
	)
	if _, ok := c3.Get("sched:now"); ok {
		t.Fatalf("expected 15m schedule entry to expire after 16m total")
	}
	if _, ok := c3.Get("meta:63098"); !ok {
		t.Fatalf("expected 24h metadata entry to still be valid at 16m")
	}
}

func TestCacheDiskQuotaPruneDeleteAndCorruptRecovery(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1_700_000_000, 0).UTC()
	clock := func() time.Time { return now }

	c := New[string, string](
		WithMaxEntries[string, string](50),
		WithDiskDir[string, string](dir),
		WithDiskMaxBytes[string, string](600),
		WithClock[string, string](clock),
	)

	// Write several ~200-byte envelopes exceeding 600B disk quota
	for i := 0; i < 6; i++ {
		c.Set(fmt.Sprintf("item-%d", i), strings.Repeat("z", 120), TTLMetadata)
		// Nudge clock slightly and sleep 2ms so filesystem ModTime ordering is unambiguous
		time.Sleep(2 * time.Millisecond)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	var diskTotal int64
	for _, e := range entries {
		info, err := e.Info()
		if err == nil {
			diskTotal += info.Size()
		}
	}
	if diskTotal > 600 {
		t.Fatalf("expected disk cache size <= 600 bytes after quota enforcement, got %d", diskTotal)
	}

	// Delete removes both L1 and L2 disk file
	c.Set("del-me", "hello", time.Hour)
	diskFile := filepath.Join(dir, c.diskKeyHash("del-me")+".json")
	if _, err := os.Stat(diskFile); err != nil {
		t.Fatalf("expected disk file for del-me to exist: %v", err)
	}
	c.Delete("del-me")
	if _, err := os.Stat(diskFile); !os.IsNotExist(err) {
		t.Fatalf("expected disk file removed after Delete, err=%v", err)
	}

	// Corrupted JSON file on disk must miss gracefully and clean itself up
	corruptKey := "corrupt"
	corruptPath := filepath.Join(dir, c.diskKeyHash(corruptKey)+".json")
	if err := os.WriteFile(corruptPath, []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("failed to write corrupt test file: %v", err)
	}
	if _, ok := c.Get(corruptKey); ok {
		t.Fatalf("expected corrupt disk entry to miss")
	}
	if _, err := os.Stat(corruptPath); !os.IsNotExist(err) {
		t.Fatalf("expected corrupt disk file to be deleted, err=%v", err)
	}

	// Prune removes expired L1 + L2 entries
	c.Clear()
	c.Set("short", "1", time.Minute)
	c.Set("long", "2", time.Hour)
	now = now.Add(2 * time.Minute)
	if removed := c.Prune(); removed < 1 {
		t.Fatalf("expected Prune to remove expired entry, got %d", removed)
	}
	if _, ok := c.Get("short"); ok {
		t.Fatalf("expected short entry pruned")
	}
	if v, ok := c.Get("long"); !ok || v != "2" {
		t.Fatalf("expected long entry preserved, got (%q, %v)", v, ok)
	}
}

func TestCacheConcurrentRaceL1AndL2(t *testing.T) {
	dir := t.TempDir()
	c := New[string, int](
		WithMaxEntries[string, int](64),
		WithMaxBytes[string, int](4096),
		WithDiskDir[string, int](dir),
		WithDiskMaxBytes[string, int](32<<10),
	)

	const goroutines = 32
	const ops = 60
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("k-%d", i%20)
				switch (id + i) % 5 {
				case 0:
					c.Set(key, id*100+i, TTLSearch)
				case 1:
					c.Get(key)
				case 2:
					_, _ = c.Do(context.Background(), "sf-"+key, TTLSchedule, func(context.Context) (int, error) {
						return i, nil
					})
				case 3:
					if i%10 == 0 {
						c.Delete(key)
					} else {
						c.Get(key)
					}
				case 4:
					_ = c.Stats()
					if i%25 == 0 {
						c.Prune()
					}
				}
			}
		}(g)
	}
	wg.Wait()

	st := c.Stats()
	if st.Entries > 64 {
		t.Fatalf("expected L1 entries <= 64 under race stress, got %d", st.Entries)
	}
}

func BenchmarkCacheL1Hit(b *testing.B) {
	c := New[string, string](WithMaxEntries[string, string](1024))
	c.Set("anime:63098", "payload", time.Hour)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Get("anime:63098")
	}
}
