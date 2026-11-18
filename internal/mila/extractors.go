package mila

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
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
	animeGOBaseURL       = "https://animego.org"
	animeGORefererURL    = "https://animego.me/"
	aniboomRefererURL    = "https://aniboom.one/"
	defaultBrowserUA     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	aniboomCacheTTL      = 10 * time.Minute
	maxExtractorBodySize = 8 * 1024 * 1024
)

var reHTMLTag = regexp.MustCompile(`<[^>]+>`)

// AnimeGOSearchItem represents one matched anime entry from AnimeGO search HTML.
type AnimeGOSearchItem struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"originalTitle,omitempty"`
	Year          int    `json:"year,omitempty"`
	Poster        string `json:"poster,omitempty"`
	Slug          string `json:"slug"`
	URL           string `json:"url"`
}

// AnimeGOEpisodeItem represents a numbered episode available on AnimeGO.
type AnimeGOEpisodeItem struct {
	Ordinal   int `json:"ordinal"`
	EpisodeID int `json:"episodeId"`
}

// AnimeGODubItem represents an extractable voiceover or subtitle track (AniBoom or Kodik).
type AnimeGODubItem struct {
	TranslationID string `json:"translationId"`
	Title         string `json:"title"`
	ProviderSlug  string `json:"providerSlug"`
	EmbedURL      string `json:"embedUrl"`
}

// AnimeGOPlayerResult aggregates episodes and direct-extractable dubs for an anime ID.
type AnimeGOPlayerResult struct {
	AnimeID  int                  `json:"animeId"`
	Episodes []AnimeGOEpisodeItem `json:"episodes"`
	Dubs     []AnimeGODubItem     `json:"dubs"`
}

// AniboomResolved holds the extracted direct HLS playlist and metadata from an Aniboom embed.
type AniboomResolved struct {
	MasterURL        string  `json:"masterUrl"`
	Duration         float64 `json:"duration"`
	Poster           string  `json:"poster,omitempty"`
	QualityVideo     int     `json:"qualityVideo,omitempty"`
	TranslationTitle string  `json:"translationTitle,omitempty"`
}

type aniboomCacheEntry struct {
	resolved  AniboomResolved
	expiresAt time.Time
}

var (
	aniboomCacheMu sync.RWMutex
	aniboomCache   = make(map[string]aniboomCacheEntry)

	reAnimeGOCardHref   = regexp.MustCompile(`href="(?:https://animego\.(?:org|me))?(/anime/(?:[a-z0-9-]+-)(\d+))"`)
	reAnimeGOImgSrc     = regexp.MustCompile(`<img[^>]+class="[^"]*image__img[^"]*"[^>]+src="([^"]+)"`)
	reAnimeGOAltTitle   = regexp.MustCompile(`(?s)<div class="fw-lighter small mb-1 text-line-clamp"[^>]*>(.*?)</div>`)
	reAnimeGOMainTitle  = regexp.MustCompile(`(?s)<div class="ani-grid__item-title[^"]*"[^>]*>.*?title="([^"]+)"`)
	reAnimeGOYear       = regexp.MustCompile(`>\s*(19\d{2}|20\d{2})\s*<`)
	reEpisodeBtn        = regexp.MustCompile(`(?s)data-episode-number="(\d+)"[^>]*?data-episode="(\d+)"|data-episode="(\d+)"[^>]*?data-episode-number="(\d+)"`)
	reProviderBtn       = regexp.MustCompile(`(?s)<button[^>]*data-anime-player-target="provider"[^>]*>`)
	reAttrPlayer        = regexp.MustCompile(`data-player="([^"]+)"`)
	reAttrProviderSlug  = regexp.MustCompile(`data-provider-slug="([^"]+)"`)
	reAttrTranslationID = regexp.MustCompile(`data-translation-id="([^"]+)"`)
	reAttrTranslationT  = regexp.MustCompile(`data-translation-title="([^"]+)"`)
	reAniboomParams     = regexp.MustCompile(`data-parameters="([^"]+)"`)
	reSibnetSrc         = regexp.MustCompile(`player\.src\(\[\s*\{\s*src:\s*["']([^"']+\.mp4[^"']*)["']`)
	reM3U8URIAttr       = regexp.MustCompile(`URI="([^"]+)"`)
)

// AnimeGOSearch queries AnimeGO and parses anime search cards for desktop & web callers.
func (m *Mila) AnimeGOSearch(query string) (string, error) {
	items, err := searchAnimeGO(context.Background(), animeGOBaseURL, query)
	if err != nil {
		return "[]", err
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return "[]", err
	}
	return string(raw), nil
}

// AnimeGOPlayer fetches episodes and non-Kodik AniBoom dubs for a specific AnimeGO ID.
func (m *Mila) AnimeGOPlayer(animeID int) (string, error) {
	res, err := fetchAnimeGOPlayer(context.Background(), animeGOBaseURL, animeID)
	if err != nil {
		return `{"animeId":0,"episodes":[],"dubs":[]}`, err
	}
	raw, err := json.Marshal(res)
	if err != nil {
		return `{"animeId":0,"episodes":[],"dubs":[]}`, err
	}
	return string(raw), nil
}

func searchAnimeGO(ctx context.Context, baseURL string, query string) ([]AnimeGOSearchItem, error) {
	clean := strings.TrimSpace(query)
	if len([]rune(clean)) < 2 {
		return []AnimeGOSearchItem{}, nil
	}
	searchURL := strings.TrimRight(baseURL, "/") + "/search/anime?q=" + url.QueryEscape(clean)
	body, status, err := fetchTextWithHeaders(ctx, searchURL, map[string]string{
		"User-Agent": defaultBrowserUA,
		"Referer":    strings.TrimRight(baseURL, "/") + "/",
	})
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("animego search HTTP %d", status)
	}
	return parseAnimeGOSearchHTML(baseURL, body), nil
}

func parseAnimeGOSearchHTML(baseURL string, rawHTML string) []AnimeGOSearchItem {
	matches := reAnimeGOCardHref.FindAllStringSubmatchIndex(rawHTML, -1)
	if len(matches) == 0 {
		return []AnimeGOSearchItem{}
	}
	seenIDs := make(map[int]struct{})
	items := make([]AnimeGOSearchItem, 0, len(matches))

	for i, loc := range matches {
		pathStr := rawHTML[loc[2]:loc[3]]
		if strings.HasPrefix(pathStr, "/anime/top-") ||
			strings.HasPrefix(pathStr, "/anime/status/") ||
			strings.HasPrefix(pathStr, "/anime/season/") ||
			strings.HasPrefix(pathStr, "/anime/random") {
			continue
		}
		idVal, err := strconv.Atoi(rawHTML[loc[4]:loc[5]])
		if err != nil || idVal <= 0 {
			continue
		}
		if _, dup := seenIDs[idVal]; dup {
			continue
		}
		seenIDs[idVal] = struct{}{}

		start := loc[0]
		end := min(len(rawHTML), start+2400)
		for j := i + 1; j < len(matches); j++ {
			nextID, nextErr := strconv.Atoi(rawHTML[matches[j][4]:matches[j][5]])
			if nextErr == nil && nextID != idVal && matches[j][0] > start {
				if matches[j][0] < end {
					end = matches[j][0]
				}
				break
			}
		}
		chunk := rawHTML[start:end]

		title := ""
		if m := reAnimeGOMainTitle.FindStringSubmatch(chunk); len(m) > 1 {
			title = cleanHTMLText(m[1])
		}
		origTitle := ""
		if m := reAnimeGOAltTitle.FindStringSubmatch(chunk); len(m) > 1 {
			origTitle = cleanHTMLText(m[1])
		}
		if title == "" {
			title = origTitle
		}
		poster := ""
		if m := reAnimeGOImgSrc.FindStringSubmatch(chunk); len(m) > 1 {
			poster = html.UnescapeString(strings.TrimSpace(m[1]))
		}
		year := 0
		if m := reAnimeGOYear.FindStringSubmatch(chunk); len(m) > 1 {
			if y, yErr := strconv.Atoi(m[1]); yErr == nil {
				year = y
			}
		}

		items = append(items, AnimeGOSearchItem{
			ID:            idVal,
			Title:         title,
			OriginalTitle: origTitle,
			Year:          year,
			Poster:        poster,
			Slug:          strings.TrimPrefix(pathStr, "/anime/"),
			URL:           strings.TrimRight(baseURL, "/") + pathStr,
		})
	}
	return items
}

func fetchAnimeGOPlayer(ctx context.Context, baseURL string, animeID int) (*AnimeGOPlayerResult, error) {
	if animeID <= 0 {
		return nil, errors.New("invalid anime id")
	}
	base := strings.TrimRight(baseURL, "/")
	ajaxHeaders := map[string]string{
		"User-Agent":       defaultBrowserUA,
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          base + "/",
	}

	type ajaxResp struct {
		Status string `json:"status"`
		Data   struct {
			Content string `json:"content"`
		} `json:"data"`
	}

	var playerHTML, episodesHTML string
	var playerErr, epErr error
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		raw, status, err := fetchTextWithHeaders(ctx, fmt.Sprintf("%s/player/%d", base, animeID), ajaxHeaders)
		if err != nil {
			playerErr = err
			return
		}
		if status < 200 || status >= 300 {
			playerErr = fmt.Errorf("animego player HTTP %d", status)
			return
		}
		var parsed ajaxResp
		if jsonErr := json.Unmarshal([]byte(raw), &parsed); jsonErr == nil && parsed.Data.Content != "" {
			playerHTML = parsed.Data.Content
		} else {
			playerHTML = raw
		}
	}()

	go func() {
		defer wg.Done()
		raw, status, err := fetchTextWithHeaders(ctx, fmt.Sprintf("%s/player/%d/episodes", base, animeID), ajaxHeaders)
		if err != nil {
			epErr = err
			return
		}
		if status < 200 || status >= 300 {
			epErr = fmt.Errorf("animego episodes HTTP %d", status)
			return
		}
		var parsed ajaxResp
		if jsonErr := json.Unmarshal([]byte(raw), &parsed); jsonErr == nil && parsed.Data.Content != "" {
			episodesHTML = parsed.Data.Content
		} else {
			episodesHTML = raw
		}
	}()

	wg.Wait()
	if playerErr != nil {
		return nil, playerErr
	}
	if epErr != nil {
		return nil, epErr
	}

	return parseAnimeGOPlayerHTML(animeID, playerHTML, episodesHTML), nil
}

func parseAnimeGOPlayerHTML(animeID int, playerHTML string, episodesHTML string) *AnimeGOPlayerResult {
	epMap := make(map[int]int)
	for _, m := range reEpisodeBtn.FindAllStringSubmatch(episodesHTML, -1) {
		ordStr, idStr := m[1], m[2]
		if ordStr == "" {
			ordStr, idStr = m[4], m[3]
		}
		ord, err1 := strconv.Atoi(ordStr)
		epID, err2 := strconv.Atoi(idStr)
		if err1 == nil && err2 == nil && ord > 0 {
			epMap[ord] = epID
		}
	}
	episodes := make([]AnimeGOEpisodeItem, 0, len(epMap))
	for ord, epID := range epMap {
		episodes = append(episodes, AnimeGOEpisodeItem{Ordinal: ord, EpisodeID: epID})
	}
	sort.Slice(episodes, func(i, j int) bool {
		return episodes[i].Ordinal < episodes[j].Ordinal
	})

	dubs := make([]AnimeGODubItem, 0)
	byTranslation := make(map[string]AnimeGODubItem)
	seenOther := make(map[string]struct{})
	for _, btn := range reProviderBtn.FindAllString(playerHTML, -1) {
		slugMatch := reAttrProviderSlug.FindStringSubmatch(btn)
		if len(slugMatch) < 2 {
			continue
		}
		slug := strings.ToLower(strings.TrimSpace(slugMatch[1]))
		if slug != "aniboom" && slug != "kodik" {
			continue
		}
		playerMatch := reAttrPlayer.FindStringSubmatch(btn)
		if len(playerMatch) < 2 {
			continue
		}
		embedURL := normalizeEmbedURL(html.UnescapeString(playerMatch[1]))
		if embedURL == "" {
			continue
		}
		trID := ""
		if m := reAttrTranslationID.FindStringSubmatch(btn); len(m) > 1 {
			trID = strings.TrimSpace(m[1])
		}
		trTitle := "AniBoom"
		if slug == "kodik" {
			trTitle = "Kodik"
		}
		if m := reAttrTranslationT.FindStringSubmatch(btn); len(m) > 1 {
			if cleaned := cleanHTMLText(m[1]); cleaned != "" {
				trTitle = cleaned
			}
		}
		item := AnimeGODubItem{
			TranslationID: trID,
			Title:         trTitle,
			ProviderSlug:  slug,
			EmbedURL:      embedURL,
		}
		if trID != "" {
			if existing, exists := byTranslation[trID]; exists {
				// Prefer 1080p AniBoom over 720p Kodik for the same translation ID.
				if existing.ProviderSlug == "aniboom" || slug != "aniboom" {
					continue
				}
			}
			byTranslation[trID] = item
		} else {
			dedupeKey := slug + ":" + embedURL
			if _, exists := seenOther[dedupeKey]; exists {
				continue
			}
			seenOther[dedupeKey] = struct{}{}
			dubs = append(dubs, item)
		}
	}
	for _, item := range byTranslation {
		dubs = append(dubs, item)
	}
	sort.SliceStable(dubs, func(i, j int) bool {
		if dubs[i].ProviderSlug != dubs[j].ProviderSlug {
			return dubs[i].ProviderSlug == "aniboom"
		}
		return dubs[i].Title < dubs[j].Title
	})

	return &AnimeGOPlayerResult{
		AnimeID:  animeID,
		Episodes: episodes,
		Dubs:     dubs,
	}
}

func resolveAniboomEpisode(ctx context.Context, rawEmbedURL string, episodeOrdinal int) (*AniboomResolved, error) {
	parsed, err := url.Parse(normalizeEmbedURL(rawEmbedURL))
	if err != nil || parsed.Host == "" {
		return nil, errors.New("invalid aniboom embed URL")
	}
	if !strings.EqualFold(parsed.Hostname(), "aniboom.one") &&
		!strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".aniboom.one") {
		return nil, errors.New("unsupported embed host")
	}
	q := parsed.Query()
	if episodeOrdinal > 0 {
		q.Set("episode", strconv.Itoa(episodeOrdinal))
	}
	parsed.RawQuery = q.Encode()
	targetURL := parsed.String()

	aniboomCacheMu.RLock()
	cached, ok := aniboomCache[targetURL]
	aniboomCacheMu.RUnlock()
	if ok && time.Now().Before(cached.expiresAt) {
		copyVal := cached.resolved
		return &copyVal, nil
	}

	body, status, err := fetchTextWithHeaders(ctx, targetURL, map[string]string{
		"User-Agent": defaultBrowserUA,
		"Referer":    animeGORefererURL,
	})
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("aniboom embed HTTP %d", status)
	}

	resolved, err := parseAniboomEmbedHTML(body)
	if err != nil {
		return nil, err
	}

	aniboomCacheMu.Lock()
	aniboomCache[targetURL] = aniboomCacheEntry{
		resolved:  *resolved,
		expiresAt: time.Now().Add(aniboomCacheTTL),
	}
	aniboomCacheMu.Unlock()

	return resolved, nil
}

func parseAniboomEmbedHTML(rawHTML string) (*AniboomResolved, error) {
	m := reAniboomParams.FindStringSubmatch(rawHTML)
	if len(m) < 2 {
		return nil, errors.New("aniboom data-parameters not found")
	}
	decoded := html.UnescapeString(m[1])

	var payload struct {
		Duration      float64 `json:"duration"`
		Poster        string  `json:"poster"`
		QualityVideo  int     `json:"qualityVideo"`
		HLS           string  `json:"hls"`
		FallbackHLS   string  `json:"fallbackHls"`
		EpisodePicker struct {
			TranslationTitle string `json:"translationTitle"`
		} `json:"episodePicker"`
	}
	if err := json.Unmarshal([]byte(decoded), &payload); err != nil {
		return nil, fmt.Errorf("aniboom params parse failed: %w", err)
	}

	var hlsSource struct {
		Src string `json:"src"`
	}
	if strings.TrimSpace(payload.HLS) != "" {
		_ = json.Unmarshal([]byte(payload.HLS), &hlsSource)
	}
	if strings.TrimSpace(hlsSource.Src) == "" && strings.TrimSpace(payload.FallbackHLS) != "" {
		_ = json.Unmarshal([]byte(payload.FallbackHLS), &hlsSource)
	}
	if strings.TrimSpace(hlsSource.Src) == "" {
		return nil, errors.New("aniboom embed missing HLS stream")
	}

	return &AniboomResolved{
		MasterURL:        normalizeEmbedURL(hlsSource.Src),
		Duration:         payload.Duration,
		Poster:           normalizeEmbedURL(payload.Poster),
		QualityVideo:     payload.QualityVideo,
		TranslationTitle: strings.TrimSpace(payload.EpisodePicker.TranslationTitle),
	}, nil
}

func extractSibnetMP4(ctx context.Context, embedURL string) (string, error) {
	normalized := normalizeEmbedURL(embedURL)
	parsed, err := url.Parse(normalized)
	if err != nil || !strings.HasSuffix(strings.ToLower(parsed.Hostname()), "sibnet.ru") {
		return "", errors.New("invalid sibnet embed URL")
	}
	body, status, err := fetchTextWithHeaders(ctx, normalized, map[string]string{
		"User-Agent": defaultBrowserUA,
		"Referer":    normalized,
	})
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("sibnet returned HTTP %d", status)
	}
	m := reSibnetSrc.FindStringSubmatch(body)
	if len(m) < 2 {
		return "", errors.New("sibnet mp4 source not found")
	}
	refURL, err := parsed.Parse(m[1])
	if err != nil {
		return "", err
	}
	return buildStreamProxyURL(refURL.String(), "https://video.sibnet.ru/"), nil
}

func rewriteStreamPlaylist(playlist string, baseURL *url.URL, referer string) string {
	lines := strings.Split(playlist, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			if strings.Contains(trimmed, `URI="`) {
				lines[i] = reM3U8URIAttr.ReplaceAllStringFunc(line, func(match string) string {
					sub := reM3U8URIAttr.FindStringSubmatch(match)
					if len(sub) < 2 {
						return match
					}
					resolved, err := baseURL.Parse(strings.TrimSpace(sub[1]))
					if err != nil {
						return match
					}
					return `URI="` + buildStreamProxyURL(resolved.String(), referer) + `"`
				})
			}
			continue
		}
		resolved, err := baseURL.Parse(trimmed)
		if err != nil {
			continue
		}
		lines[i] = buildStreamProxyURL(resolved.String(), referer)
	}
	return strings.Join(lines, "\n")
}

func buildStreamProxyURL(target string, referer string) string {
	v := url.Values{}
	v.Set("url", target)
	if strings.TrimSpace(referer) != "" {
		v.Set("ref", strings.TrimSpace(referer))
	}
	return "/ext/stream?" + v.Encode()
}

func validatePublicStreamURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return nil, errors.New("invalid stream URL")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, errors.New("unsupported stream scheme")
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".local") {
		return nil, errors.New("forbidden local stream host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return nil, errors.New("forbidden private stream address")
		}
	}
	return parsed, nil
}

func normalizeEmbedURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "//") {
		return "https:" + trimmed
	}
	return trimmed
}

func cleanHTMLText(raw string) string {
	withoutTags := reHTMLTag.ReplaceAllString(raw, "")
	return strings.Join(strings.Fields(html.UnescapeString(withoutTags)), " ")
}

func fetchTextWithHeaders(ctx context.Context, rawURL string, headers map[string]string) (string, int, error) {
	res, err := httpx.GetBytes(ctx, providerHTTPClient, providerLimiter, &providerGetFlights, rawURL, headers, httpx.RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   200 * time.Millisecond,
		MaxDelay:    3 * time.Second,
		MaxBody:     maxExtractorBodySize,
	})
	if err != nil {
		var statusErr *httpx.StatusError
		if errors.As(err, &statusErr) {
			return "", statusErr.StatusCode, err
		}
		return "", 0, err
	}
	return string(res.Body), res.StatusCode, nil
}
