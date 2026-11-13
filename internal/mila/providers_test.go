package mila

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPostProviderFormAndGuards(t *testing.T) {
	var gotMethod string
	var gotContentType string
	var gotName string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		_ = r.ParseForm()
		gotName = r.PostForm.Get("name")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":3236}]}`))
	}))
	defer srv.Close()

	params := url.Values{}
	params.Set("name", "Дандадан")
	body, err := postProviderForm(srv.URL, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Fatalf("unexpected Content-Type: %s", gotContentType)
	}
	if gotName != "Дандадан" {
		t.Fatalf("expected form field name, got %q", gotName)
	}
	if !strings.Contains(body, `"id":3236`) {
		t.Fatalf("unexpected response body: %s", body)
	}

	app := New()
	emptySearch, err := app.AnimeVostSearch("a")
	if err != nil || emptySearch != `{"data":[]}` {
		t.Fatalf("unexpected short query response: %q err=%v", emptySearch, err)
	}
	emptyPlaylist, err := app.AnimeVostPlaylist(0)
	if err != nil || emptyPlaylist != `[]` {
		t.Fatalf("unexpected invalid playlist id response: %q err=%v", emptyPlaylist, err)
	}
}
