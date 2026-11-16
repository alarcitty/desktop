package mila

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHentaiMamaSearchAndResolveFlow(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" && r.URL.Query().Get("s") == "boku kanojo":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`
<div id="dt-search-grid" class="dt-series-cards">
  <article class="series-card">
    <div class="sc-left">
      <a class="sc-poster" href="` + srv.URL + `/tvshows/boku-dake-no-hentai-kanojo-the-animation/">
        <img src="https://cdn.example/poster.webp" alt="Boku Dake no Hentai Kanojo The Animation">
      </a>
    </div>
    <div class="sc-body">
      <h3 class="sc-title"><a href="` + srv.URL + `/tvshows/boku-dake-no-hentai-kanojo-the-animation/">Boku Dake no Hentai Kanojo The Animation</a></h3>
      <div class="sc-alt">Boku Dake no Hentai Kanojo</div>
      <div class="sc-meta"><span class="sc-tag">2017</span><span class="sc-tag">2 Episodes</span></div>
    </div>
  </article>
</div>`))
		case r.URL.Path == "/tvshows/boku-dake-no-hentai-kanojo-the-animation/":
			_, _ = w.Write([]byte(`
<a href="` + srv.URL + `/episodes/boku-dake-no-hentai-kanojo-episode-2/">Ep 2</a>
<a href="` + srv.URL + `/episodes/boku-dake-no-hentai-kanojo-episode-1/">Ep 1</a>`))
		case r.URL.Path == "/episodes/boku-dake-no-hentai-kanojo-episode-1/":
			_, _ = w.Write([]byte(`<script>var dtHist = {"ajaxurl":"` + srv.URL + `/wp-admin/admin-ajax.php","episode":"776","duration":"1056"};</script>`))
		case r.URL.Path == "/episodes/boku-dake-no-hentai-kanojo-episode-2/":
			_, _ = w.Write([]byte(`<script>var dtHist = {"ajaxurl":"` + srv.URL + `/wp-admin/admin-ajax.php","episode":"777","duration":"1020"};</script>`))
		case r.URL.Path == "/wp-admin/admin-ajax.php":
			_ = r.ParseForm()
			epID := r.FormValue("a")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]string{
				`<iframe src="` + srv.URL + `/?dt_embed=hls&#038;ep=` + epID + `"></iframe>`,
				`<iframe src="` + srv.URL + `/?dt_embed=r2-mp4&#038;ep=` + epID + `"></iframe>`,
			})
		case r.URL.Path == "/" && r.URL.Query().Get("dt_embed") == "hls":
			epID := r.URL.Query().Get("ep")
			_, _ = w.Write([]byte(`<script>jwplayer("p").setup({sources: [{"file":"https:\/\/hentaidoge.org\/s\/tok\/ep` + epID + `\/master.m3u8","type":"hls"}]});</script>`))
		case r.URL.Path == "/" && r.URL.Query().Get("dt_embed") == "r2-mp4":
			epID := r.URL.Query().Get("ep")
			_, _ = w.Write([]byte(`<script>jwplayer("p").setup({sources: [{"file":"https:\/\/gdvid.info\/hls\/ep` + epID + `\/1080p.mp4","label":"1080p","type":"mp4"},{"file":"https:\/\/gdvid.info\/hls\/ep` + epID + `\/720p.mp4","label":"720p","type":"mp4"}]});</script>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	items, err := searchHentaiMama(context.Background(), srv.URL, "boku kanojo")
	if err != nil {
		t.Fatalf("searchHentaiMama failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 show, got %d: %+v", len(items), items)
	}
	if items[0].Year != 2017 || items[0].Provider != "hentaimama" || items[0].OriginalTitle != "Boku Dake no Hentai Kanojo" {
		t.Fatalf("unexpected search item: %+v", items[0])
	}

	episodes, err := resolveHentaiEpisodes(context.Background(), "hentaimama", items[0].Target, false)
	if err != nil {
		t.Fatalf("resolveHentaiEpisodes failed: %v", err)
	}
	if len(episodes) != 2 {
		t.Fatalf("expected 2 episodes, got %d", len(episodes))
	}
	if episodes[0].Ordinal != 1 || episodes[1].Ordinal != 2 {
		t.Fatalf("expected sorted ordinals 1,2; got %d,%d", episodes[0].Ordinal, episodes[1].Ordinal)
	}
	if episodes[0].Duration != 1056 || !strings.Contains(episodes[0].HLS1080, "/ext/stream?") || !strings.Contains(episodes[0].HLS1080, "ep776") {
		t.Fatalf("unexpected ep1 HLS/duration: %+v", episodes[0])
	}
	if !strings.Contains(episodes[0].MP41080, "1080p.mp4") || !strings.Contains(episodes[0].MP4720, "720p.mp4") {
		t.Fatalf("expected MP4 qualities, got: %+v", episodes[0])
	}
}

func TestHentaiGasmSearchGroupingAndSpaceEncoding(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" && r.URL.Query().Get("s") == "boku kanojo":
			_, _ = w.Write([]byte(`
<div id="post-3383" class="item cf item-post category-boku-kanojo-uncensored">
  <div class="thumb"><a class="clip-link" title="Boku Kanojo Uncensored 2 Subbed" href="` + srv.URL + `/ep-unc-2/"><img src="https://cdn.example/t2.jpg"></a></div>
</div>
<div id="post-3321" class="item cf item-post category-boku-kanojo-uncensored">
  <div class="thumb"><a class="clip-link" title="Boku Kanojo Uncensored 1 Subbed" href="` + srv.URL + `/ep-unc-1/"><img src="https://cdn.example/t1.jpg"></a></div>
</div>
<div id="post-1638" class="item cf item-post category-boku-kanojo">
  <div class="thumb"><a class="clip-link" title="Boku Kanojo 1 Subbed" href="` + srv.URL + `/ep-sub-1/"><img src="https://cdn.example/s1.jpg"></a></div>
</div>`))
		case r.URL.Path == "/ep-unc-1/":
			_, _ = w.Write([]byte(`<script>jwplayer().setup({file: "https://hgasm2.com/Boku Kanojo Uncensored 1.mp4"});</script>`))
		case r.URL.Path == "/ep-unc-2/":
			_, _ = w.Write([]byte(`<script>jwplayer().setup({file: "https://hgasm2.com/Boku Kanojo Uncensored 2.mp4"});</script>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	items, err := searchHentaiGasm(context.Background(), srv.URL, "boku kanojo")
	if err != nil {
		t.Fatalf("searchHentaiGasm failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 grouped variants (Uncensored & Subbed), got %d: %+v", len(items), items)
	}
	if items[0].Variant != "Uncensored" || items[0].EpisodeCount != 2 {
		t.Fatalf("expected Uncensored group with 2 episodes, got %+v", items[0])
	}

	episodes, err := resolveHentaiEpisodes(context.Background(), "hentaigasm", items[0].Target, false)
	if err != nil {
		t.Fatalf("resolveHentaiEpisodes(hentaigasm) failed: %v", err)
	}
	if len(episodes) != 2 || episodes[0].Ordinal != 1 || episodes[1].Ordinal != 2 {
		t.Fatalf("expected sorted episodes 1,2; got %+v", episodes)
	}
	if !strings.Contains(episodes[0].MP4720, "/ext/stream?") || !strings.Contains(episodes[0].MP4720, "Boku%2520Kanojo") {
		t.Fatalf("expected encoded space %%20 inside proxied URL, got %s", episodes[0].MP4720)
	}
}

func TestHentaiStrictHostValidation(t *testing.T) {
	if err := validateAllowedProviderURL("https://evil.example/tvshows/x/", "hentaimama.io"); err == nil {
		t.Fatal("expected non-allowlisted host to be rejected")
	}
	if err := validateAllowedProviderURL("https://hentaimama.io/tvshows/boku/", "hentaimama.io"); err != nil {
		t.Fatalf("expected hentaimama.io to pass, got %v", err)
	}
}
