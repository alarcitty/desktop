package cloud

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientPinnedTLSAndJSONHelpers(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/ping":
			_, _ = w.Write([]byte(`{"ok":true,"service":"mila-cloud","version":"0.1.0","uptime_sec":3,"time":"2026-11-15T00:00:00Z"}`))
		case "/api/v1/echo":
			if r.Header.Get("Authorization") != "Bearer tok-123" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			_ = json.NewEncoder(w).Encode(map[string]string{"got": in["msg"]})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	certSum := sha256.Sum256(srv.Certificate().Raw)
	goodFP := hex.EncodeToString(certSum[:])

	client, err := NewClient(Config{
		BaseURL:    srv.URL,
		CertSHA256: goodFP,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	ping, err := client.Ping(t.Context())
	if err != nil || !ping.OK || ping.Service != "mila-cloud" {
		t.Fatalf("Ping failed: ping=%+v err=%v", ping, err)
	}

	client.SetToken("tok-123")
	var echoResp map[string]string
	if err := client.PostJSON(t.Context(), "/api/v1/echo", map[string]string{"msg": "hello"}, &echoResp); err != nil {
		t.Fatalf("PostJSON failed: %v", err)
	}
	if echoResp["got"] != "hello" {
		t.Fatalf("unexpected echo: %+v", echoResp)
	}

	var apiErr *APIError
	if err := client.GetJSON(t.Context(), "/missing", nil); !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Fatalf("expected 404 APIError, got %v", err)
	}

	badClient, err := NewClient(Config{
		BaseURL:    srv.URL,
		CertSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if _, err := badClient.Ping(t.Context()); err == nil {
		t.Fatalf("expected TLS pin mismatch error")
	}
}
