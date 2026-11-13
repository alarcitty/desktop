package aniliberty

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
