package torrent

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TorrentSearchResult describes a single playable torrent release across Nyaa, AniLibria, RuTor, Knaben, and TorrentsCSV.
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

type queryPlan struct {
	Full         string
	Base         string
	SeasonNumber int
	SeasonQuery  string
	HiResQuery   string
}

var (
	searchHTTPClient = &http.Client{Timeout: 9 * time.Second}
	defaultTrackers  = []string{
		"http://nyaa.tracker.wf:7777/announce",
		"udp://open.stealth.si:80/announce",
		"udp://tracker.opentrackr.org:1337/announce",
		"udp://tracker.torrent.eu.org:451/announce",
		"http://tr.libria.fun:2710/announce",
		"udp://opentor.net:6969",
	}

	seasonNumPattern     = regexp.MustCompile(`(?i)\b(?:season|s)\s*0*(\d{1,2})\b|\b(\d{1,2})(?:nd|rd|th)\s+season\b|\bpart\s*0*(\d{1,2})\b`)
	seasonStripPattern   = regexp.MustCompile(`(?i)\s*(?:[-–—|]\s*)?(?:\(\d{4}\)|\[\d{4}\]|season\s*\d+|\d+(?:nd|rd|th)\s+season|part\s*\d+|cour\s*\d+|\bs0?\d{1,2}\b)\s*.*$`)
	re2kWord             = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])2k(?:[^a-z0-9]|$)`)
	rutorRowPattern      = regexp.MustCompile(`(?is)<tr class="(?:gai|tum)">(.*?)</tr>`)
	rutorMagnetPattern   = regexp.MustCompile(`(?i)href="(magnet:\?xt=urn:btih:([a-f0-9]{40}|[a-z2-7]{32})[^"]*)"`)
	rutorTitlePattern    = regexp.MustCompile(`(?is)<a href="/torrent/\d+/[^"]*">([^<]+)</a>`)
	rutorSizePattern     = regexp.MustCompile(`(?i)<td align="right">([^<]*?(?:GB|MB|GiB|MiB|TB|KB)[^<]*)</td>`)
	rutorSeedsPattern    = regexp.MustCompile(`(?is)<span class="green">[^<]*<img[^>]*alt="S"[^>]*>(?:&nbsp;|\s)*(\d+)</span>`)
	rutorLeechesPattern  = regexp.MustCompile(`(?is)<img[^>]*alt="L"[^>]*><span class="red">(?:&nbsp;|\s)*(\d+)</span>`)
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

func deriveSearchQueries(raw string) queryPlan {
	clean := strings.TrimSpace(raw)
	plan := queryPlan{Full: clean}
	if clean == "" {
		return plan
	}

	if m := seasonNumPattern.FindStringSubmatch(clean); len(m) >= 4 {
		for i := 1; i <= 3; i++ {
			if m[i] != "" {
				if n, err := strconv.Atoi(m[i]); err == nil && n > 0 {
					plan.SeasonNumber = n
					break
				}
			}
		}
	}

	// Derive base anime title by stripping subtitle after ':' and trailing season markers.
	base := clean
	if colonIdx := strings.Index(base, ":"); colonIdx > 2 {
		base = strings.TrimSpace(base[:colonIdx])
	}
	base = strings.TrimSpace(seasonStripPattern.ReplaceAllString(base, ""))
	if base == "" {
		base = clean
	}
	plan.Base = base

	if plan.SeasonNumber >= 2 {
		plan.SeasonQuery = fmt.Sprintf("%s S%02d", base, plan.SeasonNumber)
	}
	plan.HiResQuery = base + " (2160p|4K|UHD|1440p|QHD|2K)"
	return plan
}

// SearchTorrents queries Nyaa, Sukebei, Knaben, AniLibria, RuTor, and TorrentsCSV concurrently.
func SearchTorrents(ctx context.Context, query string, aniLibriaID int) ([]TorrentSearchResult, error) {
	plan := deriveSearchQueries(query)
	if plan.Full == "" && aniLibriaID <= 0 {
		return []TorrentSearchResult{}, nil
	}

	var (
		mu      sync.Mutex
		batches [][]TorrentSearchResult
		wg      sync.WaitGroup
	)
	collect := func(items []TorrentSearchResult) {
		if len(items) == 0 {
			return
		}
		mu.Lock()
		batches = append(batches, items)
		mu.Unlock()
	}

	if len([]rune(plan.Full)) >= 2 {
		// 1. Nyaa RSS exact/full query
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res, err := searchNyaaRSS(ctx, "https://nyaa.si", plan.Full, "1_0", "nyaa"); err == nil {
				collect(res)
			}
		}()

		// 2. Nyaa RSS concise season query (when title has Season N: Subtitle)
		if plan.SeasonQuery != "" && !strings.EqualFold(plan.SeasonQuery, plan.Full) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if res, err := searchNyaaRSS(ctx, "https://nyaa.si", plan.SeasonQuery, "1_0", "nyaa"); err == nil {
					collect(res)
				}
			}()
		}

		// 3. Nyaa RSS targeted 2K/4K/UHD query so older/fewer hi-res releases aren't pushed past the 75-item limit
		if plan.HiResQuery != "" {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if res, err := searchNyaaRSS(ctx, "https://nyaa.si", plan.HiResQuery, "1_0", "nyaa"); err == nil {
					collect(res)
				}
			}()
		}

		// 4. Sukebei Nyaa (18+ / uncensored OVA/anime support)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res, err := searchNyaaRSS(ctx, "https://sukebei.nyaa.si", plan.Base, "1_1", "sukebei"); err == nil {
				collect(res)
			}
		}()

		// 5. Knaben JSON API (sorted by seeders desc across Nyaa & TPB): hi-res + general
		wg.Add(2)
		go func() {
			defer wg.Done()
			q := plan.Base + " 2160p"
			if res, err := searchKnaben(ctx, "https://api.knaben.org/v1", q, 16); err == nil {
				collect(res)
			}
		}()
		go func() {
			defer wg.Done()
			q := plan.SeasonQuery
			if q == "" {
				q = plan.Base
			}
			if res, err := searchKnaben(ctx, "https://api.knaben.org/v1", q, 20); err == nil {
				collect(res)
			}
		}()

		// 6. RuTor category 10 (Animation / Anime) for Russian multi-voice packs (StudioBand, AniLibria, Dream Cast, JAM CLUB)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res, err := searchRuTorAnime(ctx, plan.Base); err == nil {
				collect(res)
			}
		}()

		// 7. TorrentsCSV global fallback sorted by seeders
		wg.Add(1)
		go func() {
			defer wg.Done()
			q := plan.SeasonQuery
			if q == "" {
				q = plan.Base
			}
			if res, err := searchTorrentsCSV(ctx, "https://torrents-csv.com/service/search", q, 20); err == nil {
				collect(res)
			}
		}()
	}

	// 8. AniLibria / AniLiberty (direct release ID if known, otherwise catalog auto-lookup by title)
	if aniLibriaID > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res, err := fetchAniLibriaTorrents(ctx, "https://anilibria.top/api/v1", aniLibriaID); err == nil {
				collect(res)
			}
		}()
	} else if len([]rune(plan.Base)) >= 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res, err := searchAniLibriaCatalogTorrents(ctx, "https://anilibria.top/api/v1", plan); err == nil {
				collect(res)
			}
		}()
	}

	wg.Wait()

	byHash := make(map[string]TorrentSearchResult)
	for _, list := range batches {
		for _, item := range list {
			hash := strings.ToLower(strings.TrimSpace(item.InfoHash))
			if hash == "" || item.Seeders <= 0 || isNonAnimeJunk(item.Title) {
				continue
			}
			if prev, exists := byHash[hash]; !exists || preferMergedItem(item, prev) {
				byHash[hash] = item
			}
		}
	}

	merged := make([]TorrentSearchResult, 0, len(byHash))
	for _, item := range byHash {
		merged = append(merged, item)
	}
	sort.Slice(merged, func(i, j int) bool {
		scoreI := rankSearchResult(merged[i], plan.SeasonNumber)
		scoreJ := rankSearchResult(merged[j], plan.SeasonNumber)
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return merged[i].Seeders > merged[j].Seeders
	})

	if len(merged) > 60 {
		merged = merged[:60]
	}
	return merged, nil
}

func preferMergedItem(cand, prev TorrentSearchResult) bool {
	if sourcePriority(cand.Source) != sourcePriority(prev.Source) && cand.Seeders >= prev.Seeders {
		return sourcePriority(cand.Source) > sourcePriority(prev.Source)
	}
	return cand.Seeders > prev.Seeders
}

func sourcePriority(src string) int {
	switch strings.ToLower(strings.TrimSpace(src)) {
	case "anilibria":
		return 5
	case "rutor":
		return 4
	case "nyaa":
		return 3
	case "knaben":
		return 2
	case "torrentscsv":
		return 1
	default:
		return 0
	}
}

func isNonAnimeJunk(title string) bool {
	lower := strings.ToLower(title)
	junkTokens := []string{
		"fitgirl",
		"repack",
		"dodi",
		"xatab",
		"elamigos",
		"goldberg",
		"rune ",
		"codex",
		".iso",
		"soundtrack",
		"flac loss",
		"[ost]",
		"original soundtrack",
	}
	for _, tok := range junkTokens {
		if strings.Contains(lower, tok) {
			return true
		}
	}
	return false
}

func matchesRequestedSeason(lowerTitle string, season int) int {
	if season < 2 {
		return 0
	}
	sTwo := fmt.Sprintf("%02d", season)
	sOne := strconv.Itoa(season)
	// Combined batch packs like S01-02 / Season 1 + 2 / S01-S02 match well too.
	if strings.Contains(lowerTitle, "s"+sTwo) ||
		strings.Contains(lowerTitle, "season "+sOne) ||
		strings.Contains(lowerTitle, "season "+sTwo) ||
		strings.Contains(lowerTitle, "-"+sTwo) ||
		strings.Contains(lowerTitle, " 0"+sOne) && strings.Contains(lowerTitle, "s01-") ||
		strings.Contains(lowerTitle, "s01-0"+sOne) ||
		strings.Contains(lowerTitle, "season 1 + "+sOne) {
		return 260
	}
	if strings.Contains(lowerTitle, "s01") && !strings.Contains(lowerTitle, "s"+sTwo) {
		return -160
	}
	if strings.Contains(lowerTitle, "season 1") && !strings.Contains(lowerTitle, "season "+sOne) && !strings.Contains(lowerTitle, "+ "+sOne) {
		return -160
	}
	return 0
}

func rankSearchResult(item TorrentSearchResult, requestedSeason int) int {
	score := min(item.Seeders, 320)
	lower := strings.ToLower(item.Title + " " + item.Quality)

	switch {
	case strings.Contains(lower, "2160") || strings.Contains(lower, "4k") || strings.Contains(lower, "uhd"):
		score += 700
	case strings.Contains(lower, "1440") || strings.Contains(lower, "qhd") || re2kWord.MatchString(lower):
		score += 600
	case strings.Contains(lower, "1080"):
		score += 360
	case strings.Contains(lower, "720"):
		score += 120
	}

	if strings.Contains(lower, "bdrip") || strings.Contains(lower, "blu-ray") || strings.Contains(lower, "bluray") || strings.Contains(lower, "bdremux") {
		score += 160
	}
	if item.Source == "anilibria" {
		score += 260
	} else if item.Source == "rutor" {
		score += 240
	}
	if strings.Contains(lower, "rus") || strings.Contains(lower, "anilibria") || strings.Contains(lower, "studioband") || strings.Contains(lower, "dream cast") {
		score += 140
	}
	score += matchesRequestedSeason(lower, requestedSeason)
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

func searchKnaben(ctx context.Context, endpoint string, query string, size int) ([]TorrentSearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	payload, err := json.Marshal(struct {
		SearchType     string `json:"search_type"`
		SearchField    string `json:"search_field"`
		Query          string `json:"query"`
		OrderBy        string `json:"order_by"`
		OrderDirection string `json:"order_direction"`
		Categories     []int  `json:"categories"`
		Size           int    `json:"size"`
		HideUnsafe     bool   `json:"hide_unsafe"`
	}{
		SearchType:     "100%",
		SearchField:    "title",
		Query:          query,
		OrderBy:        "seeders",
		OrderDirection: "desc",
		Categories:     []int{6001000, 6000000, 2005000, 2000000},
		Size:           size,
		HideUnsafe:     true,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "mila/1.0")
	resp, err := searchHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("knaben HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	return parseKnabenJSON(raw)
}

func parseKnabenJSON(raw []byte) ([]TorrentSearchResult, error) {
	var parsed struct {
		Hits []struct {
			Title        string `json:"title"`
			Hash         string `json:"hash"`
			MagnetURL    string `json:"magnetUrl"`
			Bytes        int64  `json:"bytes"`
			Seeders      int    `json:"seeders"`
			Peers        int    `json:"peers"`
			CachedOrigin string `json:"cachedOrigin"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	out := make([]TorrentSearchResult, 0, len(parsed.Hits))
	for _, h := range parsed.Hits {
		hash := strings.ToLower(strings.TrimSpace(h.Hash))
		if hash == "" {
			continue
		}
		magnet := strings.TrimSpace(h.MagnetURL)
		if magnet == "" {
			magnet = BuildMagnetLink(hash, h.Title)
		}
		src := "knaben"
		originLower := strings.ToLower(h.CachedOrigin)
		if strings.Contains(originLower, "nyaa") {
			src = "nyaa"
		}
		out = append(out, TorrentSearchResult{
			Source:   src,
			Title:    strings.TrimSpace(h.Title),
			InfoHash: hash,
			Magnet:   magnet,
			Size:     formatByteSize(h.Bytes),
			Seeders:  h.Seeders,
			Leechers: h.Peers,
			Quality:  inferQualityTag(h.Title),
			Category: strings.TrimSpace(h.CachedOrigin),
		})
	}
	return out, nil
}

func searchTorrentsCSV(ctx context.Context, endpoint string, query string, size int) ([]TorrentSearchResult, error) {
	u := fmt.Sprintf("%s?q=%s&size=%d", endpoint, url.QueryEscape(query), size)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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
		return nil, fmt.Errorf("torrentscsv HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	return parseTorrentsCSVJSON(raw, query)
}

func parseTorrentsCSVJSON(raw []byte, query string) ([]TorrentSearchResult, error) {
	var parsed struct {
		Torrents []struct {
			Infohash  string `json:"infohash"`
			Name      string `json:"name"`
			SizeBytes int64  `json:"size_bytes"`
			Seeders   int    `json:"seeders"`
			Leechers  int    `json:"leechers"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	out := make([]TorrentSearchResult, 0, len(parsed.Torrents))
	for _, it := range parsed.Torrents {
		hash := strings.ToLower(strings.TrimSpace(it.Infohash))
		if hash == "" || !sharesTitleToken(it.Name, query) {
			continue
		}
		out = append(out, TorrentSearchResult{
			Source:   "torrentscsv",
			Title:    strings.TrimSpace(it.Name),
			InfoHash: hash,
			Magnet:   BuildMagnetLink(hash, it.Name),
			Size:     formatByteSize(it.SizeBytes),
			Seeders:  it.Seeders,
			Leechers: it.Leechers,
			Quality:  inferQualityTag(it.Name),
			Category: "Global",
		})
	}
	return out, nil
}

func searchRuTorAnime(ctx context.Context, query string) ([]TorrentSearchResult, error) {
	bases := []string{"https://rutor.info", "https://rutor.is"}
	var lastErr error
	for _, base := range bases {
		// Category 10 = Animation / Anime, sort 2 = Seeders desc
		searchURL := fmt.Sprintf("%s/search/0/10/000/2/%s", base, url.PathEscape(query))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
		resp, err := searchHTTPClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if readErr != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("rutor HTTP %d", resp.StatusCode)
			continue
		}
		return parseRuTorHTML(raw), nil
	}
	return nil, lastErr
}

func parseRuTorHTML(raw []byte) []TorrentSearchResult {
	body := string(raw)
	rows := rutorRowPattern.FindAllStringSubmatch(body, -1)
	out := make([]TorrentSearchResult, 0, len(rows))
	for _, row := range rows {
		cell := row[1]
		magMatch := rutorMagnetPattern.FindStringSubmatch(cell)
		titleMatch := rutorTitlePattern.FindStringSubmatch(cell)
		if len(magMatch) < 3 || len(titleMatch) < 2 {
			continue
		}
		magnet := html.UnescapeString(strings.TrimSpace(magMatch[1]))
		hash := strings.ToLower(strings.TrimSpace(magMatch[2]))
		title := strings.TrimSpace(html.UnescapeString(titleMatch[1]))
		if hash == "" || title == "" {
			continue
		}

		size := ""
		if sm := rutorSizePattern.FindStringSubmatch(cell); len(sm) >= 2 {
			size = strings.TrimSpace(strings.ReplaceAll(html.UnescapeString(sm[1]), "\u00a0", " "))
		}
		seeds := 0
		if ssm := rutorSeedsPattern.FindStringSubmatch(cell); len(ssm) >= 2 {
			seeds, _ = strconv.Atoi(ssm[1])
		}
		leeches := 0
		if lsm := rutorLeechesPattern.FindStringSubmatch(cell); len(lsm) >= 2 {
			leeches, _ = strconv.Atoi(lsm[1])
		}

		out = append(out, TorrentSearchResult{
			Source:   "rutor",
			Title:    title,
			InfoHash: hash,
			Magnet:   magnet,
			Size:     size,
			Seeders:  seeds,
			Leechers: leeches,
			Quality:  inferQualityTag(title),
			Category: "RuTor",
		})
	}
	return out
}

func searchAniLibriaCatalogTorrents(ctx context.Context, apiBase string, plan queryPlan) ([]TorrentSearchResult, error) {
	queries := []string{plan.Full}
	if plan.Base != "" && !strings.EqualFold(plan.Base, plan.Full) {
		queries = append(queries, plan.Base)
	}

	var matchedIDs []int
	seenIDs := make(map[int]struct{})
	for _, q := range queries {
		endpoint := fmt.Sprintf("%s/app/search/releases?query=%s", strings.TrimRight(apiBase, "/"), url.QueryEscape(q))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "mila/1.0")
		resp, err := searchHTTPClient.Do(req)
		if err != nil {
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			continue
		}

		var releases []struct {
			ID   int `json:"id"`
			Name struct {
				Main    string `json:"main"`
				English string `json:"english"`
			} `json:"name"`
		}
		if err := json.Unmarshal(raw, &releases); err != nil {
			continue
		}
		for _, rel := range releases {
			if rel.ID <= 0 {
				continue
			}
			// If Season >= 2 was requested, prefer releases matching that season first.
			combinedName := strings.ToLower(rel.Name.Main + " " + rel.Name.English)
			if plan.SeasonNumber >= 2 && !strings.Contains(combinedName, strconv.Itoa(plan.SeasonNumber)) && len(releases) > 1 {
				continue
			}
			if _, ok := seenIDs[rel.ID]; !ok {
				seenIDs[rel.ID] = struct{}{}
				matchedIDs = append(matchedIDs, rel.ID)
			}
			if len(matchedIDs) >= 2 {
				break
			}
		}
		if len(matchedIDs) > 0 {
			break
		}
	}

	var out []TorrentSearchResult
	for _, id := range matchedIDs {
		if items, err := fetchAniLibriaTorrents(ctx, apiBase, id); err == nil {
			out = append(out, items...)
		}
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

func sharesTitleToken(candidateTitle string, query string) bool {
	lowerCand := strings.ToLower(candidateTitle)
	for _, tok := range strings.Fields(strings.ToLower(query)) {
		cleanTok := strings.Trim(tok, ":()[].,-!?'\"")
		if len([]rune(cleanTok)) < 3 || cleanTok == "season" || cleanTok == "the" {
			continue
		}
		if strings.Contains(lowerCand, cleanTok) {
			return true
		}
	}
	return false
}

func inferQualityTag(title string) string {
	lower := strings.ToLower(title)
	switch {
	case strings.Contains(lower, "2160") || strings.Contains(lower, "4k") || strings.Contains(lower, "uhd"):
		return "2160p"
	case strings.Contains(lower, "1440") || strings.Contains(lower, "qhd") || re2kWord.MatchString(lower):
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
