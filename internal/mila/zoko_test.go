package mila

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func encryptZokoTestBlob(plainJSON string) string {
	key := []byte(zokoXORKey)
	raw := []byte(plainJSON)
	xored := make([]byte, len(raw))
	for i, b := range raw {
		xored[i] = b ^ key[i%len(key)]
	}
	return base64.StdEncoding.EncodeToString(xored)
}

func TestZokoDeobfuscateAndResolve(t *testing.T) {
	blob := encryptZokoTestBlob(`{"src":"https://hls.dramahot.top/p/abc/master.m3u8","subtitles":[{"lang":"en","label":"English","default":true,"src":"https://hls.dramahot.top/v/abc/subs/en.vtt"}]}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stream/mal/52991/1/dub":
			_, _ = w.Write([]byte(`<script>window.__P="` + blob + `";</script>`))
		case "/stream/mal/52991/1/sub":
			_, _ = w.Write([]byte(`<script>window.__P="` + blob + `";</script>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	resolved, err := resolveZokoEpisode(context.Background(), srv.URL, 52991, 1, "dub")
	if err != nil {
		t.Fatalf("resolveZokoEpisode failed: %v", err)
	}
	if !strings.HasSuffix(resolved.MasterURL, "/master.m3u8") || len(resolved.Subtitles) != 1 {
		t.Fatalf("unexpected resolved payload: %+v", resolved)
	}

	probe, err := probeZokoRelease(context.Background(), srv.URL, srv.URL, 52991, 28, "Frieren")
	if err != nil {
		t.Fatalf("probeZokoRelease failed: %v", err)
	}
	if !probe.HasDub || !probe.HasSub || probe.MalID != 52991 || len(probe.Episodes) != 28 {
		t.Fatalf("unexpected probe result: %+v", probe)
	}
}
