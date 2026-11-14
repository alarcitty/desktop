package torrent

import (
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
