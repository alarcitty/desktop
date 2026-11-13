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
}
