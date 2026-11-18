package mila

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"mila/internal/aniliberty"
	"mila/internal/anime"
	"mila/internal/aniskip"
	"mila/internal/cache"
	"mila/internal/httpx"
	"mila/internal/server"
	"mila/internal/shikimori"
)

const (
	animeVostBaseURL      = "https://api.animevost.org/v1"
	dreamCastBaseURL      = "https://dreamerscast.com"
	dreamCastMediaBaseURL = "https://play.dreamerscast.com"
	aniLibriaAPIBaseURL   = "https://anilibria.top/api/v1"
	yaniAPIBaseURL        = "https://api.yani.tv"
	maxProxyBodyBytes     = 1 << 20
	maxPlaylistBytes      = 4 << 20
)

var (
	providerHTTPClient   = httpx.NewClient(12 * time.Second)
	mediaProxyHTTPClient = &http.Client{Transport: httpx.SharedTransport()}
	providerLimiter      = httpx.NewDomainLimiter(httpx.HostRule{RatePerSec: 10, Burst: 6}, nil)
	providerGetFlights   httpx.FlightGroup[httpx.ResponseResult]
	aniSkipOnce          sync.Once
	sharedAniSkip        *aniskip.Client
)

func getSharedAniSkipClient() *aniskip.Client {
	aniSkipOnce.Do(func() {
		dir, _ := cache.DefaultDiskCacheDir("aniskip")
		sharedAniSkip = aniskip.New(aniskip.Config{CacheDir: dir})
	})
	return sharedAniSkip
}

type providerRoute struct {
	prefix        string
	upstream      string
	rewriteM3U8   bool
}

func defaultProviderRoutes() []providerRoute {
	return []providerRoute{
		{prefix: "/ext/animevost", upstream: animeVostBaseURL},
		{prefix: "/ext/dreamerscast", upstream: dreamCastBaseURL},
		{prefix: "/ext/play-dc", upstream: dreamCastMediaBaseURL, rewriteM3U8: true},
		{prefix: "/ext/anilibria", upstream: aniLibriaAPIBaseURL},
		{prefix: "/ext/yani", upstream: yaniAPIBaseURL},
	}
}

func postProviderForm(endpoint string, params url.Values) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	encoded := params.Encode()
	res, err := httpx.DoWithRetry(ctx, providerHTTPClient, providerLimiter, httpx.RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   200 * time.Millisecond,
		MaxDelay:    3 * time.Second,
		MaxBody:     maxPlaylistBytes,
	}, func(rCtx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(rCtx, http.MethodPost, endpoint, strings.NewReader(encoded))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil
	})
	if err != nil {
		return "", err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("provider returned HTTP %d", res.StatusCode)
	}
	return string(res.Body), nil
}

// AnimeVostSearch queries AnimeVost catalog from the desktop Go host (avoids browser CORS limits).
func (m *Mila) AnimeVostSearch(query string) (string, error) {
	clean := strings.TrimSpace(query)
	if len(clean) < 2 {
		return `{"data":[]}`, nil
	}
	params := url.Values{}
	params.Set("name", clean)
	return postProviderForm(animeVostBaseURL+"/search", params)
}

// AnimeVostPlaylist fetches direct MP4 episode streams from AnimeVost for a release ID.
func (m *Mila) AnimeVostPlaylist(id int) (string, error) {
	if id <= 0 {
		return `[]`, nil
	}
	params := url.Values{}
	params.Set("id", strconv.Itoa(id))
	return postProviderForm(animeVostBaseURL+"/playlist", params)
}

// NewProviderHandler serves same-origin `/api/v1/*`, `/ext/*`, `/torrent/*`, and `/media/*` routes inside Wails.
func NewProviderHandler() http.Handler {
	return newProviderProxyHandler(defaultProviderRoutes())
}

func newDefaultAPIHandler() http.Handler {
	shikiCacheDir, _ := cache.DefaultDiskCacheDir("shikimori")
	libCacheDir, _ := cache.DefaultDiskCacheDir("aniliberty")
	shikiClient := shikimori.New(shikimori.Config{
		BaseURL:  os.Getenv("SHIKIMORI_BASE_URL"),
		Token:    os.Getenv("SHIKIMORI_TOKEN"),
		CacheDir: shikiCacheDir,
	})
	libClient := aniliberty.New(aniliberty.Config{
		BaseURL:  os.Getenv("ANILIBERTY_BASE_URL"),
		CacheDir: libCacheDir,
	})
	animeSvc := anime.NewService(libClient, shikiClient)
	return server.New(animeSvc).Handler()
}

func newProviderProxyHandler(routes []providerRoute) http.Handler {
	var apiOnce sync.Once
	var apiHandler http.Handler
	getAPIHandler := func() http.Handler {
		apiOnce.Do(func() {
			apiHandler = newDefaultAPIHandler()
		})
		return apiHandler
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || strings.HasPrefix(r.URL.Path, "/api/v1/") {
			getAPIHandler().ServeHTTP(w, r)
			return
		}
		switch r.URL.Path {
		case "/ext/stream":
			serveStreamProxy(w, r, false)
			return
		case "/ext/aniboom/master.m3u8":
			serveAniboomMasterPlaylist(w, r)
			return
		case "/ext/aniboom/resolve":
			serveAniboomResolveJSON(w, r)
			return
		case "/ext/kodik/master.m3u8":
			serveKodikMasterPlaylist(w, r)
			return
		case "/ext/kodik/resolve":
			serveKodikResolveJSON(w, r)
			return
		case "/ext/animego/search":
			serveAnimeGOSearch(w, r)
			return
		case "/ext/animego/player":
			serveAnimeGOPlayer(w, r)
			return
		case "/ext/hentai/search":
			serveHentaiSearch(w, r)
			return
		case "/ext/hentai/episodes":
			serveHentaiEpisodes(w, r)
			return
		case "/ext/zoko/probe":
			serveZokoProbe(w, r)
			return
		case "/ext/zoko/resolve":
			serveZokoResolveJSON(w, r)
			return
		case "/ext/zoko/master.m3u8":
			serveZokoMasterPlaylist(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/ext/aniskip/") || r.URL.Path == "/ext/aniskip" {
			serveAniSkipTimes(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/torrent/") {
			serveTorrentRoute(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/media/") {
			serveMediaRoute(w, r)
			return
		}
		for _, route := range routes {
			if r.URL.Path == route.prefix || strings.HasPrefix(r.URL.Path, route.prefix+"/") {
				serveProxyRoute(w, r, route)
				return
			}
		}
		http.NotFound(w, r)
	})
}

func setProxyCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")
}

func serveAnimeGOSearch(w http.ResponseWriter, r *http.Request) {
	setProxyCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	items, err := searchAnimeGO(r.Context(), animeGOBaseURL, q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(items)
}

func serveAniSkipTimes(w http.ResponseWriter, r *http.Request) {
	setProxyCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var malID, episode int
	trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/ext/aniskip"), "/")
	if trimmed != "" {
		parts := strings.Split(trimmed, "/")
		if len(parts) >= 2 {
			malID, _ = strconv.Atoi(parts[0])
			episode, _ = strconv.Atoi(parts[1])
		}
	}
	if malID <= 0 {
		malID, _ = strconv.Atoi(r.URL.Query().Get("malId"))
	}
	if episode <= 0 {
		episode, _ = strconv.Atoi(r.URL.Query().Get("episode"))
	}
	if malID <= 0 || episode <= 0 {
		http.Error(w, "invalid malId or episode", http.StatusBadRequest)
		return
	}

	res, err := getSharedAniSkipClient().GetSkipTimes(r.Context(), malID, episode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": res})
}

func serveAnimeGOPlayer(w http.ResponseWriter, r *http.Request) {
	setProxyCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	id, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("id")))
	if err != nil || id <= 0 {
		http.Error(w, "invalid anime id", http.StatusBadRequest)
		return
	}
	res, err := fetchAnimeGOPlayer(r.Context(), animeGOBaseURL, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(res)
}

func serveAniboomResolveJSON(w http.ResponseWriter, r *http.Request) {
	setProxyCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	embed := strings.TrimSpace(r.URL.Query().Get("embed"))
	ep, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("ep")))
	resolved, err := resolveAniboomEpisode(r.Context(), embed, ep)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resolved)
}

func serveAniboomMasterPlaylist(w http.ResponseWriter, r *http.Request) {
	setProxyCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	q := r.URL.Query()
	embed := strings.TrimSpace(q.Get("embed"))
	ep, _ := strconv.Atoi(strings.TrimSpace(q.Get("ep")))
	resolved, err := resolveAniboomEpisode(r.Context(), embed, ep)
	var masterURL *url.URL
	if err == nil {
		masterURL, err = validatePublicStreamURL(resolved.MasterURL)
	}
	if err != nil {
		// Fallback to paired Kodik embed when AniBoom fails or omits HLS for a translation.
		fallbackEmbed := strings.TrimSpace(q.Get("fallback"))
		animegoID, _ := strconv.Atoi(strings.TrimSpace(q.Get("animego_id")))
		epID, _ := strconv.Atoi(strings.TrimSpace(q.Get("ep_id")))
		trID := strings.TrimSpace(q.Get("tr"))
		if fallbackEmbed != "" || (epID > 0 && trID != "") {
			var kResolved *KodikResolved
			var kErr error
			if epID > 0 && trID != "" {
				kResolved, _, kErr = resolveAnimeGOKodikMaster(r.Context(), animegoID, epID, trID, fallbackEmbed)
			} else {
				kResolved, kErr = resolveKodikEpisode(r.Context(), fallbackEmbed, animeGORefererURL)
			}
			if kErr == nil && kResolved != nil && len(kResolved.Sources) > 0 {
				playlist := buildKodikMasterPlaylist(kResolved.Sources, kodikStreamReferer(fallbackEmbed))
				w.Header().Set("Content-Type", "application/vnd.apple.mpegurl; charset=utf-8")
				w.Header().Set("Content-Length", strconv.Itoa(len(playlist)))
				w.WriteHeader(http.StatusOK)
				if r.Method != http.MethodHead {
					_, _ = io.WriteString(w, playlist)
				}
				return
			}
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	fetchAndServePlaylist(w, r, masterURL, aniboomRefererURL)
}

func kodikStreamReferer(embed string) string {
	streamRef := "https://kodikplayer.com/"
	if parsed, parseErr := url.Parse(normalizeEmbedURL(embed)); parseErr == nil && parsed.Host != "" {
		streamRef = "https://" + parsed.Host + "/"
	}
	return streamRef
}

func resolveKodikFromHTTPRequest(r *http.Request) (*KodikResolved, string, error) {
	q := r.URL.Query()
	embed := strings.TrimSpace(q.Get("embed"))
	ref := strings.TrimSpace(q.Get("ref"))
	animegoID, _ := strconv.Atoi(strings.TrimSpace(q.Get("animego_id")))
	epID, _ := strconv.Atoi(strings.TrimSpace(q.Get("ep_id")))
	trID := strings.TrimSpace(q.Get("tr"))

	if epID > 0 && trID != "" {
		return resolveAnimeGOKodikMaster(r.Context(), animegoID, epID, trID, embed)
	}
	resolved, err := resolveKodikEpisode(r.Context(), embed, ref)
	if err != nil {
		return nil, "", err
	}
	return resolved, kodikStreamReferer(embed), nil
}

func serveKodikResolveJSON(w http.ResponseWriter, r *http.Request) {
	setProxyCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	resolved, _, err := resolveKodikFromHTTPRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resolved)
}

func serveKodikMasterPlaylist(w http.ResponseWriter, r *http.Request) {
	setProxyCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resolved, streamRef, err := resolveKodikFromHTTPRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	playlist := buildKodikMasterPlaylist(resolved.Sources, streamRef)
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(playlist)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = io.WriteString(w, playlist)
	}
}

func serveStreamProxy(w http.ResponseWriter, r *http.Request, allowPrivate bool) {
	setProxyCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rawTarget := r.URL.Query().Get("url")
	var targetURL *url.URL
	var err error
	if allowPrivate {
		targetURL, err = url.Parse(strings.TrimSpace(rawTarget))
		if err == nil && (targetURL.Scheme != "http" && targetURL.Scheme != "https" || targetURL.Host == "") {
			err = fmt.Errorf("invalid stream URL")
		}
	} else {
		targetURL, err = validatePublicStreamURL(rawTarget)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	referer := strings.TrimSpace(r.URL.Query().Get("ref"))
	if referer == "" {
		referer = targetURL.Scheme + "://" + targetURL.Host + "/"
	}
	if strings.HasSuffix(strings.ToLower(targetURL.Path), ".m3u8") {
		fetchAndServePlaylist(w, r, targetURL, referer)
		return
	}

	upReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), nil)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	upReq.Header.Set("User-Agent", defaultBrowserUA)
	upReq.Header.Set("Referer", referer)
	if refParsed, parseErr := url.Parse(referer); parseErr == nil && refParsed.Host != "" {
		upReq.Header.Set("Origin", refParsed.Scheme+"://"+refParsed.Host)
	}
	if rng := r.Header.Get("Range"); rng != "" {
		upReq.Header.Set("Range", rng)
	}

	resp, err := mediaProxyHTTPClient.Do(upReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "" {
		w.Header().Set("Cache-Control", cc)
	}
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		w.Header().Set("Content-Range", cr)
	}
	if ar := resp.Header.Get("Accept-Ranges"); ar != "" {
		w.Header().Set("Accept-Ranges", ar)
	}
	if strings.Contains(strings.ToLower(ct), "mpegurl") && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxPlaylistBytes))
		if readErr != nil {
			http.Error(w, "failed reading playlist", http.StatusBadGateway)
			return
		}
		rewritten := rewriteStreamPlaylist(string(raw), targetURL, referer)
		w.Header().Set("Content-Length", strconv.Itoa(len(rewritten)))
		w.WriteHeader(resp.StatusCode)
		if r.Method != http.MethodHead {
			_, _ = io.WriteString(w, rewritten)
		}
		return
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, resp.Body)
	}
}

func fetchAndServePlaylist(w http.ResponseWriter, r *http.Request, playlistURL *url.URL, referer string) {
	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, playlistURL.String(), nil)
	if err != nil {
		http.Error(w, "playlist request failed", http.StatusBadGateway)
		return
	}
	upReq.Header.Set("User-Agent", defaultBrowserUA)
	upReq.Header.Set("Referer", referer)
	if refParsed, parseErr := url.Parse(referer); parseErr == nil && refParsed.Host != "" {
		upReq.Header.Set("Origin", refParsed.Scheme+"://"+refParsed.Host)
	}
	resp, err := mediaProxyHTTPClient.Do(upReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("playlist upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		http.Error(w, fmt.Sprintf("playlist upstream HTTP %d", resp.StatusCode), http.StatusBadGateway)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxPlaylistBytes))
	if err != nil {
		http.Error(w, "failed reading playlist", http.StatusBadGateway)
		return
	}
	rewritten := rewriteStreamPlaylist(string(raw), playlistURL, referer)
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(rewritten)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = io.WriteString(w, rewritten)
	}
}

func serveProxyRoute(w http.ResponseWriter, r *http.Request, route providerRoute) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	upstreamURL, err := url.Parse(strings.TrimRight(route.upstream, "/"))
	if err != nil {
		http.Error(w, "invalid upstream config", http.StatusInternalServerError)
		return
	}

	suffix := strings.TrimPrefix(r.URL.Path, route.prefix)
	if suffix == "" {
		suffix = "/"
	}
	if !strings.HasPrefix(suffix, "/") || strings.HasPrefix(suffix, "//") {
		http.Error(w, "invalid proxy path", http.StatusBadRequest)
		return
	}

	targetStr := upstreamURL.String() + suffix
	if r.URL.RawQuery != "" {
		targetStr += "?" + r.URL.RawQuery
	}
	targetURL, err := url.Parse(targetStr)
	if err != nil || targetURL.Host != upstreamURL.Host || targetURL.Scheme != upstreamURL.Scheme {
		http.Error(w, "forbidden proxy target", http.StatusBadRequest)
		return
	}

	var bodyReader io.Reader
	if r.Body != nil && r.Method == http.MethodPost {
		payload, readErr := io.ReadAll(io.LimitReader(r.Body, maxProxyBodyBytes))
		if readErr != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		bodyReader = bytes.NewReader(payload)
	}

	upReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), bodyReader)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	upReq.Header.Set("User-Agent", "mila/1.0")
	if ct := r.Header.Get("Content-Type"); ct != "" {
		upReq.Header.Set("Content-Type", ct)
	}
	if rng := r.Header.Get("Range"); rng != "" {
		upReq.Header.Set("Range", rng)
	}

	resp, err := mediaProxyHTTPClient.Do(upReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "" {
		w.Header().Set("Cache-Control", cc)
	}
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		w.Header().Set("Content-Range", cr)
	}
	if ar := resp.Header.Get("Accept-Ranges"); ar != "" {
		w.Header().Set("Accept-Ranges", ar)
	}

	isM3U8 := route.rewriteM3U8 && (strings.HasSuffix(strings.ToLower(r.URL.Path), ".m3u8") ||
		strings.Contains(strings.ToLower(ct), "mpegurl"))

	if isM3U8 && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxPlaylistBytes))
		if readErr != nil {
			http.Error(w, "failed reading playlist", http.StatusBadGateway)
			return
		}
		rewritten := rewritePlaylistURLs(string(raw), route.upstream, route.prefix)
		w.Header().Set("Content-Length", strconv.Itoa(len(rewritten)))
		w.WriteHeader(resp.StatusCode)
		_, _ = io.WriteString(w, rewritten)
		return
	}

	if cl := resp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func rewritePlaylistURLs(playlist string, upstream string, proxyPrefix string) string {
	trimmed := strings.TrimRight(upstream, "/")
	replacement := strings.TrimRight(proxyPrefix, "/") + "/"
	out := strings.ReplaceAll(playlist, trimmed+"/", replacement)
	if strings.HasPrefix(trimmed, "https://") {
		out = strings.ReplaceAll(out, "http://"+strings.TrimPrefix(trimmed, "https://")+"/", replacement)
	}
	return out
}
