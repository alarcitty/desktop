package aniskip

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestGetSkipTimesPrefersExactOpEdAndCachesDisk(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/skip-times/21/5" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"found":      true,
			"statusCode": 200,
			"results": []map[string]any{
				{
					"skipType": "mixed-op",
					"interval": map[string]float64{"startTime": 10, "endTime": 90},
				},
				{
					"skipType": "op",
					"interval": map[string]float64{"startTime": 85.5, "endTime": 175.5},
				},
				{
					"skipType": "mixed-ed",
					"interval": map[string]float64{"startTime": 1300, "endTime": 1390},
				},
			},
		})
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	client := New(Config{
		BaseURL:  srv.URL,
		CacheDir: cacheDir,
	})

	// Concurrent first callers should deduplicate via singleflight.
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := client.GetSkipTimes(context.Background(), 21, 5)
			if err != nil {
				t.Errorf("GetSkipTimes error: %v", err)
				return
			}
			if !got.Found || got.Opening == nil || got.Ending == nil {
				t.Errorf("expected opening & ending, got %+v", got)
				return
			}
			if *got.Opening.Start != 85.5 || *got.Opening.Stop != 175.5 {
				t.Errorf("expected exact op 85.5..175.5, got %v..%v", *got.Opening.Start, *got.Opening.Stop)
			}
			if *got.Ending.Start != 1300 || *got.Ending.Stop != 1390 {
				t.Errorf("expected mixed-ed fallback 1300..1390, got %v..%v", *got.Ending.Start, *got.Ending.Stop)
			}
		}()
	}
	wg.Wait()

	if hits.Load() != 1 {
		t.Fatalf("expected 1 upstream call across concurrent requests, got %d", hits.Load())
	}

	// Second client pointing at the same CacheDir must hit L2 disk without calling upstream.
	client2 := New(Config{
		BaseURL:  srv.URL,
		CacheDir: cacheDir,
	})
	got2, err := client2.GetSkipTimes(context.Background(), 21, 5)
	if err != nil {
		t.Fatalf("second client GetSkipTimes error: %v", err)
	}
	if !got2.Found || got2.Opening == nil || *got2.Opening.Start != 85.5 {
		t.Fatalf("unexpected L2 cached payload: %+v", got2)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected disk cache hit without HTTP call, got %d hits", hits.Load())
	}
}

func TestGetSkipTimesNegativeCaches404(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"found":false,"results":[],"statusCode":404}`))
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL})
	res1, err := client.GetSkipTimes(context.Background(), 99999, 12)
	if err != nil {
		t.Fatalf("expected nil error on 404, got %v", err)
	}
	if res1.Found {
		t.Fatalf("expected Found=false on 404")
	}

	res2, err := client.GetSkipTimes(context.Background(), 99999, 12)
	if err != nil {
		t.Fatalf("second lookup error: %v", err)
	}
	if res2.Found {
		t.Fatalf("expected cached Found=false")
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 upstream 404 call after negative cache, got %d", hits.Load())
	}
}

func TestGetSkipTimesRetriesOn429WithRetryAfter(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"found":true,"statusCode":200,"results":[{"skipType":"op","interval":{"startTime":12,"endTime":102}}]}`))
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL})
	got, err := client.GetSkipTimes(context.Background(), 16498, 1)
	if err != nil {
		t.Fatalf("expected retry to recover, got %v", err)
	}
	if !got.Found || got.Opening == nil || *got.Opening.Stop != 102 {
		t.Fatalf("unexpected retry result: %+v", got)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected 2 attempts on 429 retry, got %d", hits.Load())
	}
}
