package aniliberty

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestLatestAndReleaseResolveRelativeMedia(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/anime/releases/latest":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":    10325,
					"alias": "psyren",
					"year":  2026,
					"name":  map[string]any{"main": "Псайрен", "english": "Psyren"},
					"shikimori": map[string]any{
						"id":     63098,
						"url":    "https://shikimori.io/animes/63098",
						"rating": 7.57,
					},
					"poster": map[string]any{
						"src": "/storage/releases/posters/10325/p.jpg",
						"optimized": map[string]any{
							"src": "/storage/releases/posters/10325/p.webp",
						},
					},
				},
			})
		case "/api/v1/anime/releases/psyren":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    10325,
				"alias": "psyren",
				"name":  map[string]any{"main": "Псайрен"},
				"episodes": []map[string]any{
					{
						"id":       "ep-1",
						"ordinal":  1,
						"duration": 1420,
						"hls_1080": "https://cache.libria.fun/1080.m3u8",
						"preview":  map[string]any{"src": "/storage/ep1.jpg"},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL + "/api/v1"})

	latest, err := client.Latest(context.Background(), 1)
	if err != nil {
		t.Fatalf("Latest failed: %v", err)
	}
	if len(latest) != 1 || latest[0].Poster.Src != srv.URL+"/storage/releases/posters/10325/p.jpg" {
		t.Fatalf("unexpected latest response: %+v", latest)
	}

	rel, err := client.Release(context.Background(), "psyren")
	if err != nil {
		t.Fatalf("Release failed: %v", err)
	}
	if len(rel.Episodes) != 1 || rel.Episodes[0].Preview.Src != srv.URL+"/storage/ep1.jpg" {
		t.Fatalf("unexpected release episodes: %+v", rel.Episodes)
	}

	if _, err := client.Release(context.Background(), "missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAniLibertyDiskCacheSurvivesClientRestart(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/anime/schedule/now":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"today": []map[string]any{
					{"release": map[string]any{"id": 10325, "alias": "psyren", "name": map[string]any{"main": "Псайрен"}}},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	c1 := New(Config{BaseURL: srv.URL + "/api/v1", CacheDir: cacheDir})
	sched1, err := c1.ScheduleNow(context.Background())
	if err != nil {
		t.Fatalf("ScheduleNow failed: %v", err)
	}
	if len(sched1.Today) != 1 || hits.Load() != 1 {
		t.Fatalf("unexpected initial schedule state: len=%d hits=%d", len(sched1.Today), hits.Load())
	}

	c2 := New(Config{BaseURL: srv.URL + "/api/v1", CacheDir: cacheDir})
	sched2, err := c2.ScheduleNow(context.Background())
	if err != nil {
		t.Fatalf("recreated ScheduleNow failed: %v", err)
	}
	if len(sched2.Today) != 1 || sched2.Today[0].Release.Alias != "psyren" {
		t.Fatalf("unexpected L2 schedule data: %+v", sched2.Today)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 0 upstream hits on L2 disk cache hit, got total %d", hits.Load())
	}
}
