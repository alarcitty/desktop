package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoWithRetryHonorsRetryAfterAnd429(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if n == 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	res, err := DoWithRetry(
		context.Background(),
		srv.Client(),
		nil,
		RetryConfig{MaxAttempts: 4, BaseDelay: 5 * time.Millisecond, MaxDelay: 20 * time.Millisecond},
		func(ctx context.Context) (*http.Request, error) {
			return http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/test", nil)
		},
	)
	if err != nil {
		t.Fatalf("expected eventual success, got error: %v", err)
	}
	if res.StatusCode != http.StatusOK || string(res.Body) != `{"ok":true}` {
		t.Fatalf("unexpected response: %+v", res)
	}
	if hits.Load() != 3 {
		t.Fatalf("expected exactly 3 upstream attempts (429 -> 503 -> 200), got %d", hits.Load())
	}
}

func TestDoWithRetryStopsOnContextCancel(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
	defer cancel()

	_, err := DoWithRetry(
		ctx,
		srv.Client(),
		nil,
		RetryConfig{MaxAttempts: 5, BaseDelay: 200 * time.Millisecond, MaxDelay: 5 * time.Second},
		func(rCtx context.Context) (*http.Request, error) {
			return http.NewRequestWithContext(rCtx, http.MethodGet, srv.URL, nil)
		},
	)
	if err == nil {
		t.Fatalf("expected context deadline cancellation error")
	}
	if hits.Load() != 1 {
		t.Fatalf("expected cancellation before retry sleep finished, got %d hits", hits.Load())
	}
}

func TestGetBytesSingleflightDeduplication(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(25 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("deduped-body"))
	}))
	defer srv.Close()

	var flights FlightGroup[ResponseResult]
	const workers = 12
	var wg sync.WaitGroup
	wg.Add(workers)
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			res, err := GetBytes(
				context.Background(),
				srv.Client(),
				nil,
				&flights,
				srv.URL+"/item",
				nil,
				RetryConfig{MaxAttempts: 2},
			)
			if err != nil {
				errs <- err
				return
			}
			if string(res.Body) != "deduped-body" {
				errs <- context.DeadlineExceeded
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("unexpected concurrent error: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 upstream call across %d concurrent callers, got %d", workers, hits.Load())
	}
}

func TestDomainLimiterRateSpacing(t *testing.T) {
	lim := NewDomainLimiter(HostRule{RatePerSec: 20, Burst: 1}, nil)
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := lim.Wait(context.Background(), "https://example.org/api"); err != nil {
			t.Fatalf("wait failed: %v", err)
		}
	}
	elapsed := time.Since(start)
	if elapsed < 75*time.Millisecond {
		t.Fatalf("expected token bucket spacing >=75ms for 3 calls at 20 rps (burst 1), got %v", elapsed)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	if d, ok := ParseRetryAfter("4", now); !ok || d != 4*time.Second {
		t.Fatalf("expected 4s, got %v (ok=%v)", d, ok)
	}
	httpDate := now.Add(3 * time.Second).Format(http.TimeFormat)
	if d, ok := ParseRetryAfter(httpDate, now); !ok || d != 3*time.Second {
		t.Fatalf("expected 3s from HTTP date, got %v (ok=%v)", d, ok)
	}
}
