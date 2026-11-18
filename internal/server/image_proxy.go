package server

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mila/internal/cache"
)

const (
	// DefaultImageCacheQuotaBytes enforces a strict 500 MiB on-disk LRU budget.
	DefaultImageCacheQuotaBytes int64 = 500 << 20
	// DefaultImageMaxObjectBytes caps a single cached upstream image (16 MiB).
	DefaultImageMaxObjectBytes int64 = 16 << 20
	// DefaultImageCacheTTL defines how long cached images stay fresh on disk (24h).
	DefaultImageCacheTTL = 24 * time.Hour

	imageCacheControlHeader = "public, max-age=86400, stale-while-revalidate=3600"
	maxImageURLLength       = 4096
)

var (
	errInvalidImageURL   = errors.New("invalid image url")
	errForbiddenImageURL = errors.New("forbidden image url target")
	errUpstreamImage     = errors.New("upstream image fetch failed")
	errUnsupportedImage  = errors.New("unsupported image content type or size")
)

// ImageProxyConfig configures the caching disk-backed ImageProxy.
type ImageProxyConfig struct {
	CacheDir          string
	QuotaBytes        int64
	MaxObjectBytes    int64
	TTL               time.Duration
	HTTPClient        *http.Client
	AllowPrivateHosts bool
	Now               func() time.Time
}

// ImageProxyStats exposes operational metrics for LRU eviction and upstream deduplication checks.
type ImageProxyStats struct {
	Requests        int64
	Hits            int64
	NotModified304  int64
	Misses          int64
	UpstreamFetches int64
	Evictions       int64
	Entries         int
	Bytes           int64
}

type imageMeta struct {
	Hash        string `json:"hash"`
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	ETag        string `json:"etag"`
	Size        int64  `json:"size"`
	ExpiresAt   int64  `json:"expires_at"`
	LastAccess  int64  `json:"last_access"`
}

type imageFlightResult struct {
	meta imageMeta
	body []byte
	err  error
}

type imageFlight struct {
	done chan struct{}
	res  imageFlightResult
}

// ImageProxy fetches remote artwork once via singleflight, persists files on disk
// under a strict 500 MB LRU quota, and serves cached bytes with ETag/304 & Cache-Control.
type ImageProxy struct {
	dir               string
	quotaBytes        int64
	maxObjectBytes    int64
	ttl               time.Duration
	httpClient        *http.Client
	allowPrivateHosts bool
	now               func() time.Time

	mu         sync.Mutex
	entries    map[string]*list.Element
	lru        *list.List
	totalBytes int64
	inflight   map[string]*imageFlight

	requests        atomic.Int64
	hits            atomic.Int64
	notModified304  atomic.Int64
	misses          atomic.Int64
	upstreamFetches atomic.Int64
	evictions       atomic.Int64
}

// NewImageProxy constructs a caching image proxy and hydrates existing disk metadata into its LRU.
func NewImageProxy(cfg ImageProxyConfig) (*ImageProxy, error) {
	dir := strings.TrimSpace(cfg.CacheDir)
	if dir == "" {
		resolved, err := cache.DefaultDiskCacheDir("images")
		if err != nil {
			return nil, err
		}
		dir = resolved
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	quota := cfg.QuotaBytes
	if quota <= 0 {
		quota = DefaultImageCacheQuotaBytes
	}
	maxObj := cfg.MaxObjectBytes
	if maxObj <= 0 {
		maxObj = DefaultImageMaxObjectBytes
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = DefaultImageCacheTTL
	}
	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	p := &ImageProxy{
		dir:               dir,
		quotaBytes:        quota,
		maxObjectBytes:    maxObj,
		ttl:               ttl,
		allowPrivateHosts: cfg.AllowPrivateHosts,
		now:               nowFn,
		entries:           make(map[string]*list.Element),
		lru:               list.New(),
		inflight:          make(map[string]*imageFlight),
	}

	if cfg.HTTPClient != nil {
		// Copy client so redirect guard attaches safely without mutating caller's client.
		clone := *cfg.HTTPClient
		clone.CheckRedirect = p.checkRedirect
		p.httpClient = &clone
	} else {
		p.httpClient = &http.Client{
			Timeout: 20 * time.Second,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				MaxIdleConns:          128,
				MaxIdleConnsPerHost:   32,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				ForceAttemptHTTP2:     true,
			},
			CheckRedirect: p.checkRedirect,
		}
	}

	p.loadExistingFromDisk()
	return p, nil
}

// Stats returns a thread-safe snapshot of ImageProxy cache usage and counters.
func (p *ImageProxy) Stats() ImageProxyStats {
	p.mu.Lock()
	entries := len(p.entries)
	bytes := p.totalBytes
	p.mu.Unlock()

	return ImageProxyStats{
		Requests:        p.requests.Load(),
		Hits:            p.hits.Load(),
		NotModified304:  p.notModified304.Load(),
		Misses:          p.misses.Load(),
		UpstreamFetches: p.upstreamFetches.Load(),
		Evictions:       p.evictions.Load(),
		Entries:         entries,
		Bytes:           bytes,
	}
}

// ServeHTTP handles GET/HEAD requests for /api/v1/image?url=...
func (p *ImageProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	parsed, err := p.validateURL(rawURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	p.requests.Add(1)
	normalized := parsed.String()
	hash := hashImageURL(normalized)

	if meta, ok := p.lookupFresh(hash); ok {
		if p.serveFromDisk(w, r, meta, "HIT") {
			p.hits.Add(1)
			return
		}
	}

	res := p.fetchSingleflight(r.Context(), hash, normalized)
	if res.err != nil {
		switch {
		case errors.Is(res.err, errForbiddenImageURL), errors.Is(res.err, errInvalidImageURL):
			writeError(w, http.StatusBadRequest, res.err.Error())
		case errors.Is(res.err, context.Canceled), errors.Is(res.err, context.DeadlineExceeded):
			writeError(w, http.StatusGatewayTimeout, "upstream image timeout")
		case errors.Is(res.err, errUnsupportedImage):
			writeError(w, http.StatusUnsupportedMediaType, res.err.Error())
		default:
			writeError(w, http.StatusBadGateway, res.err.Error())
		}
		return
	}

	p.writeImageResponse(w, r, res.meta, res.body, "MISS")
}

func (p *ImageProxy) validateURL(raw string) (*url.URL, error) {
	if raw == "" || len(raw) > maxImageURLLength {
		return nil, errInvalidImageURL
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 0x20 || raw[i] == 0x7f {
			return nil, errInvalidImageURL
		}
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() {
		return nil, errInvalidImageURL
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, errForbiddenImageURL
	}
	if u.User != nil {
		return nil, errForbiddenImageURL
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return nil, errInvalidImageURL
	}
	if !p.allowPrivateHosts && isDisallowedHost(host) {
		return nil, errForbiddenImageURL
	}
	u.Fragment = ""
	return u, nil
}

func (p *ImageProxy) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("stopped after 5 redirects")
	}
	_, err := p.validateURL(req.URL.String())
	return err
}

func isDisallowedHost(host string) bool {
	low := strings.ToLower(strings.TrimSuffix(host, "."))
	if low == "localhost" || strings.HasSuffix(low, ".localhost") || strings.HasSuffix(low, ".local") || strings.HasSuffix(low, ".internal") {
		return true
	}
	ip := net.ParseIP(low)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		// Block RFC 6598 CGNAT (100.64.0.0/10)
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
	}
	return false
}

func hashImageURL(normalizedURL string) string {
	sum := sha256.Sum256([]byte(normalizedURL))
	return hex.EncodeToString(sum[:])
}

func (p *ImageProxy) dataPath(hash string) string {
	return filepath.Join(p.dir, hash+".bin")
}

func (p *ImageProxy) metaPath(hash string) string {
	return filepath.Join(p.dir, hash+".meta.json")
}

func (p *ImageProxy) lookupFresh(hash string) (imageMeta, bool) {
	nowUnix := p.now().UnixNano()
	var stalePaths [2]string

	p.mu.Lock()
	elem, ok := p.entries[hash]
	if !ok {
		p.mu.Unlock()
		return imageMeta{}, false
	}
	meta := elem.Value.(*imageMeta)
	if nowUnix > meta.ExpiresAt {
		stalePaths[0], stalePaths[1] = p.removeElemLocked(elem)
		p.mu.Unlock()
		removeFilePair(stalePaths[0], stalePaths[1])
		return imageMeta{}, false
	}
	meta.LastAccess = nowUnix
	p.lru.MoveToFront(elem)
	res := *meta
	p.mu.Unlock()
	return res, true
}

func (p *ImageProxy) serveFromDisk(w http.ResponseWriter, r *http.Request, meta imageMeta, xCache string) bool {
	dataPath := p.dataPath(meta.Hash)
	if etagMatches(r.Header.Get("If-None-Match"), meta.ETag) {
		// Verify backing file still exists before claiming 304
		if _, err := os.Stat(dataPath); err == nil {
			p.notModified304.Add(1)
			p.setCacheHeaders(w, meta, xCache)
			w.WriteHeader(http.StatusNotModified)
			return true
		}
		p.evictHash(meta.Hash)
		return false
	}

	f, err := os.Open(dataPath)
	if err != nil {
		p.evictHash(meta.Hash)
		return false
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		p.evictHash(meta.Hash)
		return false
	}

	p.setCacheHeaders(w, meta, xCache)
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, f)
	}
	return true
}

func (p *ImageProxy) writeImageResponse(w http.ResponseWriter, r *http.Request, meta imageMeta, body []byte, xCache string) {
	p.setCacheHeaders(w, meta, xCache)
	if etagMatches(r.Header.Get("If-None-Match"), meta.ETag) {
		p.notModified304.Add(1)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func (p *ImageProxy) setCacheHeaders(w http.ResponseWriter, meta imageMeta, xCache string) {
	if meta.ETag != "" {
		w.Header().Set("ETag", meta.ETag)
	}
	w.Header().Set("Cache-Control", imageCacheControlHeader)
	w.Header().Set("X-Cache", xCache)
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func (p *ImageProxy) fetchSingleflight(ctx context.Context, hash, targetURL string) imageFlightResult {
	// Re-check fresh cache once before allocating an inflight slot.
	if meta, ok := p.lookupFresh(hash); ok {
		if raw, err := os.ReadFile(p.dataPath(hash)); err == nil {
			p.hits.Add(1)
			return imageFlightResult{meta: meta, body: raw}
		}
		p.evictHash(hash)
	}

	var expiredPaths [2]string
	p.mu.Lock()
	if elem, ok := p.entries[hash]; ok {
		meta := *elem.Value.(*imageMeta)
		if p.now().UnixNano() <= meta.ExpiresAt {
			p.lru.MoveToFront(elem)
			p.mu.Unlock()
			if raw, err := os.ReadFile(p.dataPath(hash)); err == nil {
				p.hits.Add(1)
				return imageFlightResult{meta: meta, body: raw}
			}
			p.evictHash(hash)
			p.mu.Lock()
		} else {
			expiredPaths[0], expiredPaths[1] = p.removeElemLocked(elem)
		}
	}

	if fl, running := p.inflight[hash]; running {
		p.mu.Unlock()
		removeFilePair(expiredPaths[0], expiredPaths[1])
		select {
		case <-fl.done:
			if fl.res.err == nil {
				p.hits.Add(1)
			}
			return fl.res
		case <-ctx.Done():
			return imageFlightResult{err: ctx.Err()}
		}
	}

	p.misses.Add(1)
	fl := &imageFlight{done: make(chan struct{})}
	p.inflight[hash] = fl
	p.mu.Unlock()

	// Decouple upstream fetch from a single caller's cancellation so 9 coalesced waiters succeed.
	fetchCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	res := p.downloadAndStore(fetchCtx, hash, targetURL)
	cancel()

	p.mu.Lock()
	fl.res = res
	delete(p.inflight, hash)
	close(fl.done)
	p.mu.Unlock()

	if ctx.Err() != nil && res.err == nil {
		return res
	}
	return res
}

func (p *ImageProxy) downloadAndStore(ctx context.Context, hash, targetURL string) imageFlightResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return imageFlightResult{err: errInvalidImageURL}
	}
	req.Header.Set("User-Agent", "mila-desktop/1.0")
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/*;q=0.8")

	p.upstreamFetches.Add(1)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return imageFlightResult{err: fmt.Errorf("%w: %v", errUpstreamImage, err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return imageFlightResult{err: fmt.Errorf("%w: HTTP %d", errUpstreamImage, resp.StatusCode)}
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, p.maxObjectBytes+1))
	if err != nil {
		return imageFlightResult{err: fmt.Errorf("%w: %v", errUpstreamImage, err)}
	}
	if int64(len(raw)) == 0 || int64(len(raw)) > p.maxObjectBytes || int64(len(raw)) > p.quotaBytes {
		return imageFlightResult{err: fmt.Errorf("%w: payload size invalid (%d bytes)", errUnsupportedImage, len(raw))}
	}

	ct, ok := normalizeSafeImageContentType(resp.Header.Get("Content-Type"), raw)
	if !ok {
		return imageFlightResult{err: fmt.Errorf("%w: non-image or blocked type %q", errUnsupportedImage, resp.Header.Get("Content-Type"))}
	}

	bodySum := sha256.Sum256(raw)
	etag := `"` + hex.EncodeToString(bodySum[:16]) + `"`

	now := p.now()
	meta := imageMeta{
		Hash:        hash,
		URL:         targetURL,
		ContentType: ct,
		ETag:        etag,
		Size:        int64(len(raw)),
		ExpiresAt:   now.Add(p.ttl).UnixNano(),
		LastAccess:  now.UnixNano(),
	}

	if err := p.writeAtomic(p.dataPath(hash), raw); err != nil {
		return imageFlightResult{meta: meta, body: raw}
	}
	metaRaw, err := json.Marshal(meta)
	if err == nil {
		_ = p.writeAtomic(p.metaPath(hash), metaRaw)
	}

	p.insertAndEnforceQuota(meta)
	return imageFlightResult{meta: meta, body: raw}
}

func (p *ImageProxy) insertAndEnforceQuota(meta imageMeta) {
	var evicted [][2]string

	p.mu.Lock()
	if elem, exists := p.entries[meta.Hash]; exists {
		old := elem.Value.(*imageMeta)
		p.totalBytes -= old.Size
		*old = meta
		p.totalBytes += meta.Size
		p.lru.MoveToFront(elem)
	} else {
		copied := meta
		elem := p.lru.PushFront(&copied)
		p.entries[meta.Hash] = elem
		p.totalBytes += meta.Size
	}

	for p.totalBytes > p.quotaBytes {
		victim := p.lru.Back()
		if victim == nil {
			break
		}
		dp, mp := p.removeElemLocked(victim)
		evicted = append(evicted, [2]string{dp, mp})
		p.evictions.Add(1)
	}
	p.mu.Unlock()

	for _, pair := range evicted {
		removeFilePair(pair[0], pair[1])
	}
}

func (p *ImageProxy) evictHash(hash string) {
	var dp, mp string
	p.mu.Lock()
	if elem, exists := p.entries[hash]; exists {
		dp, mp = p.removeElemLocked(elem)
	}
	p.mu.Unlock()
	removeFilePair(dp, mp)
}

func (p *ImageProxy) removeElemLocked(elem *list.Element) (string, string) {
	meta := elem.Value.(*imageMeta)
	delete(p.entries, meta.Hash)
	p.lru.Remove(elem)
	p.totalBytes -= meta.Size
	if p.totalBytes < 0 {
		p.totalBytes = 0
	}
	return p.dataPath(meta.Hash), p.metaPath(meta.Hash)
}

func removeFilePair(dataPath, metaPath string) {
	if dataPath != "" {
		_ = os.Remove(dataPath)
	}
	if metaPath != "" {
		_ = os.Remove(metaPath)
	}
}

func (p *ImageProxy) loadExistingFromDisk() {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		return
	}

	nowUnix := p.now().UnixNano()
	loaded := make([]*imageMeta, 0, len(entries)/2)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".tmp-") {
			_ = os.Remove(filepath.Join(p.dir, name))
			continue
		}
		if !strings.HasSuffix(name, ".meta.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(p.dir, name))
		if err != nil {
			continue
		}
		var meta imageMeta
		if err := json.Unmarshal(raw, &meta); err != nil || meta.Hash == "" {
			_ = os.Remove(filepath.Join(p.dir, name))
			continue
		}
		binPath := p.dataPath(meta.Hash)
		info, statErr := os.Stat(binPath)
		if statErr != nil || nowUnix > meta.ExpiresAt || info.Size() != meta.Size {
			_ = os.Remove(binPath)
			_ = os.Remove(filepath.Join(p.dir, name))
			continue
		}
		loaded = append(loaded, &meta)
	}

	// Sort most-recent first so PushBack preserves MRU -> LRU ordering cleanly.
	sort.Slice(loaded, func(i, j int) bool {
		return loaded[i].LastAccess > loaded[j].LastAccess
	})

	p.mu.Lock()
	defer p.mu.Unlock()
	for _, m := range loaded {
		elem := p.lru.PushBack(m)
		p.entries[m.Hash] = elem
		p.totalBytes += m.Size
	}
	for p.totalBytes > p.quotaBytes {
		victim := p.lru.Back()
		if victim == nil {
			break
		}
		p.removeElemLocked(victim)
		p.evictions.Add(1)
	}
}

func (p *ImageProxy) writeAtomic(dst string, data []byte) error {
	tmp, err := os.CreateTemp(p.dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func normalizeSafeImageContentType(headerCT string, body []byte) (string, bool) {
	base := strings.ToLower(strings.TrimSpace(strings.Split(headerCT, ";")[0]))
	if base == "image/svg+xml" || base == "text/html" || base == "application/xhtml+xml" {
		return "", false
	}

	sniffed := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(body), ";")[0]))
	// Allow explicit raster/modern image types or sniffed raster types when upstream omits header.
	switch base {
	case "image/jpeg", "image/jpg", "image/png", "image/webp", "image/gif", "image/avif", "image/bmp", "image/x-icon", "image/vnd.microsoft.icon":
		return base, true
	case "", "application/octet-stream", "binary/octet-stream":
		if strings.HasPrefix(sniffed, "image/") && sniffed != "image/svg+xml" {
			return sniffed, true
		}
		// Detect ISOBMFF AVIF box (`ftypavif` / `ftypavis`) which net/http may label octet-stream
		if len(body) >= 12 && string(body[4:8]) == "ftyp" && (string(body[8:12]) == "avif" || string(body[8:12]) == "avis") {
			return "image/avif", true
		}
		return "", false
	default:
		if strings.HasPrefix(base, "image/") {
			return base, true
		}
		return "", false
	}
}

func etagMatches(headerVal string, currentETag string) bool {
	if strings.TrimSpace(headerVal) == "" || currentETag == "" {
		return false
	}
	cleanCurrent := strings.TrimPrefix(strings.TrimSpace(currentETag), "W/")
	for _, part := range strings.Split(headerVal, ",") {
		cand := strings.TrimSpace(part)
		if cand == "" {
			continue
		}
		if cand == "*" {
			return true
		}
		candClean := strings.TrimPrefix(cand, "W/")
		if candClean == cleanCurrent {
			return true
		}
	}
	return false
}
