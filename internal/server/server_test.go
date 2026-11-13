package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mila/internal/aniliberty"
	"mila/internal/anime"
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
