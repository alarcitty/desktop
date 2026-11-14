package mila

import (
	"bytes"
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
		for _, route := range routes {
			if r.URL.Path == route.prefix || strings.HasPrefix(r.URL.Path, route.prefix+"/") {
				serveProxyRoute(w, r, route)
				return
			}
		}
		http.NotFound(w, r)
	})
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
