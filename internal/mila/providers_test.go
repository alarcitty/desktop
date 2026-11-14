package mila

import (
	"io"
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

func TestProviderProxyHandlerAndM3U8Rewrite(t *testing.T) {
	var upstreamURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1}`))
		case "/hls/test/master.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=3000000\n" + upstreamURL + "/hls/test/1080.m3u8\n"))
		case "/hls/test/seg-1.m4s":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("MP4CHUNK"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	upstreamURL = upstream.URL

	handler := newProviderProxyHandler([]providerRoute{
		{prefix: "/ext/dreamerscast", upstream: upstream.URL},
		{prefix: "/ext/play-dc", upstream: upstream.URL, rewriteM3U8: true},
	})

	req := httptest.NewRequest(http.MethodPost, "/ext/dreamerscast/", strings.NewReader("search=Dandadan"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"count":1`) {
		t.Fatalf("unexpected search proxy response: %d %s", rec.Code, rec.Body.String())
	}

	m3u8Req := httptest.NewRequest(http.MethodGet, "/ext/play-dc/hls/test/master.m3u8", nil)
	m3u8Rec := httptest.NewRecorder()
	handler.ServeHTTP(m3u8Rec, m3u8Req)
	body := m3u8Rec.Body.String()
	if m3u8Rec.Code != http.StatusOK || !strings.Contains(body, "/ext/play-dc/hls/test/1080.m3u8") {
		t.Fatalf("expected rewritten m3u8 URL, got: %d %s", m3u8Rec.Code, body)
	}
	if strings.Contains(body, upstream.URL) {
		t.Fatalf("expected upstream origin stripped from m3u8, got: %s", body)
	}

	segReq := httptest.NewRequest(http.MethodGet, "/ext/play-dc/hls/test/seg-1.m4s", nil)
	segRec := httptest.NewRecorder()
	handler.ServeHTTP(segRec, segReq)
	segBytes, _ := io.ReadAll(segRec.Result().Body)
	if segRec.Code != http.StatusOK || string(segBytes) != "MP4CHUNK" {
		t.Fatalf("unexpected segment response: %d %q", segRec.Code, string(segBytes))
	}

	badReq := httptest.NewRequest(http.MethodGet, "/ext/play-dc//evil.com/test.m3u8", nil)
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on double slash path, got %d", badRec.Code)
	}

	anilibriaHandler := newProviderProxyHandler([]providerRoute{
		{prefix: "/ext/anilibria", upstream: upstream.URL},
	})
	aReq := httptest.NewRequest(http.MethodGet, "/ext/anilibria/", nil)
	aRec := httptest.NewRecorder()
	anilibriaHandler.ServeHTTP(aRec, aReq)
	if aRec.Code != http.StatusOK || !strings.Contains(aRec.Body.String(), `"count":1`) {
		t.Fatalf("unexpected anilibria proxy response: %d %s", aRec.Code, aRec.Body.String())
	}
}
