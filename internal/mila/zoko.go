package mila

import (
	"context"
	"encoding/base64"
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
)

const (
	hianimeBaseURL = "https://hianime.at"
	zokoBaseURL    = "https://zokoanime.video"
	zokoRefererURL = "https://zokoanime.video/"
	zokoXORKey     = "otaku-embed-v1"
	zokoCacheTTL   = 15 * time.Minute
)

// ZokoSubtitle represents a VTT subtitle track attached to a ZokoAnime stream.
type ZokoSubtitle struct {
	Lang    string `json:"lang"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
	Src     string `json:"src"`
}

// ZokoResolved holds the decoded master HLS playlist URL and subtitles for a ZokoAnime episode.
type ZokoResolved struct {
	MasterURL string         `json:"masterUrl"`
	Referer   string         `json:"referer"`
	Subtitles []ZokoSubtitle `json:"subtitles,omitempty"`
}

// ZokoProbeResult describes available English Dub / Sub streams and episode ordinals for a release.
type ZokoProbeResult struct {
	MalID    int   `json:"malId"`
	HasDub   bool  `json:"hasDub"`
	HasSub   bool  `json:"hasSub"`
	Episodes []int `json:"episodes"`
}

type zokoCacheEntry struct {
	resolved  ZokoResolved
	expiresAt time.Time
}

type zokoProbeCacheEntry struct {
	result    ZokoProbeResult
	expiresAt time.Time
}

var (
	zokoCacheMu      sync.RWMutex
	zokoStreamCache  = make(map[string]zokoCacheEntry)
	zokoProbeCache   = make(map[string]zokoProbeCacheEntry)
	reZokoWindowBlob = regexp.MustCompile(`window\.__P\s*=\s*"([^"]+)"`)
	reHiSearchItem   = regexp.MustCompile(`(?s)<h3 class="film-name">\s*<a href="[^"]*/([^"/]+)"\s*title="([^"]+)"`)
	reHiEpisodeItem  = regexp.MustCompile(`data-number="(\d+)"[^>]*data-id="(\d+)"`)
	reHiZokoHash     = regexp.MustCompile(`data-type="(dub|sub)"[^>]*data-server-name="ZokoAnime"[^>]*data-hash="([^"]+)"`)
	reZokoMalPath    = regexp.MustCompile(`/mal/(\d+)/`)
)

// ZokoProbe checks English Dub & Sub availability on ZokoAnime / HiAnime for desktop & web callers.
func (m *Mila) ZokoProbe(malID int, episodeCount int, query string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := probeZokoRelease(ctx, hianimeBaseURL, zokoBaseURL, malID, episodeCount, query)
	if err != nil {
		return `{"malId":0,"hasDub":false,"hasSub":false,"episodes":[]}`, err
	}
	raw, err := json.Marshal(res)
	if err != nil {
		return `{"malId":0,"hasDub":false,"hasSub":false,"episodes":[]}`, err
	}
	return string(raw), nil
}

func serveZokoProbe(w http.ResponseWriter, r *http.Request) {
	setProxyCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	q := r.URL.Query()
	malID, _ := strconv.Atoi(strings.TrimSpace(q.Get("mal")))
	eps, _ := strconv.Atoi(strings.TrimSpace(q.Get("eps")))
	query := strings.TrimSpace(q.Get("q"))

	res, err := probeZokoRelease(r.Context(), hianimeBaseURL, zokoBaseURL, malID, eps, query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(res)
}

func serveZokoResolveJSON(w http.ResponseWriter, r *http.Request) {
	setProxyCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	q := r.URL.Query()
	malID, _ := strconv.Atoi(strings.TrimSpace(q.Get("mal")))
	ep, _ := strconv.Atoi(strings.TrimSpace(q.Get("ep")))
	mode := normalizeZokoMode(q.Get("mode"))

	resolved, err := resolveZokoEpisode(r.Context(), zokoBaseURL, malID, ep, mode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resolved)
}

func serveZokoMasterPlaylist(w http.ResponseWriter, r *http.Request) {
	setProxyCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	malID, _ := strconv.Atoi(strings.TrimSpace(q.Get("mal")))
	ep, _ := strconv.Atoi(strings.TrimSpace(q.Get("ep")))
	mode := normalizeZokoMode(q.Get("mode"))

	resolved, err := resolveZokoEpisode(r.Context(), zokoBaseURL, malID, ep, mode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	masterURL, err := validatePublicStreamURL(resolved.MasterURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	ref := resolved.Referer
	if ref == "" {
		ref = zokoRefererURL
	}
	fetchAndServePlaylist(w, r, masterURL, ref)
}

func normalizeZokoMode(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "sub") {
		return "sub"
	}
	return "dub"
}

func deobfuscateZokoBlob(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil {
			return nil, fmt.Errorf("zoko base64 decode failed: %w", err)
		}
	}
	key := []byte(zokoXORKey)
	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = b ^ key[i%len(key)]
	}
	return out, nil
}

func resolveZokoEpisode(ctx context.Context, baseZokoURL string, malID int, ep int, mode string) (*ZokoResolved, error) {
	if malID <= 0 {
		return nil, errors.New("invalid mal id")
	}
	if ep <= 0 {
		ep = 1
	}
	cleanMode := normalizeZokoMode(mode)
	base := strings.TrimRight(baseZokoURL, "/")
	cacheKey := fmt.Sprintf("%s|%d|%d|%s", base, malID, ep, cleanMode)

	zokoCacheMu.RLock()
	if entry, ok := zokoStreamCache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		cached := entry.resolved
		zokoCacheMu.RUnlock()
		return &cached, nil
	}
	zokoCacheMu.RUnlock()

	embedURL := fmt.Sprintf("%s/stream/mal/%d/%d/%s", base, malID, ep, cleanMode)
	ref := originReferer(embedURL)
	htmlBody, status, err := fetchTextWithHeaders(ctx, embedURL, map[string]string{
		"User-Agent": defaultBrowserUA,
		"Referer":    ref,
	})
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("zoko embed HTTP %d", status)
	}

	m := reZokoWindowBlob.FindStringSubmatch(htmlBody)
	if len(m) < 2 {
		return nil, errors.New("zoko stream blob not found")
	}
	decrypted, err := deobfuscateZokoBlob(m[1])
	if err != nil {
		return nil, err
	}

	var payload struct {
		Src       string         `json:"src"`
		Subtitles []ZokoSubtitle `json:"subtitles"`
	}
	if err := json.Unmarshal(decrypted, &payload); err != nil {
		return nil, fmt.Errorf("zoko json unmarshal failed: %w", err)
	}
	masterSrc := normalizeEmbedURL(payload.Src)
	if !strings.Contains(strings.ToLower(masterSrc), ".m3u8") {
		return nil, errors.New("zoko payload missing m3u8 src")
	}

	resolved := ZokoResolved{
		MasterURL: masterSrc,
		Referer:   ref,
		Subtitles: payload.Subtitles,
	}

	zokoCacheMu.Lock()
	zokoStreamCache[cacheKey] = zokoCacheEntry{
		resolved:  resolved,
		expiresAt: time.Now().Add(zokoCacheTTL),
	}
	zokoCacheMu.Unlock()
	return &resolved, nil
}

func probeZokoRelease(
	ctx context.Context,
	baseHiURL string,
	baseZokoURL string,
	malID int,
	episodeCount int,
	query string,
) (*ZokoProbeResult, error) {
	cleanQuery := strings.TrimSpace(query)
	cacheKey := fmt.Sprintf("%s|%s|%d|%d|%s", baseHiURL, baseZokoURL, malID, episodeCount, strings.ToLower(cleanQuery))

	zokoCacheMu.RLock()
	if entry, ok := zokoProbeCache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		cached := entry.result
		zokoCacheMu.RUnlock()
		return &cached, nil
	}
	zokoCacheMu.RUnlock()

	var episodes []int
	resolvedMalID := malID

	// If malID or episodeCount is missing, query HiAnime catalog to discover episodes and/or malID.
	if (resolvedMalID <= 0 || episodeCount <= 0) && len([]rune(cleanQuery)) >= 2 {
		hiMalID, hiEps, hiHasDub, hiHasSub, hiErr := discoverFromHiAnime(ctx, baseHiURL, cleanQuery)
		if hiErr == nil {
			if resolvedMalID <= 0 && hiMalID > 0 {
				resolvedMalID = hiMalID
			}
			if len(hiEps) > 0 {
				episodes = hiEps
			}
			if resolvedMalID > 0 && (hiHasDub || hiHasSub) && len(episodes) > 0 {
				res := ZokoProbeResult{
					MalID:    resolvedMalID,
					HasDub:   hiHasDub,
					HasSub:   hiHasSub,
					Episodes: episodes,
				}
				zokoCacheMu.Lock()
				zokoProbeCache[cacheKey] = zokoProbeCacheEntry{
					result:    res,
					expiresAt: time.Now().Add(zokoCacheTTL),
				}
				zokoCacheMu.Unlock()
				return &res, nil
			}
		}
	}

	if resolvedMalID <= 0 {
		return nil, errors.New("unable to resolve malId for zoko probe")
	}

	var hasDub, hasSub bool
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := resolveZokoEpisode(ctx, baseZokoURL, resolvedMalID, 1, "dub"); err == nil {
			hasDub = true
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := resolveZokoEpisode(ctx, baseZokoURL, resolvedMalID, 1, "sub"); err == nil {
			hasSub = true
		}
	}()
	wg.Wait()

	if !hasDub && !hasSub {
		return &ZokoProbeResult{MalID: resolvedMalID, Episodes: []int{}}, nil
	}

	if len(episodes) == 0 {
		count := episodeCount
		if count <= 0 && len([]rune(cleanQuery)) >= 2 {
			if _, hiEps, _, _, err := discoverFromHiAnime(ctx, baseHiURL, cleanQuery); err == nil && len(hiEps) > 0 {
				episodes = hiEps
			}
		}
		if len(episodes) == 0 {
			if count <= 0 {
				count = 12
			}
			if count > 200 {
				count = 200
			}
			episodes = make([]int, count)
			for i := 0; i < count; i++ {
				episodes[i] = i + 1
			}
		}
	}

	res := ZokoProbeResult{
		MalID:    resolvedMalID,
		HasDub:   hasDub,
		HasSub:   hasSub,
		Episodes: episodes,
	}
	zokoCacheMu.Lock()
	zokoProbeCache[cacheKey] = zokoProbeCacheEntry{
		result:    res,
		expiresAt: time.Now().Add(zokoCacheTTL),
	}
	zokoCacheMu.Unlock()
	return &res, nil
}

func discoverFromHiAnime(
	ctx context.Context,
	baseHiURL string,
	query string,
) (malID int, episodes []int, hasDub bool, hasSub bool, err error) {
	base := strings.TrimRight(baseHiURL, "/")
	searchURL := fmt.Sprintf("%s/search?keyword=%s", base, url.QueryEscape(query))
	searchHTML, status, fetchErr := fetchTextWithHeaders(ctx, searchURL, map[string]string{
		"User-Agent": defaultBrowserUA,
		"Referer":    base + "/",
	})
	if fetchErr != nil || status < 200 || status >= 300 {
		return 0, nil, false, false, fmt.Errorf("hianime search failed: %v (HTTP %d)", fetchErr, status)
	}
	if idx := strings.Index(searchHTML, `id="main-sidebar"`); idx > 0 {
		searchHTML = searchHTML[:idx]
	}

	m := reHiSearchItem.FindStringSubmatch(searchHTML)
	if len(m) < 2 {
		return 0, nil, false, false, errors.New("no hianime search results")
	}
	slug := strings.TrimSpace(m[1])
	parts := strings.Split(slug, "-")
	hiID := parts[len(parts)-1]
	if _, convErr := strconv.Atoi(hiID); convErr != nil {
		return 0, nil, false, false, errors.New("invalid hianime show id")
	}

	epListURL := fmt.Sprintf("%s/api/theme/episode/list/%s", base, hiID)
	epRaw, epStatus, epErr := fetchTextWithHeaders(ctx, epListURL, map[string]string{
		"User-Agent":       defaultBrowserUA,
		"Referer":          base + "/watch/" + slug,
		"X-Requested-With": "XMLHttpRequest",
	})
	if epErr != nil || epStatus < 200 || epStatus >= 300 {
		return 0, nil, false, false, fmt.Errorf("hianime episode list failed: %v", epErr)
	}
	epUnescaped := strings.ReplaceAll(html.UnescapeString(epRaw), `\`, ``)
	epMatches := reHiEpisodeItem.FindAllStringSubmatch(epUnescaped, -1)
	if len(epMatches) == 0 {
		return 0, nil, false, false, errors.New("no episodes found on hianime")
	}

	seenOrd := make(map[int]struct{})
	firstEpID := ""
	for _, em := range epMatches {
		if len(em) < 3 {
			continue
		}
		ord, ordErr := strconv.Atoi(em[1])
		if ordErr != nil || ord <= 0 {
			continue
		}
		if firstEpID == "" {
			firstEpID = em[2]
		}
		if _, dup := seenOrd[ord]; !dup {
			seenOrd[ord] = struct{}{}
			episodes = append(episodes, ord)
		}
	}
	sort.Ints(episodes)

	if firstEpID != "" {
		srvURL := fmt.Sprintf("%s/api/theme/episode/servers?episodeId=%s", base, firstEpID)
		srvRaw, srvStatus, srvErr := fetchTextWithHeaders(ctx, srvURL, map[string]string{
			"User-Agent":       defaultBrowserUA,
			"Referer":          base + "/watch/" + slug,
			"X-Requested-With": "XMLHttpRequest",
		})
		if srvErr == nil && srvStatus >= 200 && srvStatus < 300 {
			srvClean := strings.ReplaceAll(html.UnescapeString(srvRaw), `\"`, `"`)
			srvClean = strings.ReplaceAll(srvClean, `\/`, `/`)
			for _, sm := range reHiZokoHash.FindAllStringSubmatch(srvClean, -1) {
				if len(sm) < 3 {
					continue
				}
				kind := strings.ToLower(sm[1])
				decoded, decErr := base64.StdEncoding.DecodeString(sm[2])
				if decErr != nil {
					decoded, _ = base64.RawStdEncoding.DecodeString(sm[2])
				}
				embedStr := string(decoded)
				if malMatch := reZokoMalPath.FindStringSubmatch(embedStr); len(malMatch) > 1 {
					if parsedMal, pErr := strconv.Atoi(malMatch[1]); pErr == nil && parsedMal > 0 {
						malID = parsedMal
					}
				}
				if kind == "dub" {
					hasDub = true
				} else if kind == "sub" {
					hasSub = true
				}
			}
		}
	}

	return malID, episodes, hasDub, hasSub, nil
}
