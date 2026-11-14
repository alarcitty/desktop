package mila

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	animeVostBaseURL      = "https://api.animevost.org/v1"
	dreamCastBaseURL      = "https://dreamerscast.com"
	dreamCastMediaBaseURL = "https://play.dreamerscast.com"
	aniLibriaAPIBaseURL   = "https://anilibria.top/api/v1"
	maxProxyBodyBytes     = 1 << 20
	maxPlaylistBytes      = 4 << 20
)

var (
	providerHTTPClient = &http.Client{
		Timeout: 10 * time.Second,
	}
	mediaProxyHTTPClient = &http.Client{
		Timeout: 30 * time.Second,
	}
)

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
	}
}

func postProviderForm(endpoint string, params url.Values) (string, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := providerHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPlaylistBytes))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
	}
	return string(body), nil
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

// NewProviderHandler serves same-origin `/ext/*` reverse-proxy routes for provider APIs and HLS streams inside Wails.
func NewProviderHandler() http.Handler {
	return newProviderProxyHandler(defaultProviderRoutes())
}

func newProviderProxyHandler(routes []providerRoute) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		case "/ext/animego/search":
			serveAnimeGOSearch(w, r)
			return
		case "/ext/animego/player":
			serveAnimeGOPlayer(w, r)
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
	embed := strings.TrimSpace(r.URL.Query().Get("embed"))
	ep, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("ep")))
	resolved, err := resolveAniboomEpisode(r.Context(), embed, ep)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	masterURL, err := validatePublicStreamURL(resolved.MasterURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	fetchAndServePlaylist(w, r, masterURL, aniboomRefererURL)
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
