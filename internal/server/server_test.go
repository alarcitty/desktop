package server

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mila/internal/aniliberty"
	"mila/internal/anime"
	"mila/internal/logger"
	"mila/internal/shikimori"
)

func TestServerRoutesAndValidation(t *testing.T) {
	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/anime/releases/latest":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 1, "alias": "test-anime", "name": map[string]any{"main": "Тест"}},
			})
		case "/anime/releases/test-anime":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    1,
				"alias": "test-anime",
				"name":  map[string]any{"main": "Тест"},
				"episodes": []map[string]any{
					{
						"id":       "ep-1",
						"ordinal":  1,
						"duration": 1400,
						"hls_1080": "https://cdn.example/1080.m3u8",
						"hls_720":  "https://cdn.example/720.m3u8",
						"hls_480":  "https://cdn.example/480.m3u8",
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer libSrv.Close()

	shikiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"animes": []any{}}})
	}))
	defer shikiSrv.Close()

	svc := anime.NewService(
		aniliberty.New(aniliberty.Config{BaseURL: libSrv.URL}),
		shikimori.New(shikimori.Config{BaseURL: shikiSrv.URL, MinInterval: time.Millisecond}),
	)
	handler := New(svc).Handler()

	// Health check
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on /health, got %d", rec.Code)
	}

	// Missing query param -> 400
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/anime/search", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on missing q, got %d", rec.Code)
	}

	// Latest endpoint -> 200 + data envelope + CORS
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/anime/latest", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on latest, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("expected CORS header on response")
	}

	// Not found release -> 404
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/anime/releases/unknown", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on unknown release, got %d", rec.Code)
	}

	// Master HLS playlist -> 200 with 1080p first
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/anime/releases/test-anime/episodes/1/master.m3u8", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on master.m3u8, got %d", rec.Code)
	}
	body := rec.Body.String()
	if body == "" || body[:7] != "#EXTM3U" {
		t.Fatalf("unexpected master playlist body: %q", body)
	}
}

// valid1x1PNG decodes a minimal valid 1x1 transparent PNG payload for offline image proxy tests.
func valid1x1PNG() []byte {
	const b64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic(err)
	}
	return raw
}

func newTestServerForImages(t *testing.T, cacheDir string, cfg ImageProxyConfig) *Server {
	t.Helper()
	libSrv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(libSrv.Close)

	shikiSrv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(shikiSrv.Close)

	svc := anime.NewService(
		aniliberty.New(aniliberty.Config{BaseURL: libSrv.URL}),
		shikimori.New(shikimori.Config{BaseURL: shikiSrv.URL, MinInterval: time.Millisecond}),
	)
	cfg.CacheDir = cacheDir
	return New(svc, WithImageProxyConfig(cfg))
}

func TestImageProxySingleflightHitAndETag304(t *testing.T) {
	pngBytes := valid1x1PNG()
	var upstreamHits atomic.Int64

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits.Add(1)
		time.Sleep(25 * time.Millisecond)
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pngBytes)
	}))
	defer upstream.Close()

	cacheDir := t.TempDir()
	srv := newTestServerForImages(t, cacheDir, ImageProxyConfig{
		QuotaBytes:        DefaultImageCacheQuotaBytes,
		AllowPrivateHosts: true,
	})
	handler := srv.Handler()

	targetURL := upstream.URL + "/posters/123.png"
	proxyReqPath := "/api/v1/image?url=" + url.QueryEscape(targetURL)

	// 1. 10 concurrent requests -> singleflight deduplicates to exactly 1 upstream fetch.
	const concurrency = 10
	start := make(chan struct{})
	var wg sync.WaitGroup
	etags := make([]string, concurrency)
	statuses := make([]int, concurrency)
	bodies := make([][]byte, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, proxyReqPath, nil))
			statuses[idx] = rec.Code
			etags[idx] = rec.Header().Get("ETag")
			bodies[idx] = rec.Body.Bytes()
		}(i)
	}
	close(start)
	wg.Wait()

	if got := upstreamHits.Load(); got != 1 {
		t.Fatalf("expected 1 upstream request under 10x singleflight concurrency, got %d", got)
	}
	for i := 0; i < concurrency; i++ {
		if statuses[i] != http.StatusOK {
			t.Fatalf("worker %d expected 200, got %d", i, statuses[i])
		}
		if !bytes.Equal(bodies[i], pngBytes) {
			t.Fatalf("worker %d received unexpected image bytes", i)
		}
		if etags[i] == "" || etags[i] != etags[0] {
			t.Fatalf("worker %d mismatch ETag: got %q want %q", i, etags[i], etags[0])
		}
	}

	// 2. Repeat GET -> 0 upstream hits, X-Cache: HIT, Cache-Control set.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, proxyReqPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("repeat GET expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Cache") != "HIT" {
		t.Fatalf("repeat GET expected X-Cache=HIT, got %q", rec.Header().Get("X-Cache"))
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "max-age=86400") {
		t.Fatalf("expected Cache-Control with max-age=86400, got %q", rec.Header().Get("Cache-Control"))
	}
	if upstreamHits.Load() != 1 {
		t.Fatalf("repeat GET must not hit upstream; got %d", upstreamHits.Load())
	}

	// 3. HEAD request -> 200 OK with empty body and matching ETag.
	headRec := httptest.NewRecorder()
	handler.ServeHTTP(headRec, httptest.NewRequest(http.MethodHead, proxyReqPath, nil))
	if headRec.Code != http.StatusOK || headRec.Body.Len() != 0 {
		t.Fatalf("HEAD expected 200 with empty body, got code=%d len=%d", headRec.Code, headRec.Body.Len())
	}
	if headRec.Header().Get("ETag") != etags[0] {
		t.Fatalf("HEAD ETag mismatch: got %q want %q", headRec.Header().Get("ETag"), etags[0])
	}

	// 4. Conditional GET with If-None-Match -> 304 Not Modified, empty body, 0 upstream hits.
	condReq := httptest.NewRequest(http.MethodGet, proxyReqPath, nil)
	condReq.Header.Set("If-None-Match", etags[0])
	condRec := httptest.NewRecorder()
	handler.ServeHTTP(condRec, condReq)
	if condRec.Code != http.StatusNotModified {
		t.Fatalf("expected 304 Not Modified, got %d", condRec.Code)
	}
	if condRec.Body.Len() != 0 {
		t.Fatalf("expected empty body on 304, got %d bytes", condRec.Body.Len())
	}
	if upstreamHits.Load() != 1 {
		t.Fatalf("304 check must not hit upstream; got %d", upstreamHits.Load())
	}
	if stats := srv.ImageProxy().Stats(); stats.NotModified304 != 1 || stats.UpstreamFetches != 1 {
		t.Fatalf("unexpected ImageProxy stats: %+v", stats)
	}

	// 5. Persistent disk cache survives Server restart -> 0 upstream hits on new Server instance.
	restarted := newTestServerForImages(t, cacheDir, ImageProxyConfig{
		QuotaBytes:        DefaultImageCacheQuotaBytes,
		AllowPrivateHosts: true,
	})
	restartRec := httptest.NewRecorder()
	restarted.Handler().ServeHTTP(restartRec, httptest.NewRequest(http.MethodGet, proxyReqPath, nil))
	if restartRec.Code != http.StatusOK || restartRec.Header().Get("X-Cache") != "HIT" {
		t.Fatalf("restarted server expected 200 HIT, got code=%d X-Cache=%q", restartRec.Code, restartRec.Header().Get("X-Cache"))
	}
	if !bytes.Equal(restartRec.Body.Bytes(), pngBytes) {
		t.Fatalf("restarted server returned corrupted cached payload")
	}
	if upstreamHits.Load() != 1 {
		t.Fatalf("restarted server must serve from disk without upstream hit; got %d", upstreamHits.Load())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestImageProxySecurityAndValidation(t *testing.T) {
	var upstreamHits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits.Add(1)
		switch r.URL.Path {
		case "/bad.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
		case "/page.html":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html>not an image</html>`))
		default:
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(valid1x1PNG())
		}
	}))
	defer upstream.Close()

	// Default Server disables private/loopback targets (SSRF protection).
	strictSrv := newTestServerForImages(t, t.TempDir(), ImageProxyConfig{})
	strictHandler := strictSrv.Handler()

	badTargets := []string{
		"/api/v1/image",
		"/api/v1/image?url=",
		"/api/v1/image?url=" + url.QueryEscape("file:///etc/passwd"),
		"/api/v1/image?url=" + url.QueryEscape("javascript:alert(1)"),
		"/api/v1/image?url=" + url.QueryEscape("http://127.0.0.1/secret.png"),
		"/api/v1/image?url=" + url.QueryEscape("http://localhost:8080/secret.png"),
		"/api/v1/image?url=" + url.QueryEscape("http://169.254.169.254/latest/meta-data/"),
		"/api/v1/image?url=" + url.QueryEscape("https://user:pass@example.com/avatar.png"),
	}
	for _, path := range badTargets {
		rec := httptest.NewRecorder()
		strictHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for %s, got %d (%s)", path, rec.Code, rec.Body.String())
		}
	}

	// Redirect SSRF guard: public URL redirecting to 127.0.0.1 must be rejected.
	redirectProxy, err := NewImageProxy(ImageProxyConfig{
		CacheDir:          t.TempDir(),
		AllowPrivateHosts: false,
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				resp := httptest.NewRecorder()
				resp.Header().Set("Location", "http://127.0.0.1/internal-secret.png")
				resp.WriteHeader(http.StatusFound)
				return resp.Result(), nil
			}),
		},
	})
	if err != nil {
		t.Fatalf("NewImageProxy redirect test setup failed: %v", err)
	}
	redirRec := httptest.NewRecorder()
	redirectProxy.ServeHTTP(redirRec, httptest.NewRequest(http.MethodGet, "/api/v1/image?url="+url.QueryEscape("https://cdn.example.org/poster.png"), nil))
	if redirRec.Code < 400 {
		t.Fatalf("expected redirect to loopback to be blocked, got %d", redirRec.Code)
	}

	// Non-image / SVG XSS payloads rejected even from allowed upstreams.
	devSrv := newTestServerForImages(t, t.TempDir(), ImageProxyConfig{AllowPrivateHosts: true})
	for _, p := range []string{"/bad.svg", "/page.html"} {
		rec := httptest.NewRecorder()
		devSrv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/image?url="+url.QueryEscape(upstream.URL+p), nil))
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("expected 415 Unsupported Media Type for %s, got %d", p, rec.Code)
		}
	}
	if upstreamHits.Load() != 2 {
		t.Fatalf("expected only the 2 mime-checked requests to hit upstream, got %d", upstreamHits.Load())
	}
}

func TestImageProxyDiskLRUQuotaAnd1000Entries(t *testing.T) {
	if DefaultImageCacheQuotaBytes != 500<<20 {
		t.Fatalf("expected DefaultImageCacheQuotaBytes == 500 MiB, got %d", DefaultImageCacheQuotaBytes)
	}

	basePNG := valid1x1PNG()
	paddedPayload := make([]byte, 256)
	copy(paddedPayload, basePNG)

	var fetches sync.Map // url -> int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		val, _ := fetches.LoadOrStore(r.URL.Path, new(atomic.Int64))
		val.(*atomic.Int64).Add(1)
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(paddedPayload)
	}))
	defer upstream.Close()

	const quotaBytes = 120 * 1024 // ~468 entries max out of 1100
	const totalItems = 1100
	cacheDir := t.TempDir()
	srv := newTestServerForImages(t, cacheDir, ImageProxyConfig{
		QuotaBytes:        quotaBytes,
		AllowPrivateHosts: true,
	})
	handler := srv.Handler()

	// Insert 1100 distinct cached images (~281 KB total > 120 KB quota).
	for i := 0; i < totalItems; i++ {
		imgURL := fmt.Sprintf("%s/item/%04d.png", upstream.URL, i)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/image?url="+url.QueryEscape(imgURL), nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("item %d expected 200, got %d", i, rec.Code)
		}
	}

	stats := srv.ImageProxy().Stats()
	if stats.Bytes > quotaBytes {
		t.Fatalf("disk cache exceeded quota: %d > %d", stats.Bytes, quotaBytes)
	}
	if stats.Evictions == 0 {
		t.Fatalf("expected LRU evictions across %d items under %d byte quota, got 0", totalItems, quotaBytes)
	}

	// Verify actual .bin file bytes on disk match tracked stats and stay within quota.
	var actualDiskBytes int64
	err := filepath.WalkDir(cacheDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || filepath.Ext(path) != ".bin" {
			return walkErr
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		actualDiskBytes += info.Size()
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s) failed: %v", cacheDir, err)
	}
	if actualDiskBytes != stats.Bytes || actualDiskBytes > quotaBytes {
		t.Fatalf("disk .bin sum mismatch: disk=%d stats=%d quota=%d", actualDiskBytes, stats.Bytes, quotaBytes)
	}

	// MRU item (latest inserted) must still be cached (X-Cache: HIT, fetch count == 1).
	newestURL := fmt.Sprintf("%s/item/%04d.png", upstream.URL, totalItems-1)
	hitRec := httptest.NewRecorder()
	handler.ServeHTTP(hitRec, httptest.NewRequest(http.MethodGet, "/api/v1/image?url="+url.QueryEscape(newestURL), nil))
	if hitRec.Code != http.StatusOK || hitRec.Header().Get("X-Cache") != "HIT" {
		t.Fatalf("expected newest MRU item HIT, got code=%d X-Cache=%q", hitRec.Code, hitRec.Header().Get("X-Cache"))
	}
	if v, ok := fetches.Load(fmt.Sprintf("/item/%04d.png", totalItems-1)); !ok || v.(*atomic.Int64).Load() != 1 {
		t.Fatalf("expected newest item to remain cached without re-fetch")
	}

	// LRU item (oldest item 0) must have been evicted -> re-fetch triggers upstream count 2.
	oldestURL := fmt.Sprintf("%s/item/%04d.png", upstream.URL, 0)
	missRec := httptest.NewRecorder()
	handler.ServeHTTP(missRec, httptest.NewRequest(http.MethodGet, "/api/v1/image?url="+url.QueryEscape(oldestURL), nil))
	if missRec.Code != http.StatusOK || missRec.Header().Get("X-Cache") != "MISS" {
		t.Fatalf("expected oldest LRU item eviction MISS, got code=%d X-Cache=%q", missRec.Code, missRec.Header().Get("X-Cache"))
	}
	if v, ok := fetches.Load("/item/0000.png"); !ok || v.(*atomic.Int64).Load() != 2 {
		t.Fatalf("expected oldest item to be re-fetched after LRU eviction")
	}
}

func TestDiagnosticsZipExportEndpointAndHealthContract(t *testing.T) {
	logDir := t.TempDir()
	if _, err := logger.ConfigureDefaultFileSink(logger.FileConfig{
		Dir:          logDir,
		MaxFileBytes: 4096,
		MaxFiles:     4,
	}); err != nil {
		t.Fatalf("ConfigureDefaultFileSink failed: %v", err)
	}
	t.Cleanup(func() {
		_ = logger.CloseDefaultSink()
	})

	logger.Make("http-test").Info("check.secret", "Authorization", "Bearer http-secret-999")

	srv := newTestServerForImages(t, t.TempDir(), ImageProxyConfig{AllowPrivateHosts: true})
	handler := srv.Handler()

	// Unified /health contract.
	healthRec := httptest.NewRecorder()
	handler.ServeHTTP(healthRec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if healthRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on /health, got %d", healthRec.Code)
	}
	var healthBody struct {
		Status       string   `json:"status"`
		Mode         string   `json:"mode"`
		APIBase      string   `json:"apiBase"`
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(healthRec.Body.Bytes(), &healthBody); err != nil {
		t.Fatalf("invalid /health JSON: %v", err)
	}
	if healthBody.Status != "ok" || healthBody.Mode != "http" || healthBody.APIBase != "/api/v1" || len(healthBody.Capabilities) == 0 {
		t.Fatalf("unexpected /health payload: %+v", healthBody)
	}

	// POST /api/v1/diagnostics/logs -> JSON report & ZIP file on disk.
	exportDir := t.TempDir()
	exportRec := httptest.NewRecorder()
	handler.ServeHTTP(exportRec, httptest.NewRequest(http.MethodPost, "/api/v1/diagnostics/logs?dir="+url.QueryEscape(exportDir), nil))
	if exportRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on POST diagnostics export, got %d (%s)", exportRec.Code, exportRec.Body.String())
	}
	var exportEnv struct {
		Data logger.DiagnosticsReport `json:"data"`
	}
	if err := json.Unmarshal(exportRec.Body.Bytes(), &exportEnv); err != nil {
		t.Fatalf("invalid diagnostics export JSON: %v", err)
	}
	if !exportEnv.Data.Redacted || exportEnv.Data.ZipPath == "" {
		t.Fatalf("unexpected diagnostics export report: %+v", exportEnv.Data)
	}

	// GET /api/v1/diagnostics/logs.zip -> valid application/zip stream with zero secrets.
	zipRec := httptest.NewRecorder()
	handler.ServeHTTP(zipRec, httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/logs.zip", nil))
	if zipRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on logs.zip, got %d", zipRec.Code)
	}
	if ct := zipRec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("unexpected Content-Type: %q", ct)
	}
	payload := zipRec.Body.Bytes()
	if _, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("returned body is not a valid zip archive: %v", err)
	}
	if bytes.Contains(payload, []byte("http-secret-999")) {
		t.Fatalf("secret leaked in diagnostic zip HTTP response")
	}
}
