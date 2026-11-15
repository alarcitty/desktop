package cache

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// TTLMetadata is the canonical TTL for relatively static anime metadata (24h).
	TTLMetadata = 24 * time.Hour
	// TTLSchedule is the canonical TTL for broadcast schedules and release lists (15m).
	TTLSchedule = 15 * time.Minute
	// TTLSearch is the canonical TTL for search and filtered catalog queries (5m).
	TTLSearch = 5 * time.Minute

	// DefaultL1MaxEntries caps in-memory LRU entries per cache instance when unspecified.
	DefaultL1MaxEntries = 1024
	// DefaultL1MaxBytes caps in-memory LRU payload bytes per cache instance (32 MiB).
	DefaultL1MaxBytes int64 = 32 << 20
	// DefaultDiskMaxBytes caps L2 on-disk JSON storage per cache directory (128 MiB).
	DefaultDiskMaxBytes int64 = 128 << 20
)

// Stats reports live L1 memory usage and cumulative L1/L2 hit/miss/eviction metrics.
type Stats struct {
	Entries    int
	Bytes      int64
	HitsL1     int64
	HitsL2     int64
	Misses     int64
	Evictions  int64
	DiskReads  int64
	DiskWrites int64
}

// Option customizes a Cache created with New.
type Option[K comparable, V any] func(*Cache[K, V])

// WithMaxEntries sets the L1 LRU entry limit (<= 0 disables count-based eviction).
func WithMaxEntries[K comparable, V any](n int) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.maxEntries = n
	}
}

// WithMaxBytes sets the L1 LRU byte limit (<= 0 disables byte-based eviction).
func WithMaxBytes[K comparable, V any](bytes int64) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.maxBytes = bytes
	}
}

// WithDiskDir enables persistent file-based L2 caching inside dir (JSON files, no database).
func WithDiskDir[K comparable, V any](dir string) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.diskDir = strings.TrimSpace(dir)
	}
}

// WithDiskMaxBytes sets the L2 disk cache byte budget (<= 0 disables disk size eviction).
func WithDiskMaxBytes[K comparable, V any](bytes int64) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.diskMaxBytes = bytes
	}
}

// WithSizeFunc supplies a custom byte-size estimator for L1 LRU accounting.
func WithSizeFunc[K comparable, V any](fn func(V) int64) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.sizeFunc = fn
	}
}

// WithKeyCodec overrides how keys of type K are formatted before hashing for L2 filenames.
func WithKeyCodec[K comparable, V any](fn func(K) string) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.keyCodec = fn
	}
}

// WithClock injects a deterministic clock function (primarily for unit tests).
func WithClock[K comparable, V any](now func() time.Time) Option[K, V] {
	return func(c *Cache[K, V]) {
		if now != nil {
			c.now = now
		}
	}
}

// DefaultDiskCacheDir resolves a per-user cache directory for a named cache namespace.
func DefaultDiskCacheDir(namespace string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(base) == "" {
		base, err = os.UserConfigDir()
		if err != nil || strings.TrimSpace(base) == "" {
			base = os.TempDir()
		}
	}
	cleanNS := strings.Trim(strings.ReplaceAll(namespace, "..", "_"), `/\ `)
	if cleanNS == "" {
		cleanNS = "default"
	}
	return filepath.Join(base, "mila", "cache", cleanNS), nil
}

type lruNode[K comparable, V any] struct {
	key       K
	value     V
	expiresAt time.Time
	sizeBytes int64
}

type diskEntry[V any] struct {
	Key       string    `json:"key"`
	ExpiresAt time.Time `json:"expires_at"`
	SavedAt   time.Time `json:"saved_at"`
	Value     V         `json:"value"`
}

type flight[V any] struct {
	done chan struct{}
	val  V
	err  error
}

// Cache stores values in a thread-safe L1 memory LRU with optional L2 disk persistence,
// per-entry TTL expiration, and singleflight request deduplication.
type Cache[K comparable, V any] struct {
	mu          sync.Mutex
	items       map[K]*list.Element
	order       *list.List
	inflight    map[K]*flight[V]
	curBytes    int64
	maxEntries  int
	maxBytes    int64
	mutationSeq uint64

	diskMu          sync.RWMutex
	diskDir         string
	diskMaxBytes    int64
	diskApproxBytes int64
	diskWriteCount  uint64

	sizeFunc func(V) int64
	keyCodec func(K) string
	now      func() time.Time

	hitsL1     atomic.Int64
	hitsL2     atomic.Int64
	misses     atomic.Int64
	evictions  atomic.Int64
	diskReads  atomic.Int64
	diskWrites atomic.Int64
}

// New creates a new thread-safe L1 LRU + optional L2 disk TTL cache.
func New[K comparable, V any](opts ...Option[K, V]) *Cache[K, V] {
	c := &Cache[K, V]{
		items:        make(map[K]*list.Element),
		order:        list.New(),
		inflight:     make(map[K]*flight[V]),
		maxEntries:   DefaultL1MaxEntries,
		maxBytes:     DefaultL1MaxBytes,
		diskMaxBytes: DefaultDiskMaxBytes,
		now:          time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	if c.diskDir != "" {
		if err := os.MkdirAll(c.diskDir, 0o755); err != nil {
			// Fall back to L1-only if disk directory cannot be created.
			c.diskDir = ""
		}
	}
	return c
}

// Get returns a cached value from L1 LRU (or L2 disk cache if enabled) when present and not expired.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	now := c.now()

	c.mu.Lock()
	if elem, ok := c.items[key]; ok {
		node := elem.Value.(*lruNode[K, V])
		if !now.After(node.expiresAt) {
			c.order.MoveToFront(elem)
			val := node.value
			c.mu.Unlock()
			c.hitsL1.Add(1)
			return val, true
		}
		c.removeElemLocked(elem)
	}
	seqBeforeDisk := c.mutationSeq
	diskEnabled := c.diskDir != ""
	c.mu.Unlock()

	if !diskEnabled {
		c.misses.Add(1)
		var zero V
		return zero, false
	}

	val, expiresAt, sizeBytes, ok := c.readDisk(key, now)
	if !ok {
		c.misses.Add(1)
		var zero V
		return zero, false
	}

	c.mu.Lock()
	// Re-check if another writer updated or deleted this key while we performed disk I/O.
	if c.mutationSeq != seqBeforeDisk {
		if elem, exists := c.items[key]; exists {
			node := elem.Value.(*lruNode[K, V])
			if !c.now().After(node.expiresAt) {
				c.order.MoveToFront(elem)
				curVal := node.value
				c.mu.Unlock()
				c.hitsL1.Add(1)
				return curVal, true
			}
			c.removeElemLocked(elem)
		}
		c.mu.Unlock()
		c.hitsL2.Add(1)
		return val, true
	}

	c.insertL1Locked(key, val, expiresAt, sizeBytes)
	c.mu.Unlock()

	c.hitsL2.Add(1)
	return val, true
}

// Set stores a value in L1 LRU and (if configured) persists it to L2 disk cache.
// Non-positive TTLs are ignored.
func (c *Cache[K, V]) Set(key K, val V, ttl time.Duration) {
	if ttl <= 0 {
		return
	}

	now := c.now()
	expiresAt := now.Add(ttl)

	var diskRaw []byte
	if c.diskDir != "" {
		raw, err := json.Marshal(diskEntry[V]{
			Key:       c.formatKey(key),
			ExpiresAt: expiresAt.UTC(),
			SavedAt:   now.UTC(),
			Value:     val,
		})
		if err == nil {
			diskRaw = raw
		}
	}

	sizeBytes := c.measureBytes(val, diskRaw)

	c.mu.Lock()
	c.insertL1Locked(key, val, expiresAt, sizeBytes)
	c.mu.Unlock()

	if len(diskRaw) > 0 {
		c.writeDisk(key, diskRaw)
	}
}

// Delete removes a single entry from both L1 LRU and L2 disk cache.
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	c.mutationSeq++
	if elem, ok := c.items[key]; ok {
		c.removeElemLocked(elem)
	}
	diskEnabled := c.diskDir != ""
	c.mu.Unlock()

	if diskEnabled {
		c.deleteDisk(key)
	}
}

// Do returns a cached value (L1/L2) or executes fn once for concurrent callers on the same key.
func (c *Cache[K, V]) Do(ctx context.Context, key K, ttl time.Duration, fn func(context.Context) (V, error)) (V, error) {
	if v, ok := c.Get(key); ok {
		return v, nil
	}

	c.mu.Lock()
	if elem, ok := c.items[key]; ok {
		node := elem.Value.(*lruNode[K, V])
		if !c.now().After(node.expiresAt) {
			c.order.MoveToFront(elem)
			v := node.value
			c.mu.Unlock()
			c.hitsL1.Add(1)
			return v, nil
		}
		c.removeElemLocked(elem)
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

// Prune removes all expired entries from L1 memory and L2 disk cache and returns how many were removed.
func (c *Cache[K, V]) Prune() int {
	now := c.now()
	removed := 0

	c.mu.Lock()
	c.mutationSeq++
	var expiredKeys []K
	for elem := c.order.Back(); elem != nil; {
		prev := elem.Prev()
		node := elem.Value.(*lruNode[K, V])
		if now.After(node.expiresAt) {
			expiredKeys = append(expiredKeys, node.key)
			c.removeElemLocked(elem)
			removed++
		}
		elem = prev
	}
	diskEnabled := c.diskDir != ""
	c.mu.Unlock()

	if !diskEnabled {
		return removed
	}

	removedHashes := make(map[string]struct{}, len(expiredKeys))
	for _, k := range expiredKeys {
		h := c.diskKeyHash(k)
		removedHashes[h] = struct{}{}
	}

	removed += c.pruneDisk(now, removedHashes)
	return removed
}

// Clear removes all entries from L1 memory and L2 disk cache.
func (c *Cache[K, V]) Clear() {
	c.mu.Lock()
	c.mutationSeq++
	c.items = make(map[K]*list.Element)
	c.order.Init()
	c.curBytes = 0
	diskDir := c.diskDir
	c.mu.Unlock()

	if diskDir == "" {
		return
	}

	c.diskMu.Lock()
	defer c.diskMu.Unlock()
	c.diskApproxBytes = 0
	entries, err := os.ReadDir(diskDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".json") || strings.HasPrefix(name, ".tmp-") {
			_ = os.Remove(filepath.Join(diskDir, name))
		}
	}
}

// Stats returns a consistent snapshot of cache size and operational counters.
func (c *Cache[K, V]) Stats() Stats {
	c.mu.Lock()
	entries := len(c.items)
	bytes := c.curBytes
	c.mu.Unlock()

	return Stats{
		Entries:    entries,
		Bytes:      bytes,
		HitsL1:     c.hitsL1.Load(),
		HitsL2:     c.hitsL2.Load(),
		Misses:     c.misses.Load(),
		Evictions:  c.evictions.Load(),
		DiskReads:  c.diskReads.Load(),
		DiskWrites: c.diskWrites.Load(),
	}
}

func (c *Cache[K, V]) insertL1Locked(key K, val V, expiresAt time.Time, sizeBytes int64) {
	c.mutationSeq++
	if elem, ok := c.items[key]; ok {
		if c.maxBytes > 0 && sizeBytes > c.maxBytes {
			c.removeElemLocked(elem)
			return
		}
		node := elem.Value.(*lruNode[K, V])
		c.curBytes -= node.sizeBytes
		node.value = val
		node.expiresAt = expiresAt
		node.sizeBytes = sizeBytes
		c.curBytes += sizeBytes
		c.order.MoveToFront(elem)
		c.evictL1Locked()
		return
	}

	if c.maxBytes > 0 && sizeBytes > c.maxBytes {
		// Single item exceeds entire L1 byte budget; do not evict healthy cache for it.
		return
	}

	node := &lruNode[K, V]{
		key:       key,
		value:     val,
		expiresAt: expiresAt,
		sizeBytes: sizeBytes,
	}
	elem := c.order.PushFront(node)
	c.items[key] = elem
	c.curBytes += sizeBytes
	c.evictL1Locked()
}

func (c *Cache[K, V]) evictL1Locked() {
	now := c.now()
	// First sweep expired tail items, then strictly enforce maxEntries / maxBytes LRU limits.
	for (c.maxEntries > 0 && c.order.Len() > c.maxEntries) || (c.maxBytes > 0 && c.curBytes > c.maxBytes) {
		victim := c.findExpiredTailLocked(now)
		expired := victim != nil
		if victim == nil {
			victim = c.order.Back()
		}
		if victim == nil {
			break
		}
		c.removeElemLocked(victim)
		if !expired {
			c.evictions.Add(1)
		}
	}
}

func (c *Cache[K, V]) findExpiredTailLocked(now time.Time) *list.Element {
	// Bounded inspection from tail keeps eviction O(1) amortized.
	const maxProbe = 8
	el := c.order.Back()
	for i := 0; i < maxProbe && el != nil; i++ {
		node := el.Value.(*lruNode[K, V])
		if now.After(node.expiresAt) {
			return el
		}
		el = el.Prev()
	}
	return nil
}

func (c *Cache[K, V]) removeElemLocked(elem *list.Element) {
	node := elem.Value.(*lruNode[K, V])
	delete(c.items, node.key)
	c.order.Remove(elem)
	c.curBytes -= node.sizeBytes
	if c.curBytes < 0 {
		c.curBytes = 0
	}
}

func (c *Cache[K, V]) formatKey(key K) string {
	if c.keyCodec != nil {
		return c.keyCodec(key)
	}
	return fmt.Sprint(key)
}

func (c *Cache[K, V]) diskKeyHash(key K) string {
	sum := sha256.Sum256([]byte(c.formatKey(key)))
	return hex.EncodeToString(sum[:])
}

func (c *Cache[K, V]) diskPathForKey(key K) string {
	return filepath.Join(c.diskDir, c.diskKeyHash(key)+".json")
}

func (c *Cache[K, V]) readDisk(key K, now time.Time) (V, time.Time, int64, bool) {
	var zero V
	path := c.diskPathForKey(key)

	c.diskMu.RLock()
	raw, err := os.ReadFile(path)
	c.diskMu.RUnlock()
	if err != nil {
		return zero, time.Time{}, 0, false
	}

	c.diskReads.Add(1)

	var env diskEntry[V]
	if err := json.Unmarshal(raw, &env); err != nil {
		// Corrupted file on disk: drop it so it won't poison future lookups.
		c.removeCorruptOrExpiredFile(path)
		return zero, time.Time{}, 0, false
	}
	if env.Key != "" && env.Key != c.formatKey(key) {
		return zero, time.Time{}, 0, false
	}
	if now.After(env.ExpiresAt) {
		c.removeCorruptOrExpiredFile(path)
		return zero, time.Time{}, 0, false
	}

	sizeBytes := c.measureBytes(env.Value, raw)
	return env.Value, env.ExpiresAt, sizeBytes, true
}

func (c *Cache[K, V]) removeCorruptOrExpiredFile(path string) {
	c.diskMu.Lock()
	_ = os.Remove(path)
	c.diskMu.Unlock()
}

func (c *Cache[K, V]) writeDisk(key K, raw []byte) {
	if c.diskDir == "" || (c.diskMaxBytes > 0 && int64(len(raw)) > c.diskMaxBytes) {
		return
	}
	dst := c.diskPathForKey(key)

	c.diskMu.Lock()
	defer c.diskMu.Unlock()

	tmp, err := os.CreateTemp(c.diskDir, ".tmp-*")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return
	}
	cleanupTmp = false
	c.diskWrites.Add(1)
	c.diskWriteCount++
	c.diskApproxBytes += int64(len(raw))

	if c.diskMaxBytes > 0 && (c.diskApproxBytes > c.diskMaxBytes || c.diskWriteCount%32 == 0) {
		c.enforceDiskQuotaLocked(c.now())
	}
}

func (c *Cache[K, V]) deleteDisk(key K) {
	path := c.diskPathForKey(key)
	c.diskMu.Lock()
	err := os.Remove(path)
	c.diskMu.Unlock()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
}

func (c *Cache[K, V]) pruneDisk(now time.Time, skipAlreadyCounted map[string]struct{}) int {
	c.diskMu.Lock()
	defer c.diskMu.Unlock()

	entries, err := os.ReadDir(c.diskDir)
	if err != nil {
		return 0
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		full := filepath.Join(c.diskDir, name)
		if strings.HasPrefix(name, ".tmp-") {
			_ = os.Remove(full)
			continue
		}
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		hash := strings.TrimSuffix(name, ".json")
		raw, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		var header struct {
			ExpiresAt time.Time `json:"expires_at"`
		}
		if err := json.Unmarshal(raw, &header); err != nil || header.ExpiresAt.IsZero() || now.After(header.ExpiresAt) {
			if os.Remove(full) == nil {
				if _, counted := skipAlreadyCounted[hash]; !counted {
					removed++
				}
			}
		}
	}

	if c.diskMaxBytes > 0 {
		c.enforceDiskQuotaLocked(now)
	}
	return removed
}

type diskFileMeta struct {
	path    string
	size    int64
	modTime time.Time
}

func (c *Cache[K, V]) enforceDiskQuotaLocked(now time.Time) {
	entries, err := os.ReadDir(c.diskDir)
	if err != nil {
		return
	}

	var totalBytes int64
	files := make([]diskFileMeta, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		full := filepath.Join(c.diskDir, name)
		if strings.HasPrefix(name, ".tmp-") {
			_ = os.Remove(full)
			continue
		}
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(full)
		if err == nil {
			var header struct {
				ExpiresAt time.Time `json:"expires_at"`
			}
			if json.Unmarshal(raw, &header) != nil || now.After(header.ExpiresAt) {
				_ = os.Remove(full)
				continue
			}
		}
		totalBytes += info.Size()
		files = append(files, diskFileMeta{
			path:    full,
			size:    info.Size(),
			modTime: info.ModTime(),
		})
	}

	if totalBytes <= c.diskMaxBytes {
		c.diskApproxBytes = totalBytes
		return
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})
	for _, f := range files {
		if totalBytes <= c.diskMaxBytes {
			break
		}
		if os.Remove(f.path) == nil {
			totalBytes -= f.size
		}
	}
	c.diskApproxBytes = totalBytes
}

func (c *Cache[K, V]) measureBytes(val V, marshaled []byte) int64 {
	if c.sizeFunc != nil {
		s := c.sizeFunc(val)
		if s < 1 {
			return 1
		}
		return s
	}
	if len(marshaled) > 0 {
		return int64(len(marshaled))
	}

	switch v := any(val).(type) {
	case string:
		if len(v) > 0 {
			return int64(len(v))
		}
		return 8
	case []byte:
		if len(v) > 0 {
			return int64(len(v))
		}
		return 8
	case int, int64, uint, uint64, float64, bool, int32, uint32, float32:
		return 8
	}

	// Only marshal generic structs when byte-budget eviction is enabled.
	if c.maxBytes > 0 {
		if raw, err := json.Marshal(val); err == nil && len(raw) > 0 {
			return int64(len(raw))
		}
	}
	return 64
}
