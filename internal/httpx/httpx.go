package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseDelay = 250 * time.Millisecond
	defaultMaxDelay  = 5 * time.Second
)

var (
	sharedTransportOnce sync.Once
	sharedTransport     *http.Transport
	rngMu               sync.Mutex
	rng                 = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// SharedTransport returns a process-wide keep-alive HTTP/2-capable transport.
func SharedTransport() *http.Transport {
	sharedTransportOnce.Do(func() {
		sharedTransport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   6 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          128,
			MaxIdleConnsPerHost:   32,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}
	})
	return sharedTransport
}

// NewClient constructs an http.Client backed by SharedTransport with the given request timeout.
func NewClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: SharedTransport(),
	}
}

// HostRule defines token-bucket throughput limits for a host.
type HostRule struct {
	RatePerSec float64
	Burst      int
}

type tokenBucket struct {
	mu     sync.Mutex
	tokens float64
	max    float64
	rate   float64
	last   time.Time
}

func newTokenBucket(ratePerSec float64, burst int, now time.Time) *tokenBucket {
	if ratePerSec <= 0 {
		ratePerSec = 8
	}
	if burst < 1 {
		burst = 1
	}
	return &tokenBucket{
		tokens: float64(burst),
		max:    float64(burst),
		rate:   ratePerSec,
		last:   now,
	}
}

func (b *tokenBucket) wait(ctx context.Context, nowFn func() time.Time) error {
	for {
		b.mu.Lock()
		now := nowFn()
		elapsed := now.Sub(b.last).Seconds()
		if elapsed > 0 {
			b.tokens += elapsed * b.rate
			if b.tokens > b.max {
				b.tokens = b.max
			}
			b.last = now
		}
		if b.tokens >= 1 {
			b.tokens -= 1
			b.mu.Unlock()
			return nil
		}
		waitSec := (1 - b.tokens) / b.rate
		waitDur := time.Duration(waitSec * float64(time.Second))
		if waitDur < time.Millisecond {
			waitDur = time.Millisecond
		}
		b.mu.Unlock()

		timer := time.NewTimer(waitDur)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// DomainLimiter enforces per-domain token-bucket rate limits across concurrent callers.
type DomainLimiter struct {
	mu          sync.Mutex
	buckets     map[string]*tokenBucket
	rules       map[string]HostRule
	defaultRule HostRule
	nowFn       func() time.Time
}

// NewDomainLimiter creates a per-host token bucket limiter with optional host overrides.
func NewDomainLimiter(defaultRule HostRule, overrides map[string]HostRule) *DomainLimiter {
	if defaultRule.RatePerSec <= 0 {
		defaultRule.RatePerSec = 12
	}
	if defaultRule.Burst < 1 {
		defaultRule.Burst = 6
	}
	copied := make(map[string]HostRule, len(overrides))
	for k, v := range overrides {
		copied[strings.ToLower(strings.TrimSpace(k))] = v
	}
	return &DomainLimiter{
		buckets:     make(map[string]*tokenBucket),
		rules:       copied,
		defaultRule: defaultRule,
		nowFn:       time.Now,
	}
}

// Wait acquires one token for the request URL host or returns when ctx cancels.
func (d *DomainLimiter) Wait(ctx context.Context, rawURL string) error {
	if d == nil {
		return nil
	}
	host := extractHostKey(rawURL)
	if host == "" {
		return nil
	}

	d.mu.Lock()
	b, ok := d.buckets[host]
	if !ok {
		rule, hasOverride := d.rules[host]
		if !hasOverride {
			for pattern, r := range d.rules {
				if host == pattern || strings.HasSuffix(host, "."+pattern) {
					rule = r
					hasOverride = true
					break
				}
			}
		}
		if !hasOverride {
			rule = d.defaultRule
		}
		b = newTokenBucket(rule.RatePerSec, rule.Burst, d.nowFn())
		d.buckets[host] = b
	}
	d.mu.Unlock()

	return b.wait(ctx, d.nowFn)
}

func extractHostKey(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// ParseRetryAfter parses an HTTP Retry-After header (seconds or HTTP-date).
func ParseRetryAfter(header string, now time.Time) (time.Duration, bool) {
	trimmed := strings.TrimSpace(header)
	if trimmed == "" {
		return 0, false
	}
	if sec, err := strconv.Atoi(trimmed); err == nil {
		if sec <= 0 {
			return 0, true
		}
		return time.Duration(sec) * time.Second, true
	}
	if ts, err := http.ParseTime(trimmed); err == nil {
		delta := ts.Sub(now)
		if delta <= 0 {
			return 0, true
		}
		return delta, true
	}
	return 0, false
}

// BackoffDelay computes retry wait duration honoring Retry-After or using jittered exponential backoff.
func BackoffDelay(attempt int, base, maxDelay time.Duration, retryAfter string) time.Duration {
	if base <= 0 {
		base = defaultBaseDelay
	}
	if maxDelay <= 0 {
		maxDelay = defaultMaxDelay
	}
	if ra, ok := ParseRetryAfter(retryAfter, time.Now()); ok {
		if ra > maxDelay {
			return maxDelay
		}
		if ra > 0 {
			return ra
		}
	}
	delay := base
	for i := 0; i < attempt; i++ {
		if delay >= maxDelay/2 {
			delay = maxDelay
			break
		}
		delay *= 2
	}
	if delay > maxDelay {
		delay = maxDelay
	}
	// Apply +/- 20% jitter to avoid thundering herd retries.
	rngMu.Lock()
	jitterFactor := 0.8 + rng.Float64()*0.4
	rngMu.Unlock()
	jittered := time.Duration(float64(delay) * jitterFactor)
	if jittered < 5*time.Millisecond {
		jittered = 5 * time.Millisecond
	}
	return jittered
}

// SleepContext pauses for d or aborts immediately on ctx cancellation.
func SleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// RetryConfig configures automatic retry/backoff behavior for DoWithRetry.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	MaxBody     int64
}

// ResponseResult holds a materialized HTTP response body and metadata.
type ResponseResult struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// StatusError captures non-2xx HTTP responses after retry policy completes.
type StatusError struct {
	StatusCode int
	URL        string
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("httpx: %s returned HTTP %d", e.URL, e.StatusCode)
}

// FlightGroup deduplicates concurrent work sharing the same string key.
type FlightGroup[V any] struct {
	mu    sync.Mutex
	calls map[string]*flightCall[V]
}

type flightCall[V any] struct {
	done chan struct{}
	val  V
	err  error
}

// Do invokes fn at most once per concurrent key; waiting callers respect their own ctx cancellation.
func (g *FlightGroup[V]) Do(ctx context.Context, key string, fn func(context.Context) (V, error)) (V, error) {
	if g == nil || strings.TrimSpace(key) == "" {
		return fn(ctx)
	}
	g.mu.Lock()
	if g.calls == nil {
		g.calls = make(map[string]*flightCall[V])
	}
	if existing, ok := g.calls[key]; ok {
		g.mu.Unlock()
		select {
		case <-existing.done:
			return existing.val, existing.err
		case <-ctx.Done():
			var zero V
			return zero, ctx.Err()
		}
	}

	call := &flightCall[V]{done: make(chan struct{})}
	g.calls[key] = call
	g.mu.Unlock()

	go func() {
		defer close(call.done)
		// Run shared flight with detached timeout-safe background context if caller supplied one?
		// Keep leader ctx so caller deadlines propagate properly, removing entry only after completion.
		call.val, call.err = fn(ctx)
		g.mu.Lock()
		if g.calls[key] == call {
			delete(g.calls, key)
		}
		g.mu.Unlock()
	}()

	select {
	case <-call.done:
		return call.val, call.err
	case <-ctx.Done():
		var zero V
		return zero, ctx.Err()
	}
}

// DoWithRetry executes reqBuilder with domain rate limiting and exponential retry on 429/5xx/transport errors.
func DoWithRetry(
	ctx context.Context,
	client *http.Client,
	limiter *DomainLimiter,
	cfg RetryConfig,
	reqBuilder func(context.Context) (*http.Request, error),
) (ResponseResult, error) {
	if client == nil {
		client = NewClient(12 * time.Second)
	}
	attempts := cfg.MaxAttempts
	if attempts < 1 {
		attempts = 3
	}
	baseDelay := cfg.BaseDelay
	if baseDelay <= 0 {
		baseDelay = defaultBaseDelay
	}
	maxDelay := cfg.MaxDelay
	if maxDelay <= 0 {
		maxDelay = defaultMaxDelay
	}
	maxBody := cfg.MaxBody
	if maxBody <= 0 {
		maxBody = 8 << 20
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := reqBuilder(ctx)
		if err != nil {
			return ResponseResult{}, err
		}
		if limiter != nil && req.URL != nil {
			if err := limiter.Wait(ctx, req.URL.String()); err != nil {
				return ResponseResult{}, err
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return ResponseResult{}, err
			}
			if attempt+1 < attempts {
				if sErr := SleepContext(ctx, BackoffDelay(attempt, baseDelay, maxDelay, "")); sErr != nil {
					return ResponseResult{}, sErr
				}
			}
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			retryAfter := resp.Header.Get("Retry-After")
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			lastErr = &StatusError{StatusCode: resp.StatusCode, URL: req.URL.String()}
			if attempt+1 < attempts {
				if sErr := SleepContext(ctx, BackoffDelay(attempt, baseDelay, maxDelay, retryAfter)); sErr != nil {
					return ResponseResult{}, sErr
				}
			}
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		hdr := resp.Header.Clone()
		status := resp.StatusCode
		resp.Body.Close()

		if readErr != nil {
			lastErr = readErr
			if attempt+1 < attempts {
				if sErr := SleepContext(ctx, BackoffDelay(attempt, baseDelay, maxDelay, "")); sErr != nil {
					return ResponseResult{}, sErr
				}
			}
			continue
		}
		return ResponseResult{
			StatusCode: status,
			Header:     hdr,
			Body:       body,
		}, nil
	}

	if lastErr != nil {
		return ResponseResult{}, lastErr
	}
	return ResponseResult{}, errors.New("httpx: request failed after retries")
}

var defaultGetFlights FlightGroup[ResponseResult]

// GetBytes performs a rate-limited, retried, singleflight-deduplicated HTTP GET request.
func GetBytes(
	ctx context.Context,
	client *http.Client,
	limiter *DomainLimiter,
	flights *FlightGroup[ResponseResult],
	rawURL string,
	headers map[string]string,
	cfg RetryConfig,
) (ResponseResult, error) {
	fg := flights
	if fg == nil {
		fg = &defaultGetFlights
	}
	flightKey := "GET:" + rawURL
	if len(headers) > 0 {
		if ref, ok := headers["Referer"]; ok && ref != "" {
			flightKey += "|ref=" + ref
		}
	}
	return fg.Do(ctx, flightKey, func(callCtx context.Context) (ResponseResult, error) {
		return DoWithRetry(callCtx, client, limiter, cfg, func(rCtx context.Context) (*http.Request, error) {
			req, err := http.NewRequestWithContext(rCtx, http.MethodGet, rawURL, nil)
			if err != nil {
				return nil, err
			}
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			return req, nil
		})
	})
}
