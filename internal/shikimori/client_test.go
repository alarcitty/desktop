package shikimori

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCleanDescription(t *testing.T) {
	raw := "Встреча [character=10478]Агэхи[/character] и [url=https://example.com]Сакурако[/url].[br]Конец."
	got := CleanDescription(raw)
	want := "Встреча Агэхи и Сакурако.\nКонец."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGetByIDsBatchesCachesAndRetries(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Errorf("expected non-empty User-Agent")
		}
		n := hits.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"animes": []map[string]any{
					{
						"id":          "63098",
						"malId":       "63098",
						"name":        "Psyren",
						"russian":     "Псайрен",
						"description": "[b]Тест[/b]",
						"score":       7.5,
						"poster": map[string]any{
							"id":          "1",
							"originalUrl": "/uploads/poster.jpg",
							"main2xUrl":   "/uploads/poster_2x.webp",
						},
						"screenshots": []map[string]any{
							{"id": "10", "originalUrl": "/uploads/shot1.jpg", "x332Url": "/uploads/shot1_332.jpg"},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := New(Config{
		BaseURL:     srv.URL,
		MinInterval: time.Millisecond,
	})

	res, err := client.GetByIDs(context.Background(), []int{63098, 63098})
	if err != nil {
		t.Fatalf("GetByIDs failed: %v", err)
	}
	a, ok := res[63098]
	if !ok {
		t.Fatalf("expected id 63098 in result")
	}
	if a.Description != "Тест" {
		t.Fatalf("expected cleaned description, got %q", a.Description)
	}
	if a.Poster == nil || a.Poster.OriginalURL != srv.URL+"/uploads/poster.jpg" {
		t.Fatalf("expected resolved poster URL, got %+v", a.Poster)
	}
	if len(a.Screenshots) != 1 || a.Screenshots[0].OriginalURL != srv.URL+"/uploads/shot1.jpg" {
		t.Fatalf("expected resolved screenshot URL, got %+v", a.Screenshots)
	}

	// Second call should hit cache without touching server.
	before := hits.Load()
	if _, err := client.GetByID(context.Background(), 63098); err != nil {
		t.Fatalf("cached GetByID failed: %v", err)
	}
	if hits.Load() != before {
		t.Fatalf("expected cache hit on second GetByID call")
	}
}
