package anime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mila/internal/aniliberty"
	"mila/internal/shikimori"
)

func TestServiceMergesAniLibertyAndShikimori(t *testing.T) {
	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":          10325,
				"alias":       "psyren",
				"year":        2026,
				"name":        map[string]any{"main": "Псайрен", "english": "Psyren"},
				"description": "Короткое описание AniLiberty",
				"shikimori":   map[string]any{"id": 63098, "rating": 7.4},
				"poster":      map[string]any{"src": "/storage/lib_poster.jpg"},
				"genres":      []map[string]any{{"id": 14, "name": "Экшен"}},
			},
		})
	}))
	defer libSrv.Close()

	shikiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"animes": []map[string]any{
					{
						"id":          "63098",
						"malId":       "63098",
						"name":        "Psyren",
						"russian":     "Псайрен",
						"description": "[character=1]Агэха[/character] — описание Shikimori",
						"score":       7.85,
						"poster": map[string]any{
							"originalUrl": "https://shikimori.io/poster_orig.jpg",
							"main2xUrl":   "https://shikimori.io/poster_2x.webp",
						},
						"screenshots": []map[string]any{
							{"originalUrl": "https://shikimori.io/banner_1.jpg", "x332Url": "https://shikimori.io/banner_332.jpg"},
						},
						"studios": []map[string]any{{"id": "1", "name": "Satelight"}},
					},
				},
			},
		})
	}))
	defer shikiSrv.Close()

	svc := NewService(
		aniliberty.New(aniliberty.Config{BaseURL: libSrv.URL}),
		shikimori.New(shikimori.Config{BaseURL: shikiSrv.URL, MinInterval: time.Millisecond}),
	)

	items, err := svc.Latest(context.Background(), 5)
	if err != nil {
		t.Fatalf("Latest failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	got := items[0]
	if got.Description != "Агэха — описание Shikimori" {
		t.Fatalf("expected cleaned shikimori description, got %q", got.Description)
	}
	if got.Banner != "https://shikimori.io/banner_1.jpg" {
		t.Fatalf("expected shikimori screenshot banner, got %q", got.Banner)
	}
	if got.Poster.Original != "https://shikimori.io/poster_orig.jpg" {
		t.Fatalf("expected shikimori poster, got %q", got.Poster.Original)
	}
	if len(got.Studios) != 1 || got.Studios[0] != "Satelight" {
		t.Fatalf("expected studio Satelight, got %v", got.Studios)
	}
}

func TestServiceGracefullyFallsBackWhenShikimoriUnavailable(t *testing.T) {
	libSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":          10325,
				"alias":       "psyren",
				"name":        map[string]any{"main": "Псайрен"},
				"description": "AniLiberty fallback",
				"shikimori":   map[string]any{"id": 63098, "rating": 7.4},
				"poster":      map[string]any{"src": "/storage/lib_poster.jpg"},
			},
		})
	}))
	defer libSrv.Close()

	shikiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer shikiSrv.Close()

	svc := NewService(
		aniliberty.New(aniliberty.Config{BaseURL: libSrv.URL}),
		shikimori.New(shikimori.Config{BaseURL: shikiSrv.URL, MinInterval: time.Millisecond}),
	)

	items, err := svc.Latest(context.Background(), 5)
	if err != nil {
		t.Fatalf("expected graceful fallback, got error: %v", err)
	}
	if len(items) != 1 || items[0].Description != "AniLiberty fallback" {
		t.Fatalf("expected fallback AniLiberty description, got %+v", items)
	}
	if items[0].Banner != libSrv.URL+"/storage/lib_poster.jpg" {
		t.Fatalf("expected fallback poster banner, got %q", items[0].Banner)
	}
}
