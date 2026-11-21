package torrent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseNyaaRSS(t *testing.T) {
	rss := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:nyaa="https://nyaa.si/xmlns/nyaa">
  <channel>
    <item>
      <title>[SubsPlease] Sousou no Frieren - 01 (1080p) [ABC123].mkv</title>
      <nyaa:seeders>128</nyaa:seeders>
      <nyaa:leechers>9</nyaa:leechers>
      <nyaa:infoHash>EF3E7AD1B12BDD9FC341691D8866CD1FA8374A4B</nyaa:infoHash>
      <nyaa:category>Anime - English-translated</nyaa:category>
      <nyaa:size>1.4 GiB</nyaa:size>
    </item>
  </channel>
</rss>`)

	items, err := parseNyaaRSS(rss)
	if err != nil {
		t.Fatalf("parseNyaaRSS failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 torrent item, got %d", len(items))
	}
	if items[0].Seeders != 128 || items[0].Quality != "1080p" {
		t.Fatalf("unexpected torrent fields: %+v", items[0])
	}
	if !strings.HasPrefix(items[0].Magnet, "magnet:?xt=urn:btih:ef3e7ad1b12bdd9fc341691d8866cd1fa8374a4b") {
		t.Fatalf("unexpected magnet link: %s", items[0].Magnet)
	}

	sukebeiRSS := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:nyaa="https://sukebei.nyaa.si/xmlns/nyaa">
  <channel>
    <item>
      <title>[Hentai] Boku dake no Hentai Kanojo The Animation [1080p][Uncensored].mkv</title>
      <nyaa:seeders>42</nyaa:seeders>
      <nyaa:leechers>3</nyaa:leechers>
      <nyaa:infoHash>A1B2C3D4E5F60718293A4B5C6D7E8F9012345678</nyaa:infoHash>
      <nyaa:category>Art - Anime</nyaa:category>
      <nyaa:size>640 MiB</nyaa:size>
    </item>
  </channel>
</rss>`)
	sItems, err := parseNyaaRSSWithSource(sukebeiRSS, "sukebei")
	if err != nil {
		t.Fatalf("parseNyaaRSSWithSource failed: %v", err)
	}
	if len(sItems) != 1 || sItems[0].Source != "sukebei" || sItems[0].Quality != "1080p" {
		t.Fatalf("unexpected sukebei items: %+v", sItems)
	}
}

func TestParseEpisodeOrdinal(t *testing.T) {
	cases := map[string]int{
		"[Erai-raws] Sousou no Frieren - 01 [1080p][HEVC][MultiSub].mkv":               1,
		"Mushoku.Tensei.S02E14.1080p.WEB-DL.Rus.Jpn.mkv":                               14,
		"Dandadan - 12v2 (BDRip 1080p x265 10bit).mkv":                                  12,
		"Attack on Titan - Episode 25 Final.mp4":                                       25,
		"Frieren - 03 [1080p].mkv":                                                     3,
		"Anime Series (1998) - 07 [720p].mkv":                                          7,
	}
	for name, want := range cases {
		if got := ParseEpisodeOrdinal(name); got != want {
			t.Errorf("ParseEpisodeOrdinal(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestManagerMountHandlerAndReleaseJSON(t *testing.T) {
	m := &Manager{}
	mux := http.NewServeMux()
	m.loopbackMux = mux

	called := false
	m.MountHandler("/media/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	// Idempotent duplicate mount should not panic.
	m.MountHandler("/media/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/media/stream.mp4?hash=abc&file=0", nil)
	mux.ServeHTTP(rec, req)
	if !called || rec.Code != http.StatusNoContent {
		t.Fatalf("expected mounted handler to run cleanly (called=%v code=%d)", called, rec.Code)
	}

	raw, err := json.Marshal(TorrentReleaseInfo{
		InfoHash:     "abc",
		Name:         "Show",
		MediaBaseURL: "http://127.0.0.1:54321",
	})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if !strings.Contains(string(raw), `"mediaBaseUrl":"http://127.0.0.1:54321"`) {
		t.Fatalf("expected mediaBaseUrl in serialized release info, got %s", string(raw))
	}
}

func TestDeriveSearchQueriesAndMultiSourceParsers(t *testing.T) {
	plan := deriveSearchQueries("Solo Leveling Season 2: Arise from the Shadow")
	if plan.Base != "Solo Leveling" {
		t.Fatalf("expected Base='Solo Leveling', got %q", plan.Base)
	}
	if plan.SeasonNumber != 2 || plan.SeasonQuery != "Solo Leveling S02" {
		t.Fatalf("expected SeasonNumber=2 SeasonQuery='Solo Leveling S02', got %d %q", plan.SeasonNumber, plan.SeasonQuery)
	}
	if !strings.Contains(plan.HiResQuery, "2160p|4K|UHD|1440p|QHD|2K") {
		t.Fatalf("unexpected HiResQuery: %q", plan.HiResQuery)
	}

	knabenJSON := []byte(`{"hits":[
		{"title":"[SickeClipse] Solo Leveling - Season 2 [2160p]","hash":"315027557c70ef8af2f64a4f0b26ae54bf0434e3","bytes":15780000000,"seeders":12,"peers":2,"cachedOrigin":"Nyaa.si"},
		{"title":"[Breeze] Solo Leveling S01 [1080p]","hash":"abcdef1234567890abcdef1234567890abcdef12","bytes":10000000000,"seeders":90,"peers":4,"cachedOrigin":"The Pirate Bay"}
	]}`)
	kItems, err := parseKnabenJSON(knabenJSON)
	if err != nil || len(kItems) != 2 {
		t.Fatalf("parseKnabenJSON failed: len=%d err=%v", len(kItems), err)
	}
	if kItems[0].Source != "nyaa" || kItems[0].Quality != "2160p" {
		t.Fatalf("unexpected first knaben item: %+v", kItems[0])
	}
	if kItems[1].Source != "knaben" || kItems[1].Quality != "1080p" {
		t.Fatalf("unexpected second knaben item: %+v", kItems[1])
	}
	if rankSearchResult(kItems[0], 2) <= rankSearchResult(kItems[1], 2) {
		t.Fatalf("expected 2160p S02 torrent to outrank S01 1080p torrent")
	}

	rutorHTML := []byte(`<table>
		<tr class="gai"><td>22&nbsp;Сен&nbsp;25</td><td><a href="magnet:?xt=urn:btih:2e823dd449a13c7f89628681dc2286e348e3ac39&dn=rutor.info">M</a><a href="/torrent/1054139/solo">Поднятие уровня в одиночку / Solo Leveling [S01-02] BDRip 1080p | StudioBand</a></td><td align="right">1</td><td align="right">27.82&nbsp;GB</td><td align="center"><span class="green"><img alt="S" />&nbsp;50</span>&nbsp;<img alt="L" /><span class="red">&nbsp;14</span></td></tr>
	</table>`)
	rItems := parseRuTorHTML(rutorHTML)
	if len(rItems) != 1 || rItems[0].Source != "rutor" || rItems[0].Seeders != 50 || rItems[0].Leechers != 14 || rItems[0].Size != "27.82 GB" {
		t.Fatalf("unexpected rutor parse result: %+v", rItems)
	}

	csvJSON := []byte(`{"torrents":[{"infohash":"1234567890abcdef1234567890abcdef12345678","name":"Solo Leveling S02 1440p QHD","size_bytes":2147483648,"seeders":19,"leechers":3},{"infohash":"ffffffffffffffffffffffffffffffffffffffff","name":"Unrelated Movie","size_bytes":1000,"seeders":99,"leechers":1}]}`)
	csvItems, err := parseTorrentsCSVJSON(csvJSON, "Solo Leveling")
	if err != nil || len(csvItems) != 1 || csvItems[0].Quality != "1440p" {
		t.Fatalf("unexpected torrentscsv parse result: items=%+v err=%v", csvItems, err)
	}
}
