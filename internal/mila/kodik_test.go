package mila

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func encodeKodikTestURL(rawURL string, decodeShift int) string {
	b64 := strings.TrimRight(base64.StdEncoding.EncodeToString([]byte(rawURL)), "=")
	encodeShift := (26 - ((decodeShift%26)+26)%26) % 26
	return caesarShiftASCII(b64, encodeShift)
}

func TestExtractKodikScriptConfigDynamic(t *testing.T) {
	js18 := `$.ajax({type:"POST",url:atob("L2Z0b3I="),data:e});var y=atob(t.replace(/[a-zA-Z]/g,function(e){return String.fromCharCode((e<="Z"?90:122)>=(e=e.charCodeAt(0)+18)?e:e-26)}));`
	cfg18 := extractKodikScriptConfig(js18)
	if cfg18.endpoint != "/ftor" || cfg18.shift != 18 {
		t.Fatalf("expected /ftor + shift 18, got %+v", cfg18)
	}

	js13 := `$.ajax({type:"POST",url:atob("L2d2aTI="),data:e});var z=atob(t.replace(/[a-zA-Z]/g,function(e){return String.fromCharCode((e<="Z"?90:122)>=(e=e.charCodeAt(0)+13)?e:e-26)}));`
	cfg13 := extractKodikScriptConfig(js13)
	if cfg13.endpoint != "/gvi2" || cfg13.shift != 13 {
		t.Fatalf("expected /gvi2 + shift 13, got %+v", cfg13)
	}
}

func TestDecodeKodikLinkSelfHealing(t *testing.T) {
	target720 := "https://p14.solodcdn.com/s/m/abc/720.mp4:hls:manifest.m3u8"
	enc18 := encodeKodikTestURL(target720, 18)

	// Direct match with expected shift=18.
	got, usedShift, ok := decodeKodikLink(enc18, 18)
	if !ok || got != target720 || usedShift != 18 {
		t.Fatalf("expected %s with shift 18, got ok=%v shift=%d url=%s", target720, ok, usedShift, got)
	}

	// Self-healing: even with a stale shift=7, decoder automatically discovers shift=18.
	healed, discoveredShift, ok := decodeKodikLink(enc18, 7)
	if !ok || healed != target720 || discoveredShift != 18 {
		t.Fatalf("expected self-healed shift 18 -> %s, got ok=%v shift=%d url=%s", target720, ok, discoveredShift, healed)
	}
}

func TestBuildKodikMasterPlaylist(t *testing.T) {
	playlist := buildKodikMasterPlaylist(map[string]string{
		"360": "https://p14.solodcdn.com/360.mp4:hls:manifest.m3u8",
		"720": "https://p14.solodcdn.com/720.mp4:hls:manifest.m3u8",
		"480": "https://p14.solodcdn.com/480.mp4:hls:manifest.m3u8",
	}, "https://kodikplayer.com/")

	idx720 := strings.Index(playlist, `NAME="720p"`)
	idx480 := strings.Index(playlist, `NAME="480p"`)
	idx360 := strings.Index(playlist, `NAME="360p"`)
	if idx720 < 0 || idx480 < 0 || idx360 < 0 {
		t.Fatalf("expected all 3 qualities in master playlist:\n%s", playlist)
	}
	if !(idx720 < idx480 && idx480 < idx360) {
		t.Fatalf("expected descending quality order 720p -> 480p -> 360p:\n%s", playlist)
	}
	if !strings.Contains(playlist, `/ext/stream?ref=https%3A%2F%2Fkodikplayer.com%2F&url=https%3A%2F%2Fp14.solodcdn.com%2F720.mp4%3Ahls%3Amanifest.m3u8`) {
		t.Fatalf("expected proxied 720p URL with kodik referer:\n%s", playlist)
	}
}

func TestParseKodikEmbedHTMLAndAnimeGOEpisodeFetch(t *testing.T) {
	embedHTML := `
	<script type="text/javascript">
	var domain = "yani.tv";
	var d_sign = "sigD";
	var pd = "kodikplayer.com";
	var pd_sign = "sigPD";
	var ref = "https://yani.tv/";
	var ref_sign = "sigRef";
	</script>
	<script src="/assets/js/app.player_single.deadbeef.js"></script>
	<script>
	var vInfo = {};
	vInfo.type = 'seria';
	vInfo.hash = 'abc123';
	vInfo.id = '99999';
	</script>`
	params, err := parseKodikEmbedHTML(embedHTML)
	if err != nil {
		t.Fatalf("parseKodikEmbedHTML error: %v", err)
	}
	if params.domain != "yani.tv" || params.ref != "https://yani.tv/" || params.vID != "99999" || params.scriptPath != "/assets/js/app.player_single.deadbeef.js" {
		t.Fatalf("unexpected embed params: %+v", params)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/player/1619" {
			_, _ = w.Write([]byte(`{"status":"success","data":{"content":"<button type=\"button\" data-anime-player-target=\"provider\" data-player=\"//kodikplayer.com/seria/723282/f1f30aa/720p\" data-provider-slug=\"kodik\" data-translation-id=\"1\" data-translation-title=\"AniDUB\"></button>"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	gotEmbed, err := fetchAnimeGOKodikEmbedForEpisode(context.Background(), srv.URL, 1619, "1")
	if err != nil {
		t.Fatalf("fetchAnimeGOKodikEmbedForEpisode error: %v", err)
	}
	if gotEmbed != "https://kodikplayer.com/seria/723282/f1f30aa/720p" {
		t.Fatalf("unexpected animego kodik embed: %s", gotEmbed)
	}
}
