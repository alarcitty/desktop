package mila

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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

const (
	kodikDefaultReferer = "https://yani.tv/"
	kodikDefaultPath    = "/ftor"
	kodikDefaultShift   = 18
	kodikCacheTTL       = 10 * time.Minute
	kodikScriptCacheTTL = 30 * time.Minute
)

// KodikResolved holds decoded HLS stream URLs per quality along with the auto-detected API parameters.
type KodikResolved struct {
	Sources  map[string]string `json:"sources"`
	Endpoint string            `json:"endpoint,omitempty"`
	Shift    int               `json:"shift"`
}

type kodikScriptConfig struct {
	endpoint  string
	shift     int
	hasShift  bool
	fetchedAt time.Time
}

type kodikResolveCacheEntry struct {
	resolved  KodikResolved
	expiresAt time.Time
}

type kodikEmbedParams struct {
	domain     string
	dSign      string
	pd         string
	pdSign     string
	ref        string
	refSign    string
	vType      string
	vHash      string
	vID        string
	scriptPath string
}

var (
	kodikScriptMu    sync.RWMutex
	kodikScriptCache = make(map[string]kodikScriptConfig)

	kodikResolveMu    sync.RWMutex
	kodikResolveCache = make(map[string]kodikResolveCacheEntry)

	reKodikVarDomain   = regexp.MustCompile(`var\s+domain\s*=\s*["']([^"']*)["']`)
	reKodikVarDSign    = regexp.MustCompile(`var\s+d_sign\s*=\s*["']([^"']+)["']`)
	reKodikVarPD       = regexp.MustCompile(`var\s+pd\s*=\s*["']([^"']*)["']`)
	reKodikVarPDSign   = regexp.MustCompile(`var\s+pd_sign\s*=\s*["']([^"']+)["']`)
	reKodikVarRef      = regexp.MustCompile(`var\s+ref\s*=\s*["']([^"']*)["']`)
	reKodikVarRefSign  = regexp.MustCompile(`var\s+ref_sign\s*=\s*["']([^"']+)["']`)
	reKodikInfoType    = regexp.MustCompile(`(?:vInfo\.type|var\s+type)\s*=\s*["']([^"']+)["']`)
	reKodikInfoHash    = regexp.MustCompile(`(?:vInfo\.hash|var\s+hash)\s*=\s*["']([^"']+)["']`)
	reKodikInfoID      = regexp.MustCompile(`(?:vInfo\.id|var\s+id)\s*=\s*["']([^"']+)["']`)
	reKodikScriptSrc   = regexp.MustCompile(`src\s*=\s*["']([^"']*app\.player[^"']*\.js(?:\?[^"']*)?)["']`)
	reKodikAjaxAtob    = regexp.MustCompile(`\$\.ajax\(\s*\{[^}]*type:\s*["']POST["'][^}]*url:\s*atob\(\s*["']([A-Za-z0-9+/=]+)["']\s*\)`)
	reKodikAjaxLiteral = regexp.MustCompile(`\$\.ajax\(\s*\{[^}]*type:\s*["']POST["'][^}]*url:\s*["'](/[a-zA-Z0-9_-]{2,24})["']`)
	reKodikAnyAtob     = regexp.MustCompile(`atob\(\s*["']([A-Za-z0-9+/=]{4,36})["']\s*\)`)
	reKodikShiftExpr   = regexp.MustCompile(`charCodeAt\(\s*0\s*\)\s*([+-])\s*(\d+)`)
)

func isAllowedKodikHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	for _, allowed := range []string{"kodikplayer.com", "kodik.info", "kodik.cc", "aniqit.com"} {
		if h == allowed || strings.HasSuffix(h, "."+allowed) {
			return true
		}
	}
	return false
}

func resolveKodikEpisode(ctx context.Context, rawEmbedURL string, donorReferer string) (*KodikResolved, error) {
	parsed, err := url.Parse(normalizeEmbedURL(rawEmbedURL))
	if err != nil || parsed.Host == "" {
		return nil, errors.New("invalid kodik embed URL")
	}
	if !isAllowedKodikHost(parsed.Hostname()) {
		return nil, errors.New("unsupported kodik embed host")
	}
	targetURL := parsed.String()
	ref := strings.TrimSpace(donorReferer)
	if ref == "" {
		ref = kodikDefaultReferer
	}

	cacheKey := targetURL + "|" + ref
	kodikResolveMu.RLock()
	if cached, ok := kodikResolveCache[cacheKey]; ok && time.Now().Before(cached.expiresAt) {
		copyVal := cloneKodikResolved(cached.resolved)
		kodikResolveMu.RUnlock()
		return &copyVal, nil
	}
	kodikResolveMu.RUnlock()

	embedHTML, status, err := fetchTextWithHeaders(ctx, targetURL, map[string]string{
		"User-Agent": defaultBrowserUA,
		"Referer":    ref,
	})
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("kodik embed HTTP %d", status)
	}

	params, err := parseKodikEmbedHTML(embedHTML)
	if err != nil {
		return nil, err
	}

	cfg := getKodikScriptConfig(ctx, parsed, params.scriptPath, false)
	resolved, err := requestKodikLinks(ctx, parsed, targetURL, params, cfg)
	if err != nil && params.scriptPath != "" {
		// Self-heal: refresh JS config and try again (handles endpoint/cipher rotation).
		freshCfg := getKodikScriptConfig(ctx, parsed, params.scriptPath, true)
		if freshCfg.endpoint != cfg.endpoint || freshCfg.shift != cfg.shift || freshCfg.hasShift != cfg.hasShift {
			if retryRes, retryErr := requestKodikLinks(ctx, parsed, targetURL, params, freshCfg); retryErr == nil {
				resolved = retryRes
				err = nil
				cfg = freshCfg
			}
		}
		if err != nil {
			// Also try all candidate endpoints discovered inside the refreshed JS bundle.
			if jsBody := fetchKodikScriptBody(ctx, parsed, params.scriptPath); jsBody != "" {
				for _, altEndpoint := range extractKodikCandidateEndpoints(jsBody) {
					if altEndpoint == cfg.endpoint || altEndpoint == freshCfg.endpoint {
						continue
					}
					altCfg := kodikScriptConfig{endpoint: altEndpoint, shift: freshCfg.shift, hasShift: freshCfg.hasShift}
					if retryRes, retryErr := requestKodikLinks(ctx, parsed, targetURL, params, altCfg); retryErr == nil {
						updateKodikScriptCache(scriptCacheKey(parsed, params.scriptPath), altCfg)
						resolved = retryRes
						err = nil
						break
					}
				}
			}
		}
	}
	if err != nil {
		return nil, err
	}

	kodikResolveMu.Lock()
	kodikResolveCache[cacheKey] = kodikResolveCacheEntry{
		resolved:  cloneKodikResolved(*resolved),
		expiresAt: time.Now().Add(kodikCacheTTL),
	}
	kodikResolveMu.Unlock()

	return resolved, nil
}

func resolveAnimeGOKodikMaster(ctx context.Context, animeID int, episodeID int, translationID string, fallbackEmbed string) (*KodikResolved, string, error) {
	embedURL := strings.TrimSpace(fallbackEmbed)
	if episodeID > 0 && translationID != "" {
		found, err := fetchAnimeGOKodikEmbedForEpisode(ctx, animeGOBaseURL, episodeID, translationID)
		if err == nil && found != "" {
			embedURL = found
		}
	}
	if embedURL == "" && animeID > 0 && translationID != "" {
		if playerRes, err := fetchAnimeGOPlayer(ctx, animeGOBaseURL, animeID); err == nil {
			for _, dub := range playerRes.Dubs {
				if dub.ProviderSlug == "kodik" && dub.TranslationID == translationID && dub.EmbedURL != "" {
					embedURL = dub.EmbedURL
					break
				}
			}
		}
	}
	if embedURL == "" {
		return nil, "", errors.New("kodik embed URL not found for animego episode")
	}
	resolved, err := resolveKodikEpisode(ctx, embedURL, animeGORefererURL)
	if err != nil {
		return nil, "", err
	}
	parsedEmbed, _ := url.Parse(normalizeEmbedURL(embedURL))
	streamRef := "https://kodikplayer.com/"
	if parsedEmbed != nil && parsedEmbed.Host != "" {
		streamRef = "https://" + parsedEmbed.Host + "/"
	}
	return resolved, streamRef, nil
}

func fetchAnimeGOKodikEmbedForEpisode(ctx context.Context, baseURL string, episodeID int, translationID string) (string, error) {
	base := strings.TrimRight(baseURL, "/")
	raw, status, err := fetchTextWithHeaders(ctx, fmt.Sprintf("%s/player/%d", base, episodeID), map[string]string{
		"User-Agent":       defaultBrowserUA,
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          base + "/",
	})
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("animego episode player HTTP %d", status)
	}
	var parsed struct {
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	content := raw
	if json.Unmarshal([]byte(raw), &parsed) == nil && parsed.Data.Content != "" {
		content = parsed.Data.Content
	}
	for _, btn := range reProviderBtn.FindAllString(content, -1) {
		slugMatch := reAttrProviderSlug.FindStringSubmatch(btn)
		if len(slugMatch) < 2 || !strings.EqualFold(strings.TrimSpace(slugMatch[1]), "kodik") {
			continue
		}
		trMatch := reAttrTranslationID.FindStringSubmatch(btn)
		if len(trMatch) < 2 || strings.TrimSpace(trMatch[1]) != translationID {
			continue
		}
		playerMatch := reAttrPlayer.FindStringSubmatch(btn)
		if len(playerMatch) < 2 {
			continue
		}
		return normalizeEmbedURL(playerMatch[1]), nil
	}
	return "", errors.New("matching kodik translation not found on episode")
}

func parseKodikEmbedHTML(rawHTML string) (*kodikEmbedParams, error) {
	extract := func(re *regexp.Regexp) string {
		if m := re.FindStringSubmatch(rawHTML); len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
		return ""
	}
	p := &kodikEmbedParams{
		domain:     extract(reKodikVarDomain),
		dSign:      extract(reKodikVarDSign),
		pd:         extract(reKodikVarPD),
		pdSign:     extract(reKodikVarPDSign),
		ref:        extract(reKodikVarRef),
		refSign:    extract(reKodikVarRefSign),
		vType:      extract(reKodikInfoType),
		vHash:      extract(reKodikInfoHash),
		vID:        extract(reKodikInfoID),
		scriptPath: extract(reKodikScriptSrc),
	}
	if p.vType == "" || p.vHash == "" || p.vID == "" || p.dSign == "" {
		return nil, errors.New("kodik embed missing video metadata or signatures")
	}
	return p, nil
}

func scriptCacheKey(embedURL *url.URL, scriptPath string) string {
	return strings.ToLower(embedURL.Hostname()) + "|" + scriptPath
}

func getKodikScriptConfig(ctx context.Context, embedURL *url.URL, scriptPath string, forceRefresh bool) kodikScriptConfig {
	key := scriptCacheKey(embedURL, scriptPath)
	if !forceRefresh && scriptPath != "" {
		kodikScriptMu.RLock()
		cached, ok := kodikScriptCache[key]
		kodikScriptMu.RUnlock()
		if ok && time.Since(cached.fetchedAt) < kodikScriptCacheTTL {
			return cached
		}
	}

	cfg := kodikScriptConfig{
		endpoint:  kodikDefaultPath,
		shift:     kodikDefaultShift,
		hasShift:  true,
		fetchedAt: time.Now(),
	}
	if scriptPath != "" {
		if jsBody := fetchKodikScriptBody(ctx, embedURL, scriptPath); jsBody != "" {
			cfg = extractKodikScriptConfig(jsBody)
			updateKodikScriptCache(key, cfg)
		}
	}
	return cfg
}

func updateKodikScriptCache(key string, cfg kodikScriptConfig) {
	if key == "" {
		return
	}
	cfg.fetchedAt = time.Now()
	kodikScriptMu.Lock()
	kodikScriptCache[key] = cfg
	kodikScriptMu.Unlock()
}

func fetchKodikScriptBody(ctx context.Context, embedURL *url.URL, scriptPath string) string {
	resolved, err := embedURL.Parse(scriptPath)
	if err != nil || !isAllowedKodikHost(resolved.Hostname()) {
		return ""
	}
	body, status, err := fetchTextWithHeaders(ctx, resolved.String(), map[string]string{
		"User-Agent": defaultBrowserUA,
		"Referer":    embedURL.String(),
	})
	if err != nil || status < 200 || status >= 300 {
		return ""
	}
	return body
}

func extractKodikScriptConfig(jsBody string) kodikScriptConfig {
	cfg := kodikScriptConfig{
		endpoint:  kodikDefaultPath,
		shift:     kodikDefaultShift,
		hasShift:  true,
		fetchedAt: time.Now(),
	}

	if m := reKodikAjaxAtob.FindStringSubmatch(jsBody); len(m) > 1 {
		if decoded, err := decodeBase64Flexible(m[1]); err == nil && isLikelyKodikEndpoint(decoded) {
			cfg.endpoint = decoded
		}
	} else if m := reKodikAjaxLiteral.FindStringSubmatch(jsBody); len(m) > 1 {
		if isLikelyKodikEndpoint(m[1]) {
			cfg.endpoint = m[1]
		}
	} else {
		for _, ep := range extractKodikCandidateEndpoints(jsBody) {
			cfg.endpoint = ep
			break
		}
	}

	if m := reKodikShiftExpr.FindStringSubmatch(jsBody); len(m) > 2 {
		if n, err := strconv.Atoi(m[2]); err == nil {
			shift := ((n % 26) + 26) % 26
			if m[1] == "-" {
				shift = (26 - shift) % 26
			}
			cfg.shift = shift
			cfg.hasShift = true
		}
	}

	return cfg
}

func extractKodikCandidateEndpoints(jsBody string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, 4)
	add := func(p string) {
		if !isLikelyKodikEndpoint(p) {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	for _, m := range reKodikAnyAtob.FindAllStringSubmatch(jsBody, -1) {
		if decoded, err := decodeBase64Flexible(m[1]); err == nil {
			add(decoded)
		}
	}
	for _, m := range reKodikAjaxLiteral.FindAllStringSubmatch(jsBody, -1) {
		add(m[1])
	}
	return out
}

func isLikelyKodikEndpoint(p string) bool {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || len(p) < 3 || len(p) > 24 {
		return false
	}
	switch strings.ToLower(p) {
	case "/stats", "/assets", "/video", "/seria", "/serial", "/season":
		return false
	}
	for i := 1; i < len(p); i++ {
		c := p[i]
		isValid := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '/'
		if !isValid {
			return false
		}
	}
	return true
}

func requestKodikLinks(ctx context.Context, embedURL *url.URL, embedPageURL string, p *kodikEmbedParams, cfg kodikScriptConfig) (*KodikResolved, error) {
	endpoint := cfg.endpoint
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = kodikDefaultPath
	}
	apiURL := fmt.Sprintf("https://%s%s", embedURL.Host, endpoint)

	form := url.Values{}
	form.Set("d", p.domain)
	form.Set("d_sign", p.dSign)
	form.Set("pd", p.pd)
	form.Set("pd_sign", p.pdSign)
	form.Set("ref", p.ref)
	form.Set("ref_sign", p.refSign)
	form.Set("bad_user", "false")
	form.Set("cdn_is_working", "true")
	form.Set("type", p.vType)
	form.Set("hash", p.vHash)
	form.Set("id", p.vID)
	form.Set("info", "{}")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("User-Agent", defaultBrowserUA)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", "https://"+embedURL.Host)
	req.Header.Set("Referer", embedPageURL)

	resp, err := providerHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, maxExtractorBodySize))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kodik links API %s returned HTTP %d", endpoint, resp.StatusCode)
	}

	var payload struct {
		Links map[string][]struct {
			Src  string `json:"src"`
			Type string `json:"type"`
		} `json:"links"`
	}
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return nil, fmt.Errorf("kodik links JSON parse failed: %w", err)
	}
	if len(payload.Links) == 0 {
		return nil, errors.New("kodik links payload contains no qualities")
	}

	sources := make(map[string]string, len(payload.Links))
	activeShift := cfg.shift
	for q, entries := range payload.Links {
		for _, entry := range entries {
			if strings.TrimSpace(entry.Src) == "" {
				continue
			}
			decoded, usedShift, ok := decodeKodikLink(entry.Src, activeShift)
			if ok {
				if usedShift >= 0 && usedShift != activeShift {
					activeShift = usedShift
					if p.scriptPath != "" {
						updateKodikScriptCache(scriptCacheKey(embedURL, p.scriptPath), kodikScriptConfig{
							endpoint: endpoint,
							shift:    usedShift,
							hasShift: true,
						})
					}
				}
				sources[strings.TrimSpace(q)] = decoded
				break
			}
		}
	}
	if len(sources) == 0 {
		return nil, errors.New("failed to decode kodik stream links")
	}

	return &KodikResolved{
		Sources:  sources,
		Endpoint: endpoint,
		Shift:    activeShift,
	}, nil
}

func decodeKodikLink(rawSrc string, preferredShift int) (string, int, bool) {
	trimmed := strings.TrimSpace(rawSrc)
	if trimmed == "" {
		return "", -1, false
	}
	if strings.Contains(trimmed, "//") {
		if norm := normalizeEmbedURL(trimmed); isValidKodikStreamURL(norm) {
			return norm, -1, true
		}
	}

	tryShift := func(shift int) (string, bool) {
		shifted := caesarShiftASCII(trimmed, shift)
		decoded, err := decodeBase64Flexible(shifted)
		if err != nil {
			return "", false
		}
		norm := normalizeEmbedURL(decoded)
		if !isValidKodikStreamURL(norm) {
			return "", false
		}
		return norm, true
	}

	if preferredShift >= 0 {
		if norm, ok := tryShift(preferredShift); ok {
			return norm, preferredShift, true
		}
	}

	// Self-healing fallback: probe all 26 Caesar shifts (+0..+25).
	for shift := 0; shift < 26; shift++ {
		if shift == preferredShift {
			continue
		}
		if norm, ok := tryShift(shift); ok {
			return norm, shift, true
		}
	}
	return "", -1, false
}

func caesarShiftASCII(input string, shift int) string {
	s := ((shift % 26) + 26) % 26
	if s == 0 {
		return input
	}
	var b strings.Builder
	b.Grow(len(input))
	for i := 0; i < len(input); i++ {
		c := input[i]
		switch {
		case c >= 'A' && c <= 'Z':
			b.WriteByte('A' + byte((int(c-'A')+s)%26))
		case c >= 'a' && c <= 'z':
			b.WriteByte('a' + byte((int(c-'a')+s)%26))
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func decodeBase64Flexible(raw string) (string, error) {
	cleaned := strings.TrimSpace(raw)
	if rem := len(cleaned) % 4; rem != 0 {
		cleaned += strings.Repeat("=", 4-rem)
	}
	if decoded, err := base64.StdEncoding.DecodeString(cleaned); err == nil {
		return string(decoded), nil
	}
	decoded, err := base64.URLEncoding.DecodeString(cleaned)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func isValidKodikStreamURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	if !strings.Contains(u.Host, ".") || len(u.Path) < 2 {
		return false
	}
	lower := strings.ToLower(raw)
	return strings.Contains(lower, ".m3u8") || strings.Contains(lower, "manifest") || strings.Contains(lower, ".mp4")
}

func buildKodikMasterPlaylist(sources map[string]string, referer string) string {
	type variant struct {
		qNum       int
		label      string
		bandwidth  int
		resolution string
		proxyURL   string
	}
	specs := map[int]struct {
		bw  int
		res string
	}{
		1080: {bw: 5500000, res: "1920x1080"},
		720:  {bw: 2800000, res: "1280x720"},
		480:  {bw: 1400000, res: "854x480"},
		360:  {bw: 800000, res: "640x360"},
		240:  {bw: 450000, res: "426x240"},
	}

	variants := make([]variant, 0, len(sources))
	for qStr, streamURL := range sources {
		qNum, err := strconv.Atoi(strings.TrimSpace(qStr))
		if err != nil {
			qNum = 480
		}
		spec, ok := specs[qNum]
		if !ok {
			spec.bw = qNum * 3000
			spec.res = fmt.Sprintf("%dx%d", (qNum*16)/9, qNum)
		}
		variants = append(variants, variant{
			qNum:       qNum,
			label:      fmt.Sprintf("%dp", qNum),
			bandwidth:  spec.bw,
			resolution: spec.res,
			proxyURL:   buildStreamProxyURL(streamURL, referer),
		})
	}
	sort.Slice(variants, func(i, j int) bool {
		return variants[i].qNum > variants[j].qNum
	})

	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n")
	for _, v := range variants {
		b.WriteString(fmt.Sprintf(`#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%s,NAME="%s"`, v.bandwidth, v.resolution, v.label))
		b.WriteByte('\n')
		b.WriteString(v.proxyURL)
		b.WriteByte('\n')
	}
	return b.String()
}

func cloneKodikResolved(in KodikResolved) KodikResolved {
	out := KodikResolved{
		Sources:  make(map[string]string, len(in.Sources)),
		Endpoint: in.Endpoint,
		Shift:    in.Shift,
	}
	for k, v := range in.Sources {
		out.Sources[k] = v
	}
	return out
}
