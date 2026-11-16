package torrent

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TorrentSearchResult describes a single playable torrent release from Nyaa or AniLibria.
type TorrentSearchResult struct {
	Source   string `json:"source"`
	Title    string `json:"title"`
	InfoHash string `json:"infoHash"`
	Magnet   string `json:"magnet"`
	Size     string `json:"size"`
	Seeders  int    `json:"seeders"`
	Leechers int    `json:"leechers"`
	Quality  string `json:"quality,omitempty"`
	Category string `json:"category,omitempty"`
}

var (
	searchHTTPClient = &http.Client{Timeout: 12 * time.Second}
	defaultTrackers  = []string{
		"http://nyaa.tracker.wf:7777/announce",
		"udp://open.stealth.si:80/announce",
		"udp://tracker.opentrackr.org:1337/announce",
		"udp://tracker.torrent.eu.org:451/announce",
		"http://tr.libria.fun:2710/announce",
	}
)

// BuildMagnetLink builds a standard magnet URI with healthy public trackers for an infohash.
func BuildMagnetLink(infoHash string, displayName string) string {
	hash := strings.ToLower(strings.TrimSpace(infoHash))
	if hash == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("magnet:?xt=urn:btih:")
	b.WriteString(url.QueryEscape(hash))
	if strings.TrimSpace(displayName) != "" {
		b.WriteString("&dn=")
		b.WriteString(url.QueryEscape(displayName))
	}
	for _, tr := range defaultTrackers {
		b.WriteString("&tr=")
		b.WriteString(url.QueryEscape(tr))
	}
	return b.String()
}

// SearchTorrents queries Nyaa RSS, Sukebei Nyaa RSS, and optional AniLibria release torrents concurrently.
func SearchTorrents(ctx context.Context, query string, aniLibriaID int) ([]TorrentSearchResult, error) {
	clean := strings.TrimSpace(query)
	if clean == "" && aniLibriaID <= 0 {
		return []TorrentSearchResult{}, nil
	}

	var wg sync.WaitGroup
	var nyaaRes, sukebeiRes, libRes []TorrentSearchResult

	if len([]rune(clean)) >= 2 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			res, err := searchNyaaRSS(ctx, "https://nyaa.si", clean, "1_0", "nyaa")
			if err == nil {
				nyaaRes = res
			}
		}()
		go func() {
			defer wg.Done()
			res, err := searchNyaaRSS(ctx, "https://sukebei.nyaa.si", clean, "1_1", "sukebei")
			if err == nil {
				sukebeiRes = res
			}
		}()
	}

	if aniLibriaID > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := fetchAniLibriaTorrents(ctx, "https://anilibria.top/api/v1", aniLibriaID)
			if err == nil {
				libRes = res
			}
		}()
	}

	wg.Wait()

	byHash := make(map[string]TorrentSearchResult)
	combined := make([]TorrentSearchResult, 0, len(libRes)+len(nyaaRes)+len(sukebeiRes))
	combined = append(combined, libRes...)
	combined = append(combined, nyaaRes...)
	combined = append(combined, sukebeiRes...)
	for _, item := range combined {
		hash := strings.ToLower(strings.TrimSpace(item.InfoHash))
		if hash == "" || item.Seeders <= 0 {
			continue
		}
		if prev, exists := byHash[hash]; !exists || item.Seeders > prev.Seeders {
			byHash[hash] = item
		}
	}

	merged := make([]TorrentSearchResult, 0, len(byHash))
	for _, item := range byHash {
		merged = append(merged, item)
	}
	sort.Slice(merged, func(i, j int) bool {
		// Prioritize seeded 1080p releases, then AniLibria RU BDRip/WEB-DL, then seeders count.
		scoreI := rankSearchResult(merged[i])
		scoreJ := rankSearchResult(merged[j])
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return merged[i].Seeders > merged[j].Seeders
	})

	if len(merged) > 30 {
		merged = merged[:30]
	}
	return merged, nil
}

func rankSearchResult(item TorrentSearchResult) int {
	score := item.Seeders
	lower := strings.ToLower(item.Title + " " + item.Quality)
	if strings.Contains(lower, "2160") || strings.Contains(lower, "4k") || strings.Contains(lower, "uhd") {
		score += 420
	} else if strings.Contains(lower, "1440") || strings.Contains(lower, "2k") {
		score += 410
	} else if strings.Contains(lower, "1080") {
		score += 400
	}
	if strings.Contains(lower, "bdrip") || strings.Contains(lower, "blu-ray") || strings.Contains(lower, "bluray") {
		score += 180
	}
	if item.Source == "anilibria" {
		score += 220
	}
	if strings.Contains(lower, "rus") || strings.Contains(lower, "anilibria") || strings.Contains(lower, "studiy") {
		score += 120
	}
	return score
}

func searchNyaaRSS(ctx context.Context, baseURL string, query string, category string, sourceName string) ([]TorrentSearchResult, error) {
	if strings.TrimSpace(category) == "" {
		category = "1_0"
	}
	rssURL := fmt.Sprintf("%s/?page=rss&q=%s&c=%s", strings.TrimRight(baseURL, "/"), url.QueryEscape(query), url.QueryEscape(category))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rssURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mila/1.0")
	resp, err := searchHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s HTTP %d", sourceName, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return parseNyaaRSSWithSource(raw, sourceName)
}

func parseNyaaRSS(raw []byte) ([]TorrentSearchResult, error) {
	return parseNyaaRSSWithSource(raw, "nyaa")
}

func parseNyaaRSSWithSource(raw []byte, sourceName string) ([]TorrentSearchResult, error) {
	if strings.TrimSpace(sourceName) == "" {
		sourceName = "nyaa"
	}
	var feed struct {
		Channel struct {
			Items []struct {
				Title    string `xml:"title"`
				InfoHash string `xml:"infoHash"`
				Seeders  int    `xml:"seeders"`
				Leechers int    `xml:"leechers"`
				Size     string `xml:"size"`
				Category string `xml:"category"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(raw, &feed); err != nil {
		return nil, err
	}

	out := make([]TorrentSearchResult, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		hash := strings.ToLower(strings.TrimSpace(it.InfoHash))
		if hash == "" {
			continue
		}
		out = append(out, TorrentSearchResult{
			Source:   sourceName,
			Title:    strings.TrimSpace(it.Title),
			InfoHash: hash,
			Magnet:   BuildMagnetLink(hash, it.Title),
			Size:     strings.TrimSpace(it.Size),
			Seeders:  it.Seeders,
			Leechers: it.Leechers,
			Quality:  inferQualityTag(it.Title),
			Category: strings.TrimSpace(it.Category),
		})
	}
	return out, nil
}

func fetchAniLibriaTorrents(ctx context.Context, apiBase string, releaseID int) ([]TorrentSearchResult, error) {
	endpoint := fmt.Sprintf("%s/anime/torrents/release/%d", strings.TrimRight(apiBase, "/"), releaseID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mila/1.0")
	resp, err := searchHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("anilibria torrents HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}

	var entries []struct {
		Hash     string `json:"hash"`
		Size     int64  `json:"size"`
		Label    string `json:"label"`
		Magnet   string `json:"magnet"`
		Seeders  int    `json:"seeders"`
		Leechers int    `json:"leechers"`
		Quality  struct {
			Value string `json:"value"`
		} `json:"quality"`
		Codec struct {
			Label string `json:"label"`
		} `json:"codec"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}

	out := make([]TorrentSearchResult, 0, len(entries))
	for _, e := range entries {
		hash := strings.ToLower(strings.TrimSpace(e.Hash))
		if hash == "" {
			continue
		}
		magnet := strings.TrimSpace(e.Magnet)
		if magnet == "" {
			magnet = BuildMagnetLink(hash, e.Label)
		}
		qual := strings.TrimSpace(e.Quality.Value)
		if e.Codec.Label != "" {
			qual = strings.TrimSpace(qual + " " + e.Codec.Label)
		}
		out = append(out, TorrentSearchResult{
			Source:   "anilibria",
			Title:    strings.TrimSpace(e.Label),
			InfoHash: hash,
			Magnet:   magnet,
			Size:     formatByteSize(e.Size),
			Seeders:  e.Seeders,
			Leechers: e.Leechers,
			Quality:  qual,
			Category: "AniLibria",
		})
	}
	return out, nil
}

func inferQualityTag(title string) string {
	lower := strings.ToLower(title)
	switch {
	case strings.Contains(lower, "2160") || strings.Contains(lower, "4k") || strings.Contains(lower, "uhd"):
		return "2160p"
	case strings.Contains(lower, "1440") || strings.Contains(lower, "2k"):
		return "1440p"
	case strings.Contains(lower, "1080"):
		return "1080p"
	case strings.Contains(lower, "720"):
		return "720p"
	case strings.Contains(lower, "480"):
		return "480p"
	default:
		return ""
	}
}

func formatByteSize(bytes int64) string {
	if bytes <= 0 {
		return ""
	}
	const giB = 1024 * 1024 * 1024
	const miB = 1024 * 1024
	if bytes >= giB {
		return strconv.FormatFloat(float64(bytes)/float64(giB), 'f', 2, 64) + " GiB"
	}
	return strconv.FormatFloat(float64(bytes)/float64(miB), 'f', 1, 64) + " MiB"
}
