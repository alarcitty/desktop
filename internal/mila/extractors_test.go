package mila

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAnimeGOSearchAndPlayerParsing(t *testing.T) {
	searchHTML := `
	<div class="ani-grid__item">
	  <a href="/anime/top-250">Top 250</a>
	  <a class="d-block ani-grid__item-picture" href="/anime/provozhayuschaya-v-posledniy-put-friren-2430">
	    <img class="image__img rounded" src="https://img.cdngos.com/v/250x350/anime/frieren.jpg" alt="Frieren">
	  </a>
	  <div class="ani-grid__item-body pt-2">
	    <div class="fw-lighter small mb-1 text-line-clamp">Sousou no Frieren</div>
	    <div class="ani-grid__item-title h5"><a title="Провожающая в последний путь Фрирен" href="/anime/provozhayuschaya-v-posledniy-put-friren-2430">Провожающая в последний путь Фрирен</a></div>
	    <span class="ani-grid__item-genres__link">2023</span>
	  </div>
	</div>`

	playerAjax := `{"status":"success","data":{"content":"<button type=\"button\" data-anime-player-target=\"provider\" data-player=\"//kodikplayer.com/seria/123\" data-provider-slug=\"kodik\" data-translation-id=\"2\" data-translation-title=\"AniLibria\"></button><button type=\"button\" data-anime-player-target=\"provider\" data-player=\"//aniboom.one/embed/vy7q8mlM8bg?episode=1&amp;translation=1\" data-provider-slug=\"aniboom\" data-translation-id=\"1\" data-translation-title=\"Студийная Банда\"></button><button type=\"button\" data-anime-player-target=\"provider\" data-player=\"//aniboom.one/embed/vy7q8mlM8bg?episode=1&amp;translation=27\" data-provider-slug=\"aniboom\" data-translation-id=\"27\" data-translation-title=\"Субтитры\"></button>"}}`
	episodesAjax := `{"status":"success","data":{"content":"<div data-episode-number=\"2\" data-episode=\"5002\"></div><div data-episode-number=\"1\" data-episode=\"5001\"></div>"}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/search/anime":
			_, _ = w.Write([]byte(searchHTML))
		case r.URL.Path == "/player/2430":
			_, _ = w.Write([]byte(playerAjax))
		case r.URL.Path == "/player/2430/episodes":
			_, _ = w.Write([]byte(episodesAjax))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	items, err := searchAnimeGO(context.Background(), srv.URL, "frieren")
	if err != nil {
		t.Fatalf("searchAnimeGO error: %v", err)
	}
	if len(items) != 1 || items[0].ID != 2430 || items[0].Year != 2023 {
		t.Fatalf("unexpected search items: %+v", items)
	}
	if items[0].Title != "Провожающая в последний путь Фрирен" || items[0].OriginalTitle != "Sousou no Frieren" {
		t.Fatalf("unexpected titles: %+v", items[0])
	}

	res, err := fetchAnimeGOPlayer(context.Background(), srv.URL, 2430)
	if err != nil {
		t.Fatalf("fetchAnimeGOPlayer error: %v", err)
	}
	if len(res.Episodes) != 2 || res.Episodes[0].Ordinal != 1 || res.Episodes[1].Ordinal != 2 {
		t.Fatalf("expected sorted episodes [1, 2], got %+v", res.Episodes)
	}
	if len(res.Dubs) != 3 {
		t.Fatalf("expected 3 dubs (2 aniboom + 1 kodik), got %+v", res.Dubs)
	}
	if res.Dubs[0].ProviderSlug != "aniboom" || res.Dubs[0].Title != "Студийная Банда" || !strings.HasPrefix(res.Dubs[0].EmbedURL, "https://aniboom.one/embed/") {
		t.Fatalf("unexpected first aniboom dub entry: %+v", res.Dubs[0])
	}
	if res.Dubs[2].ProviderSlug != "kodik" || res.Dubs[2].Title != "AniLibria" || !strings.HasPrefix(res.Dubs[2].EmbedURL, "https://kodikplayer.com/seria/") {
		t.Fatalf("unexpected kodik dub entry: %+v", res.Dubs[2])
	}
}

func TestParseAniboomAndPlaylistRewrite(t *testing.T) {
	embedHTML := `<div id="video" data-parameters="{&quot;duration&quot;:1415,&quot;poster&quot;:&quot;https:\/\/i1.boom-img.com\/38\/p.jpg&quot;,&quot;qualityVideo&quot;:1080,&quot;hls&quot;:&quot;{\&quot;src\&quot;:\&quot;https:\\\/\\\/young.ya-low1.site\\\/38\\\/master.m3u8\&quot;,\&quot;type\&quot;:\&quot;application\\\/x-mpegURL\&quot;}&quot;,&quot;episodePicker&quot;:{&quot;translationTitle&quot;:&quot;Crunchyroll&quot;}}"></div>`
	resolved, err := parseAniboomEmbedHTML(embedHTML)
	if err != nil {
		t.Fatalf("parseAniboomEmbedHTML failed: %v", err)
	}
	if resolved.MasterURL != "https://young.ya-low1.site/38/master.m3u8" || resolved.QualityVideo != 1080 {
		t.Fatalf("unexpected resolved aniboom payload: %+v", resolved)
	}

	masterURL, _ := url.Parse(resolved.MasterURL)
	masterM3U8 := strings.Join([]string{
		"#EXTM3U",
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="group_A1",NAME="audio_1",DEFAULT=YES,URI="media_1.m3u8"`,
		`#EXT-X-MAP:URI="init_6.m4s"`,
		"#EXT-X-STREAM-INF:BANDWIDTH=2831122,RESOLUTION=1920x1080,AUDIO=\"group_A1\"",
		"media_6.m3u8",
	}, "\n")

	rewritten := rewriteStreamPlaylist(masterM3U8, masterURL, aniboomRefererURL)
	if !strings.Contains(rewritten, `URI="/ext/stream?ref=https%3A%2F%2Faniboom.one%2F&url=https%3A%2F%2Fyoung.ya-low1.site%2F38%2Fmedia_1.m3u8"`) {
		t.Fatalf("expectedrewritten EXT-X-MEDIA URI, got:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, `URI="/ext/stream?ref=https%3A%2F%2Faniboom.one%2F&url=https%3A%2F%2Fyoung.ya-low1.site%2F38%2Finit_6.m4s"`) {
		t.Fatalf("expected rewritten EXT-X-MAP URI, got:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "\n/ext/stream?ref=https%3A%2F%2Faniboom.one%2F&url=https%3A%2F%2Fyoung.ya-low1.site%2F38%2Fmedia_6.m3u8") {
		t.Fatalf("expected rewritten child playlist URL, got:\n%s", rewritten)
	}
}

func TestStreamValidationAndRangeProxy(t *testing.T) {
	if _, err := validatePublicStreamURL("https://young.ya-low1.site/38/master.m3u8"); err != nil {
		t.Fatalf("expected public URL allowed, got err=%v", err)
	}
	for _, blocked := range []string{"http://127.0.0.1:8080/a", "http://localhost/test", "http://192.168.1.5/x", "file:///c:/windows/win.ini"} {
		if _, err := validatePublicStreamURL(blocked); err == nil {
			t.Fatalf("expected blocked private/local URL: %s", blocked)
		}
	}

	var gotRef, gotRange string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRef = r.Header.Get("Referer")
		gotRange = r.Header.Get("Range")
		if r.URL.Path == "/seg.m4s" {
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Range", "bytes 0-3/8")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("ABCD"))
			return
		}
		http.NotFound(w, r)
	}))
	defer up.Close()

	req := httptest.NewRequest(http.MethodGet, buildStreamProxyURL(up.URL+"/seg.m4s", aniboomRefererURL), nil)
	req.Header.Set("Range", "bytes=0-3")
	rec := httptest.NewRecorder()
	serveStreamProxy(rec, req, true)

	if rec.Code != http.StatusPartialContent || rec.Body.String() != "ABCD" {
		t.Fatalf("unexpected range proxy response: %d %q", rec.Code, rec.Body.String())
	}
	if gotRef != aniboomRefererURL || gotRange != "bytes=0-3" {
		t.Fatalf("expected headers Referer=%q Range=%q, got Referer=%q Range=%q", aniboomRefererURL, "bytes=0-3", gotRef, gotRange)
	}
}
