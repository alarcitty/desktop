package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPKCEAndAuthURL(t *testing.T) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		t.Fatalf("generatePKCE: %v", err)
	}
	if len(verifier) < 43 {
		t.Fatalf("verifier too short: %d", len(verifier))
	}
	expected := sha256.Sum256([]byte(verifier))
	if got := base64.RawURLEncoding.EncodeToString(expected[:]); got != challenge {
		t.Fatalf("challenge mismatch: got %q want %q", challenge, got)
	}

	raw := buildGoogleAuthURL(googleAuthEndpoint, "cid-123", "http://127.0.0.1:9999/callback", "state-xyz", challenge)
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	q := parsed.Query()
	if q.Get("client_id") != "cid-123" {
		t.Fatalf("unexpected client_id: %q", q.Get("client_id"))
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Fatalf("unexpected method: %q", q.Get("code_challenge_method"))
	}
	if q.Get("state") != "state-xyz" {
		t.Fatalf("unexpected state: %q", q.Get("state"))
	}
}

func TestSessionFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cloud_session.json")
	want := &CloudSession{
		Token:     "tok-abc",
		ExpiresAt: 1799999999,
		User: CloudUser{
			ID:        1,
			Email:     "mila@example.com",
			Name:      "Mila User",
			AvatarURL: "https://example.com/a.png",
		},
	}
	if err := SaveSessionTo(path, want); err != nil {
		t.Fatalf("SaveSessionTo: %v", err)
	}
	got, err := LoadSessionFrom(path)
	if err != nil {
		t.Fatalf("LoadSessionFrom: %v", err)
	}
	if got.Token != want.Token || got.User.Email != want.User.Email || got.User.Name != want.User.Name {
		t.Fatalf("loaded mismatch: got %+v want %+v", got, want)
	}
	if err := ClearSessionAt(path); err != nil {
		t.Fatalf("ClearSessionAt: %v", err)
	}
	if _, err := LoadSessionFrom(path); err == nil {
		t.Fatal("expected error loading cleared session")
	}
}

func TestLoginWithGoogleLoopbackFlow(t *testing.T) {
	const issuedToken = "session-token-42"

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/google":
			var req struct {
				Code         string `json:"code"`
				CodeVerifier string `json:"code_verifier"`
				RedirectURI  string `json:"redirect_uri"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req.Code != "auth-code-99" || req.CodeVerifier == "" || !strings.HasPrefix(req.RedirectURI, "http://127.0.0.1:") {
				http.Error(w, "invalid oauth exchange payload", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(CloudSession{
				Token:     issuedToken,
				ExpiresAt: time.Now().Add(time.Hour).Unix(),
				User: CloudUser{
					ID:        100,
					Email:     "test@gmail.com",
					Name:      "Test User",
					AvatarURL: "https://lh3.googleusercontent.com/avatar.png",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/me":
			if r.Header.Get("Authorization") != "Bearer "+issuedToken {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(meResponse{
				ExpiresAt: time.Now().Add(time.Hour).Unix(),
				User: CloudUser{
					ID:    100,
					Email: "test@gmail.com",
					Name:  "Test User",
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/logout":
			if r.Header.Get("Authorization") != "Bearer "+issuedToken {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	sum := sha256.Sum256(srv.Certificate().Raw)
	client, err := NewClient(Config{
		BaseURL:    srv.URL,
		CertSHA256: hex.EncodeToString(sum[:]),
		Timeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	sessionPath := filepath.Join(t.TempDir(), "cloud_session.json")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sess, err := client.loginWithGoogleConfig(ctx, googleAuthEndpoint, "cid-test", sessionPath, func(authURL string) {
		go func() {
			parsed, parseErr := url.Parse(authURL)
			if parseErr != nil {
				return
			}
			q := parsed.Query()
			cbURL := fmtCallbackURL(q.Get("redirect_uri"), "auth-code-99", q.Get("state"))
			resp, reqErr := http.Get(cbURL)
			if reqErr == nil {
				_ = resp.Body.Close()
			}
		}()
	})
	if err != nil {
		t.Fatalf("loginWithGoogleConfig: %v", err)
	}
	if sess.Token != issuedToken || sess.User.Email != "test@gmail.com" {
		t.Fatalf("unexpected session: %+v", sess)
	}

	saved, err := LoadSessionFrom(sessionPath)
	if err != nil {
		t.Fatalf("LoadSessionFrom: %v", err)
	}
	if saved.Token != issuedToken {
		t.Fatalf("saved token mismatch: got %q", saved.Token)
	}

	me, err := client.FetchMe(ctx)
	if err != nil {
		t.Fatalf("FetchMe: %v", err)
	}
	if me.User.ID != 100 {
		t.Fatalf("unexpected me user: %+v", me.User)
	}

	if err := client.Logout(ctx); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := client.FetchMe(ctx); err == nil {
		t.Fatal("expected FetchMe to fail after logout clears token")
	}
}

func fmtCallbackURL(redirectURI, code, state string) string {
	u, _ := url.Parse(redirectURI)
	q := u.Query()
	q.Set("code", code)
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String()
}
