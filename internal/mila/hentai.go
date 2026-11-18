package mila

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mila/internal/httpx"
)

const (
	hentaiMamaBaseURL    = "https://hentaimama.io"
	hentaiGasmBaseURL    = "https://hentaigasm.com"
	hentaiCacheTTL       = 10 * time.Minute
	hentaiMaxConcurrency = 4
)

// HentaiSearchItem represents one matched series/variant entry from HentaiMama or HentaiGasm.
type HentaiSearchItem struct {
	Provider      string `json:"provider"`
	Target        string `json:"target"`
	Title         string `json:"title"`
	OriginalTitle string `json:"originalTitle,omitempty"`
	Year          int    `json:"year,omitempty"`
	Poster        string `json:"poster,omitempty"`
	Variant       string `json:"variant,omitempty"`
	EpisodeCount  int    `json:"episodeCount,omitempty"`
}

// HentaiEpisodeItem represents one resolved playable episode from a hentai provider.
type HentaiEpisodeItem struct {
	Ordinal  int    `json:"ordinal"`
	Name     string `json:"name,omitempty"`
	Duration int    `json:"duration,omitempty"`
	HLS1080  string `json:"hls_1080,omitempty"`
	MP41080  string `json:"mp4_1080,omitempty"`
	MP4720   string `json:"mp4_720,omitempty"`
	MP4480   string `json:"mp4_480,omitempty"`
}

type hentaiSearchCacheEntry struct {
	items     []HentaiSearchItem
	expiresAt time.Time
}

type hentaiEpisodesCacheEntry struct {
	episodes  []HentaiEpisodeItem
	expiresAt time.Time
}

var (
	hentaiCacheMu       sync.RWMutex
	hentaiSearchCache   = make(map[string]hentaiSearchCacheEntry)
	hentaiEpisodesCache = make(map[string]hentaiEpisodesCacheEntry)

	reMamaArticleStart = regexp.MustCompile(`<article\b[^>]*class="[^"]*\bseries-card\b[^"]*"[^>]*>`)
	reMamaPosterHref   = regexp.MustCompile(`<a\b[^>]*class="[^"]*\bsc-poster\b[^"]*"[^>]*href="([^"]+)"`)
	reMamaImgSrc       = regexp.MustCompile(`<img\b[^>]*\bsrc="([^"]+)"`)
	reMamaImgAlt       = regexp.MustCompile(`<img\b[^>]*\balt="([^"]*)"`)
	reMamaTitle        = regexp.MustCompile(`(?s)<h3\b[^>]*class="[^"]*\bsc-title\b[^"]*"[^>]*>.*?<a\b[^>]*>(.*?)</a>`)
	reMamaAltTitle     = regexp.MustCompile(`(?s)<div\b[^>]*class="[^"]*\bsc-alt\b[^"]*"[^>]*>(.*?)</div>`)
	reMamaYearTag      = regexp.MustCompile(`<span\b[^>]*class="[^"]*\bsc-tag\b[^"]*"[^>]*>\s*(19\d{2}|20\d{2})\s*</span>`)
	reMamaEpisodeURL   = regexp.MustCompile(`href="(https?://[^"]+/episodes/[^"]+)"`)
	reMamaEpID         = regexp.MustCompile(`"episode"\s*:\s*"(\d+)"`)
	reMamaDuration     = regexp.MustCompile(`"duration"\s*:\s*"(\d+)"`)
	reMamaIframeSrc    = regexp.MustCompile(`src=["']([^"']+)["']`)
	reMamaSourcesArr   = regexp.MustCompile(`(?s)sources\s*:\s*(\[[^\]]+\])`)
	reTrailingEpNum    = regexp.MustCompile(`(?i)(?:-episode-|-ep-|-|#|\s)(\d+)(?:/?$|\s|$)`)

	reGasmPostStart = regexp.MustCompile(`<div\b[^>]*class="[^"]*\bitem-post\b[^"]*"[^>]*>`)
	reGasmCategory  = regexp.MustCompile(`\bcategory-([a-z0-9-]+)`)
	reGasmClipLink  = regexp.MustCompile(`<a\b[^>]*class="[^"]*\bclip-link\b[^"]*"[^>]*title="([^"]+)"[^>]*href="([^"]+)"`)
	reGasmClipRev   = regexp.MustCompile(`<a\b[^>]*class="[^"]*\bclip-link\b[^"]*"[^>]*href="([^"]+)"[^>]*title="([^"]+)"`)
	reGasmTitleLink = regexp.MustCompile(`(?s)<h2\b[^>]*class="[^"]*\btitle\b[^"]*"[^>]*>\s*<a\b[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	reGasmMP4File   = regexp.MustCompile(`(?i)(?:file|src)\s*[:=]\s*["']([^"']+\.mp4[^"']*)["']`)
	reGasmOrdTail   = regexp.MustCompile(`(?i)(?:uncensored\s*)?(\d+)\s*(?:subbed|dubbed|raw)?\s*$`)
)

// HentaiSearch searches configured hentai providers from the desktop Go host.
func (m *Mila) HentaiSearch(query string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	items, err := searchHentaiAll(ctx, hentaiMamaBaseURL, hentaiGasmBaseURL, query)
	if err != nil {
		return "[]", err
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return "[]", err
	}
	return string(raw), nil
}

// HentaiGetEpisodes resolves playable HLS/MP4 episode streams for a hentai match.
func (m *Mila) HentaiGetEpisodes(provider string, target string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	episodes, err := resolveHentaiEpisodes(ctx, provider, target, true)
	if err != nil {
		return "[]", err
	}
	raw, err := json.Marshal(episodes)
	if err != nil {
		return "[]", err
	}
	return string(raw), nil
}

func serveHentaiSearch(w http.ResponseWriter, r *http.Request) {
	setProxyCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	items, err := searchHentaiAll(r.Context(), hentaiMamaBaseURL, hentaiGasmBaseURL, q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(items)
}

func serveHentaiEpisodes(w http.ResponseWriter, r *http.Request) {
	setProxyCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	target := strings.TrimSpace(r.URL.Query().Get("target"))
	episodes, err := resolveHentaiEpisodes(r.Context(), provider, target, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(episodes)
}

func searchHentaiAll(ctx context.Context, mamaBase string, gasmBase string, query string) ([]HentaiSearchItem, error) {
	clean := strings.TrimSpace(query)
	if len([]rune(clean)) < 2 {
		return []HentaiSearchItem{}, nil
	}

	cacheKey := strings.ToLower(mamaBase + "|" + gasmBase + "|" + clean)
	hentaiCacheMu.RLock()
	if entry, ok := hentaiSearchCache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		hentaiCacheMu.RUnlock()
		return entry.items, nil
	}
	hentaiCacheMu.RUnlock()

	var wg sync.WaitGroup
	var mamaItems, gasmItems []HentaiSearchItem
	var mamaErr, gasmErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		mamaItems, mamaErr = searchHentaiMama(ctx, mamaBase, clean)
	}()
	go func() {
		defer wg.Done()
		gasmItems, gasmErr = searchHentaiGasm(ctx, gasmBase, clean)
	}()
	wg.Wait()

	if mamaErr != nil && gasmErr != nil && len(mamaItems) == 0 && len(gasmItems) == 0 {
		return []HentaiSearchItem{}, fmt.Errorf("hentai search failed: %v; %v", mamaErr, gasmErr)
	}

	out := make([]HentaiSearchItem, 0, len(mamaItems)+len(gasmItems))
	out = append(out, mamaItems...)
	out = append(out, gasmItems...)

	hentaiCacheMu.Lock()
	hentaiSearchCache[cacheKey] = hentaiSearchCacheEntry{
		items:     out,
		expiresAt: time.Now().Add(hentaiCacheTTL),
	}
	hentaiCacheMu.Unlock()
	return out, nil
}

func resolveHentaiEpisodes(ctx context.Context, provider string, target string, strictHost bool) ([]HentaiEpisodeItem, error) {
	prov := strings.ToLower(strings.TrimSpace(provider))
	cleanTarget := strings.TrimSpace(target)
	if cleanTarget == "" {
		return []HentaiEpisodeItem{}, errors.New("missing hentai target")
	}

	cacheKey := prov + "::" + cleanTarget
	hentaiCacheMu.RLock()
	if entry, ok := hentaiEpisodesCache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		hentaiCacheMu.RUnlock()
		return entry.episodes, nil
	}
	hentaiCacheMu.RUnlock()

	var episodes []HentaiEpisodeItem
	var err error
	switch prov {
	case "hentaimama":
		if strictHost {
			if err := validateAllowedProviderURL(cleanTarget, "hentaimama.io"); err != nil {
				return nil, err
			}
		}
		episodes, err = fetchHentaiMamaEpisodes(ctx, cleanTarget, strictHost)
	case "hentaigasm":
		epURLs := splitHentaiGasmTargetURLs(cleanTarget)
		if len(epURLs) == 0 {
			return []HentaiEpisodeItem{}, errors.New("no valid hentaigasm episode URLs")
		}
		if strictHost {
			for _, u := range epURLs {
				if err := validateAllowedProviderURL(u, "hentaigasm.com"); err != nil {
					return nil, err
				}
			}
		}
		episodes, err = fetchHentaiGasmEpisodes(ctx, epURLs, strictHost)
	default:
		return nil, fmt.Errorf("unsupported hentai provider: %s", provider)
	}
	if err != nil {
		return nil, err
	}

	hentaiCacheMu.Lock()
	hentaiEpisodesCache[cacheKey] = hentaiEpisodesCacheEntry{
		episodes:  episodes,
		expiresAt: time.Now().Add(hentaiCacheTTL),
	}
	hentaiCacheMu.Unlock()
	return episodes, nil
}

func validateAllowedProviderURL(rawURL string, allowedHost string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return errors.New("invalid provider URL")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errors.New("unsupported provider URL scheme")
	}
	host := strings.ToLower(parsed.Hostname())
	allowed := strings.ToLower(allowedHost)
	if host != allowed && !strings.HasSuffix(host, "."+allowed) {
		return fmt.Errorf("forbidden provider host: %s", host)
	}
	return nil
}

func fetchHentaiHTML(ctx context.Context, rawURL string, referer string) (string, error) {
	headers := map[string]string{
		"User-Agent": defaultBrowserUA,
		"Accept":     "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
	}
	if strings.TrimSpace(referer) != "" {
		headers["Referer"] = referer
	}
	body, status, err := fetchTextWithHeaders(ctx, rawURL, headers)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("upstream returned HTTP %d", status)
	}
	return body, nil
}

func searchHentaiMama(ctx context.Context, baseURL string, query string) ([]HentaiSearchItem, error) {
	endpoint := fmt.Sprintf("%s/?s=%s", strings.TrimRight(baseURL, "/"), url.QueryEscape(query))
	rawHTML, err := fetchHentaiHTML(ctx, endpoint, strings.TrimRight(baseURL, "/")+"/")
	if err != nil {
		return nil, err
	}
	return parseHentaiMamaSearchHTML(rawHTML), nil
}

func parseHentaiMamaSearchHTML(rawHTML string) []HentaiSearchItem {
	locs := reMamaArticleStart.FindAllStringIndex(rawHTML, 20)
	if len(locs) == 0 {
		return []HentaiSearchItem{}
	}
	items := make([]HentaiSearchItem, 0, len(locs))
	seen := make(map[string]struct{})
	for i, loc := range locs {
		start := loc[0]
		end := min(len(rawHTML), start+6000)
		if i+1 < len(locs) && locs[i+1][0] < end {
			end = locs[i+1][0]
		}
		chunk := rawHTML[start:end]

		posterMatch := reMamaPosterHref.FindStringSubmatch(chunk)
		if len(posterMatch) < 2 {
			continue
		}
		showURL := strings.TrimSpace(html.UnescapeString(posterMatch[1]))
		if !strings.Contains(strings.ToLower(showURL), "/tvshows/") {
			continue
		}
		if _, dup := seen[showURL]; dup {
			continue
		}
		seen[showURL] = struct{}{}

		title := ""
		if m := reMamaTitle.FindStringSubmatch(chunk); len(m) > 1 {
			title = cleanHTMLText(m[1])
		}
		if title == "" {
			if m := reMamaImgAlt.FindStringSubmatch(chunk); len(m) > 1 {
				title = cleanHTMLText(m[1])
			}
		}
		if title == "" {
			continue
		}

		altTitle := ""
		if m := reMamaAltTitle.FindStringSubmatch(chunk); len(m) > 1 {
			altTitle = cleanHTMLText(m[1])
		}
		poster := ""
		if m := reMamaImgSrc.FindStringSubmatch(chunk); len(m) > 1 {
			poster = html.UnescapeString(strings.TrimSpace(m[1]))
		}
		year := 0
		if m := reMamaYearTag.FindStringSubmatch(chunk); len(m) > 1 {
			year, _ = strconv.Atoi(m[1])
		}

		items = append(items, HentaiSearchItem{
			Provider:      "hentaimama",
			Target:        showURL,
			Title:         title,
			OriginalTitle: altTitle,
			Year:          year,
			Poster:        poster,
			Variant:       "Subbed",
		})
	}
	return items
}

func fetchHentaiMamaEpisodes(ctx context.Context, showURL string, strictPublicStreams bool) ([]HentaiEpisodeItem, error) {
	showHTML, err := fetchHentaiHTML(ctx, showURL, originReferer(showURL))
	if err != nil {
		return nil, err
	}
	epURLs := extractHentaiMamaEpisodeURLs(showHTML)
	if len(epURLs) == 0 {
		return []HentaiEpisodeItem{}, nil
	}

	type epResult struct {
		item HentaiEpisodeItem
		ok   bool
	}
	results := make([]epResult, len(epURLs))
	sem := make(chan struct{}, hentaiMaxConcurrency)
	var wg sync.WaitGroup

	for idx, epURL := range epURLs {
		wg.Add(1)
		go func(i int, targetEpURL string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			item, resolveErr := resolveSingleHentaiMamaEpisode(ctx, targetEpURL, i+1, showURL, strictPublicStreams)
			if resolveErr == nil {
				results[i] = epResult{item: item, ok: true}
			}
		}(idx, epURL)
	}
	wg.Wait()

	out := make([]HentaiEpisodeItem, 0, len(results))
	for _, r := range results {
		if r.ok {
			out = append(out, r.item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Ordinal < out[j].Ordinal
	})
	return out, nil
}

func extractHentaiMamaEpisodeURLs(showHTML string) []string {
	matches := reMamaEpisodeURL.FindAllStringSubmatch(showHTML, -1)
	if len(matches) == 0 {
		return nil
	}
	type epEntry struct {
		url string
		ord int
		idx int
	}
	seen := make(map[string]struct{})
	entries := make([]epEntry, 0, len(matches))
	for i, m := range matches {
		if len(m) < 2 {
			continue
		}
		u := strings.TrimSpace(html.UnescapeString(m[1]))
		if u == "" {
			continue
		}
		if _, dup := seen[u]; dup {
			continue
		}
		seen[u] = struct{}{}
		entries = append(entries, epEntry{
			url: u,
			ord: extractTrailingOrdinal(u, i+1),
			idx: i,
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].ord != entries[j].ord {
			return entries[i].ord < entries[j].ord
		}
		return entries[i].idx < entries[j].idx
	})
	urls := make([]string, 0, len(entries))
	for _, e := range entries {
		urls = append(urls, e.url)
	}
	return urls
}

func resolveSingleHentaiMamaEpisode(
	ctx context.Context,
	epURL string,
	fallbackOrdinal int,
	showReferer string,
	strictPublicStreams bool,
) (HentaiEpisodeItem, error) {
	epHTML, err := fetchHentaiHTML(ctx, epURL, showReferer)
	if err != nil {
		return HentaiEpisodeItem{}, err
	}
	epIDMatch := reMamaEpID.FindStringSubmatch(epHTML)
	if len(epIDMatch) < 2 {
		return HentaiEpisodeItem{}, errors.New("missing hentaimama episode id")
	}
	epID := epIDMatch[1]
	duration := 0
	if durMatch := reMamaDuration.FindStringSubmatch(epHTML); len(durMatch) > 1 {
		duration, _ = strconv.Atoi(durMatch[1])
	}

	parsedEp, err := url.Parse(epURL)
	if err != nil || parsedEp.Host == "" {
		return HentaiEpisodeItem{}, errors.New("invalid episode URL")
	}
	ajaxURL := parsedEp.Scheme + "://" + parsedEp.Host + "/wp-admin/admin-ajax.php"
	form := url.Values{}
	form.Set("action", "get_player_contents")
	form.Set("a", epID)

	encodedForm := form.Encode()
	res, err := httpx.DoWithRetry(ctx, providerHTTPClient, providerLimiter, httpx.RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   250 * time.Millisecond,
		MaxDelay:    4 * time.Second,
		MaxBody:     maxExtractorBodySize,
	}, func(rCtx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(rCtx, http.MethodPost, ajaxURL, strings.NewReader(encodedForm))
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", defaultBrowserUA)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", epURL)
		return req, nil
	})
	if err != nil {
		return HentaiEpisodeItem{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return HentaiEpisodeItem{}, fmt.Errorf("hentaimama ajax HTTP %d", res.StatusCode)
	}
	rawJSON := res.Body

	var iframes []string
	if err := json.Unmarshal(rawJSON, &iframes); err != nil {
		return HentaiEpisodeItem{}, err
	}

	var hlsEmbedURL, mp4EmbedURL string
	for _, frame := range iframes {
		m := reMamaIframeSrc.FindStringSubmatch(frame)
		if len(m) < 2 {
			continue
		}
		src := html.UnescapeString(strings.TrimSpace(m[1]))
		lower := strings.ToLower(src)
		if strings.Contains(lower, "dt_embed=hls&") || strings.Contains(lower, "dt_embed=hls") && !strings.Contains(lower, "dt_embed=hls-mp4") {
			if hlsEmbedURL == "" {
				hlsEmbedURL = src
			}
		} else if strings.Contains(lower, "dt_embed=r2-mp4") {
			mp4EmbedURL = src
		} else if strings.Contains(lower, "dt_embed=hls-mp4") && mp4EmbedURL == "" {
			mp4EmbedURL = src
		}
	}

	ordinal := extractTrailingOrdinal(epURL, fallbackOrdinal)
	item := HentaiEpisodeItem{
		Ordinal:  ordinal,
		Name:     fmt.Sprintf("Episode %d", ordinal),
		Duration: duration,
	}

	referer := originReferer(epURL)
	var embedWG sync.WaitGroup
	var mu sync.Mutex

	if hlsEmbedURL != "" {
		embedWG.Add(1)
		go func() {
			defer embedWG.Done()
			embedHTML, fetchErr := fetchHentaiHTML(ctx, hlsEmbedURL, epURL)
			if fetchErr != nil {
				return
			}
			for _, s := range parseJWPlayerSources(embedHTML) {
				if strings.Contains(strings.ToLower(s.File), ".m3u8") || strings.EqualFold(s.Type, "hls") {
					proxied, proxyErr := sanitizeAndProxyHentaiStream(s.File, referer, strictPublicStreams)
					if proxyErr == nil {
						mu.Lock()
						item.HLS1080 = proxied
						mu.Unlock()
						return
					}
				}
			}
		}()
	}

	if mp4EmbedURL != "" {
		embedWG.Add(1)
		go func() {
			defer embedWG.Done()
			embedHTML, fetchErr := fetchHentaiHTML(ctx, mp4EmbedURL, epURL)
			if fetchErr != nil {
				return
			}
			sources := parseJWPlayerSources(embedHTML)
			mu.Lock()
			assignMP4Qualities(&item, sources, referer, strictPublicStreams)
			mu.Unlock()
		}()
	}

	embedWG.Wait()

	if item.HLS1080 == "" && item.MP41080 == "" && item.MP4720 == "" && item.MP4480 == "" {
		return HentaiEpisodeItem{}, errors.New("no playable sources found in hentaimama embeds")
	}
	return item, nil
}

type jwPlayerSource struct {
	File  string `json:"file"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

func parseJWPlayerSources(embedHTML string) []jwPlayerSource {
	m := reMamaSourcesArr.FindStringSubmatch(embedHTML)
	if len(m) < 2 {
		return nil
	}
	var out []jwPlayerSource
	if err := json.Unmarshal([]byte(m[1]), &out); err != nil {
		return nil
	}
	return out
}

func assignMP4Qualities(item *HentaiEpisodeItem, sources []jwPlayerSource, referer string, strictPublicStreams bool) {
	for _, s := range sources {
		proxied, err := sanitizeAndProxyHentaiStream(s.File, referer, strictPublicStreams)
		if err != nil {
			continue
		}
		lowerLabel := strings.ToLower(s.Label + " " + s.File)
		switch {
		case strings.Contains(lowerLabel, "1080"):
			if item.MP41080 == "" {
				item.MP41080 = proxied
			}
		case strings.Contains(lowerLabel, "720"):
			if item.MP4720 == "" {
				item.MP4720 = proxied
			}
		case strings.Contains(lowerLabel, "480"):
			if item.MP4480 == "" {
				item.MP4480 = proxied
			}
		default:
			if item.MP4720 == "" {
				item.MP4720 = proxied
			}
		}
	}
}

func searchHentaiGasm(ctx context.Context, baseURL string, query string) ([]HentaiSearchItem, error) {
	endpoint := fmt.Sprintf("%s/?s=%s", strings.TrimRight(baseURL, "/"), url.QueryEscape(query))
	rawHTML, err := fetchHentaiHTML(ctx, endpoint, strings.TrimRight(baseURL, "/")+"/")
	if err != nil {
		return nil, err
	}
	return parseHentaiGasmSearchHTML(rawHTML), nil
}

type gasmGroup struct {
	item     HentaiSearchItem
	episodes []gasmEpisodeEntry
	seenURLs map[string]struct{}
}

type gasmEpisodeEntry struct {
	url string
	ord int
}

func parseHentaiGasmSearchHTML(rawHTML string) []HentaiSearchItem {
	locs := reGasmPostStart.FindAllStringIndex(rawHTML, 40)
	if len(locs) == 0 {
		return []HentaiSearchItem{}
	}

	groupOrder := make([]string, 0, len(locs))
	groups := make(map[string]*gasmGroup)

	for i, loc := range locs {
		start := loc[0]
		end := min(len(rawHTML), start+5000)
		if i+1 < len(locs) && locs[i+1][0] < end {
			end = locs[i+1][0]
		}
		chunk := rawHTML[start:end]

		var epURL, rawTitle string
		if m := reGasmClipLink.FindStringSubmatch(chunk); len(m) > 2 {
			rawTitle = cleanHTMLText(m[1])
			epURL = strings.TrimSpace(html.UnescapeString(m[2]))
		} else if m := reGasmClipRev.FindStringSubmatch(chunk); len(m) > 2 {
			epURL = strings.TrimSpace(html.UnescapeString(m[1]))
			rawTitle = cleanHTMLText(m[2])
		} else if m := reGasmTitleLink.FindStringSubmatch(chunk); len(m) > 2 {
			epURL = strings.TrimSpace(html.UnescapeString(m[1]))
			rawTitle = cleanHTMLText(m[2])
		}
		if epURL == "" || rawTitle == "" {
			continue
		}

		poster := ""
		if m := reMamaImgSrc.FindStringSubmatch(chunk); len(m) > 1 {
			poster = strings.TrimSpace(html.UnescapeString(m[1]))
		}

		catSlug := ""
		if m := reGasmCategory.FindStringSubmatch(chunk); len(m) > 1 {
			catSlug = strings.ToLower(strings.TrimSpace(m[1]))
		}

		seriesTitle, variant, ord := splitHentaiGasmEpisodeTitle(rawTitle)
		if seriesTitle == "" {
			seriesTitle = rawTitle
		}
		groupKey := catSlug
		if groupKey == "" {
			groupKey = strings.ToLower(seriesTitle) + "|" + strings.ToLower(variant)
		}

		grp, exists := groups[groupKey]
		if !exists {
			grp = &gasmGroup{
				item: HentaiSearchItem{
					Provider: "hentaigasm",
					Title:    seriesTitle,
					Poster:   poster,
					Variant:  variant,
				},
				seenURLs: make(map[string]struct{}),
			}
			groups[groupKey] = grp
			groupOrder = append(groupOrder, groupKey)
		}
		if grp.item.Poster == "" && poster != "" {
			grp.item.Poster = poster
		}
		if _, dup := grp.seenURLs[epURL]; !dup {
			grp.seenURLs[epURL] = struct{}{}
			grp.episodes = append(grp.episodes, gasmEpisodeEntry{url: epURL, ord: ord})
		}
	}

	out := make([]HentaiSearchItem, 0, len(groupOrder))
	for _, key := range groupOrder {
		grp := groups[key]
		if len(grp.episodes) == 0 {
			continue
		}
		sort.SliceStable(grp.episodes, func(i, j int) bool {
			return grp.episodes[i].ord < grp.episodes[j].ord
		})
		urls := make([]string, 0, len(grp.episodes))
		for _, ep := range grp.episodes {
			urls = append(urls, ep.url)
		}
		item := grp.item
		item.Target = strings.Join(urls, "|")
		item.EpisodeCount = len(urls)
		out = append(out, item)
	}
	return out
}

func splitHentaiGasmEpisodeTitle(rawTitle string) (seriesTitle string, variant string, ordinal int) {
	trimmed := strings.TrimSpace(rawTitle)
	lower := strings.ToLower(trimmed)
	variant = "Subbed"
	if strings.Contains(lower, "uncensored") {
		variant = "Uncensored"
	} else if strings.Contains(lower, "dubbed") {
		variant = "Dubbed"
	}

	ordinal = 1
	if m := reGasmOrdTail.FindStringSubmatchIndex(trimmed); len(m) >= 4 {
		if m[2] >= 0 && m[3] > m[2] {
			if n, err := strconv.Atoi(trimmed[m[2]:m[3]]); err == nil && n > 0 {
				ordinal = n
			}
		}
		seriesTitle = strings.TrimSpace(trimmed[:m[0]])
	} else {
		seriesTitle = trimmed
	}
	seriesTitle = strings.TrimSpace(strings.TrimSuffix(seriesTitle, "-"))
	return seriesTitle, variant, ordinal
}

func splitHentaiGasmTargetURLs(target string) []string {
	trimmed := strings.TrimSpace(target)
	if strings.HasPrefix(trimmed, "[") {
		var list []string
		if json.Unmarshal([]byte(trimmed), &list) == nil {
			out := make([]string, 0, len(list))
			for _, u := range list {
				if clean := strings.TrimSpace(u); clean != "" {
					out = append(out, clean)
				}
			}
			return out
		}
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '|' || r == '\n'
	})
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{})
	for _, part := range parts {
		clean := strings.TrimSpace(part)
		if clean == "" {
			continue
		}
		if _, dup := seen[clean]; dup {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func fetchHentaiGasmEpisodes(ctx context.Context, epURLs []string, strictPublicStreams bool) ([]HentaiEpisodeItem, error) {
	type epResult struct {
		item HentaiEpisodeItem
		ok   bool
	}
	results := make([]epResult, len(epURLs))
	sem := make(chan struct{}, hentaiMaxConcurrency)
	var wg sync.WaitGroup

	for idx, epURL := range epURLs {
		wg.Add(1)
		go func(i int, targetEpURL string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			epHTML, err := fetchHentaiHTML(ctx, targetEpURL, originReferer(targetEpURL))
			if err != nil {
				return
			}
			mp4Matches := reGasmMP4File.FindAllStringSubmatch(epHTML, -1)
			if len(mp4Matches) == 0 {
				return
			}
			item := HentaiEpisodeItem{
				Ordinal: extractHentaiGasmOrdinalFromURL(targetEpURL, i+1),
			}
			item.Name = fmt.Sprintf("Episode %d", item.Ordinal)
			sources := make([]jwPlayerSource, 0, len(mp4Matches))
			for _, m := range mp4Matches {
				if len(m) > 1 {
					sources = append(sources, jwPlayerSource{File: m[1], Type: "mp4"})
				}
			}
			assignMP4Qualities(&item, sources, originReferer(targetEpURL), strictPublicStreams)
			if item.MP41080 != "" || item.MP4720 != "" || item.MP4480 != "" {
				results[i] = epResult{item: item, ok: true}
			}
		}(idx, epURL)
	}
	wg.Wait()

	out := make([]HentaiEpisodeItem, 0, len(results))
	for _, r := range results {
		if r.ok {
			out = append(out, r.item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Ordinal < out[j].Ordinal
	})
	return out, nil
}

func extractHentaiGasmOrdinalFromURL(epURL string, fallback int) int {
	slug := strings.Trim(epURL, "/")
	if idx := strings.LastIndex(slug, "/"); idx >= 0 {
		slug = slug[idx+1:]
	}
	slug = strings.ReplaceAll(slug, "-", " ")
	_, _, ord := splitHentaiGasmEpisodeTitle(slug)
	if ord > 0 && strings.Contains(slug, strconv.Itoa(ord)) {
		return ord
	}
	return fallback
}

func extractTrailingOrdinal(raw string, fallback int) int {
	if m := reTrailingEpNum.FindStringSubmatch(raw); len(m) > 1 {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func sanitizeAndProxyHentaiStream(rawStream string, referer string, strictPublic bool) (string, error) {
	unescaped := html.UnescapeString(strings.TrimSpace(rawStream))
	unescaped = strings.ReplaceAll(unescaped, `\/`, `/`)
	unescaped = normalizeEmbedURL(unescaped)
	unescaped = strings.ReplaceAll(unescaped, " ", "%20")
	if unescaped == "" {
		return "", errors.New("empty stream URL")
	}
	if strictPublic {
		if _, err := validatePublicStreamURL(unescaped); err != nil {
			return "", err
		}
	} else {
		parsed, err := url.Parse(unescaped)
		if err != nil || parsed.Host == "" {
			return "", errors.New("invalid stream URL")
		}
	}
	return buildStreamProxyURL(unescaped, referer), nil
}

func originReferer(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" || parsed.Scheme == "" {
		return "https://hentaimama.io/"
	}
	return parsed.Scheme + "://" + parsed.Host + "/"
}
