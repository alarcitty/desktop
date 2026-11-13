package aniliberty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mila/internal/cache"
)

const (
	defaultBaseURL    = "https://aniliberty.top/api/v1"
	defaultMirrorURL  = "https://anilibria.top/api/v1"
	defaultUserAgent  = "mila-desktop/1.0"
	defaultListTTL    = 2 * time.Minute
	defaultReleaseTTL = 3 * time.Minute
	defaultRefTTL     = 6 * time.Hour
	maxRetryAttempts  = 2
)

// ErrNotFound is returned when AniLiberty reports 404 for a release.
var ErrNotFound = errors.New("aniliberty: release not found")

// Config configures the AniLiberty API v1 client.
type Config struct {
	BaseURL    string
	MirrorURL  string
	UserAgent  string
	HTTPClient *http.Client
	ListTTL    time.Duration
	ReleaseTTL time.Duration
	RefTTL     time.Duration
}

// Client interacts with AniLiberty API v1 with caching, mirror fallback, and URL normalization.
type Client struct {
	baseURL    string
	mirrorURL  string
	mediaRoot  string
	userAgent  string
	http       *http.Client
	listTTL    time.Duration
	releaseTTL time.Duration
	refTTL     time.Duration

	releasesCache   *cache.Cache[string, Release]
	listCache       *cache.Cache[string, []Release]
	catalogCache    *cache.Cache[string, CatalogResponse]
	schedNowCache   *cache.Cache[string, ScheduleNow]
	schedWeekCache  *cache.Cache[string, []ScheduleItem]
	referencesCache *cache.Cache[string, References]
}

// New creates an AniLiberty client with sensible defaults and mirror fallback.
func New(cfg Config) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	mirrorURL := strings.TrimRight(cfg.MirrorURL, "/")
	if mirrorURL == "" && cfg.BaseURL == "" {
		mirrorURL = defaultMirrorURL
	}
	userAgent := strings.TrimSpace(cfg.UserAgent)
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 12 * time.Second}
	}
	listTTL := cfg.ListTTL
	if listTTL <= 0 {
		listTTL = defaultListTTL
	}
	releaseTTL := cfg.ReleaseTTL
	if releaseTTL <= 0 {
		releaseTTL = defaultReleaseTTL
	}
	refTTL := cfg.RefTTL
	if refTTL <= 0 {
		refTTL = defaultRefTTL
	}

	return &Client{
		baseURL:         baseURL,
		mirrorURL:       mirrorURL,
		mediaRoot:       originFromBase(baseURL),
		userAgent:       userAgent,
		http:            httpClient,
		listTTL:         listTTL,
		releaseTTL:      releaseTTL,
		refTTL:          refTTL,
		releasesCache:   cache.New[string, Release](),
		listCache:       cache.New[string, []Release](),
		catalogCache:    cache.New[string, CatalogResponse](),
		schedNowCache:   cache.New[string, ScheduleNow](),
		schedWeekCache:  cache.New[string, []ScheduleItem](),
		referencesCache: cache.New[string, References](),
	}
}

// Latest fetches the latest updated anime releases.
func (c *Client) Latest(ctx context.Context, limit int) ([]Release, error) {
	if limit <= 0 {
		limit = 14
	}
	if limit > 50 {
		limit = 50
	}
	key := fmt.Sprintf("latest:%d", limit)
	return c.listCache.Do(ctx, key, c.listTTL, func(ctx context.Context) ([]Release, error) {
		var items []Release
		path := fmt.Sprintf("/anime/releases/latest?limit=%d", limit)
		if err := c.getJSON(ctx, path, &items); err != nil {
			return nil, err
		}
		for i := range items {
			c.normalizeRelease(&items[i])
		}
		return items, nil
	})
}

// Catalog fetches paginated releases with optional search, sorting, genres, types, seasons, and years.
func (c *Client) Catalog(ctx context.Context, params CatalogParams) (CatalogResponse, error) {
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.Limit <= 0 {
		params.Limit = 20
	}
	if params.Limit > 50 {
		params.Limit = 50
	}

	q := url.Values{}
	q.Set("page", strconv.Itoa(params.Page))
	q.Set("limit", strconv.Itoa(params.Limit))
	if s := strings.TrimSpace(params.Search); s != "" {
		q.Set("f[search]", s)
	}
	if s := strings.TrimSpace(params.Sorting); s != "" {
		q.Set("f[sorting]", s)
	}
	if len(params.Genres) > 0 {
		parts := make([]string, len(params.Genres))
		for i, g := range params.Genres {
			parts[i] = strconv.Itoa(g)
		}
		q.Set("f[genres]", strings.Join(parts, ","))
	}
	if len(params.Types) > 0 {
		q.Set("f[types]", strings.Join(params.Types, ","))
	}
	if len(params.Seasons) > 0 {
		q.Set("f[seasons]", strings.Join(params.Seasons, ","))
	}
	if params.FromYear > 0 {
		q.Set("f[years][from_year]", strconv.Itoa(params.FromYear))
	}
	if params.ToYear > 0 {
		q.Set("f[years][to_year]", strconv.Itoa(params.ToYear))
	}

	encoded := q.Encode()
	return c.catalogCache.Do(ctx, encoded, c.listTTL, func(ctx context.Context) (CatalogResponse, error) {
		var raw struct {
			Data []Release `json:"data"`
			Meta struct {
				Pagination Pagination `json:"pagination"`
			} `json:"meta"`
		}
		if err := c.getJSON(ctx, "/anime/catalog/releases?"+encoded, &raw); err != nil {
			return CatalogResponse{}, err
		}
		for i := range raw.Data {
			c.normalizeRelease(&raw.Data[i])
		}
		return CatalogResponse{
			Data:       raw.Data,
			Pagination: raw.Meta.Pagination,
		}, nil
	})
}

// Search performs quick release search via /app/search/releases.
func (c *Client) Search(ctx context.Context, query string) ([]Release, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return []Release{}, nil
	}
	key := "search:" + strings.ToLower(trimmed)
	return c.listCache.Do(ctx, key, c.listTTL, func(ctx context.Context) ([]Release, error) {
		var items []Release
		path := "/app/search/releases?query=" + url.QueryEscape(trimmed)
		if err := c.getJSON(ctx, path, &items); err != nil {
			return nil, err
		}
		for i := range items {
			c.normalizeRelease(&items[i])
		}
		return items, nil
	})
}

// Release fetches full details (including episodes, HLS streams, and torrents) by numeric ID or alias.
func (c *Client) Release(ctx context.Context, idOrAlias string) (Release, error) {
	idOrAlias = strings.TrimSpace(idOrAlias)
	if idOrAlias == "" {
		return Release{}, fmt.Errorf("aniliberty: empty release identifier")
	}
	return c.releasesCache.Do(ctx, idOrAlias, c.releaseTTL, func(ctx context.Context) (Release, error) {
		var rel Release
		path := "/anime/releases/" + url.PathEscape(idOrAlias)
		if err := c.getJSON(ctx, path, &rel); err != nil {
			return Release{}, err
		}
		c.normalizeRelease(&rel)
		return rel, nil
	})
}

// ScheduleNow fetches yesterday, today, and tomorrow broadcast schedules.
func (c *Client) ScheduleNow(ctx context.Context) (ScheduleNow, error) {
	return c.schedNowCache.Do(ctx, "now", c.listTTL, func(ctx context.Context) (ScheduleNow, error) {
		var sched ScheduleNow
		if err := c.getJSON(ctx, "/anime/schedule/now", &sched); err != nil {
			return ScheduleNow{}, err
		}
		c.normalizeScheduleItems(sched.Today)
		c.normalizeScheduleItems(sched.Tomorrow)
		c.normalizeScheduleItems(sched.Yesterday)
		return sched, nil
	})
}

// ScheduleWeek fetches the full weekly release schedule.
func (c *Client) ScheduleWeek(ctx context.Context) ([]ScheduleItem, error) {
	return c.schedWeekCache.Do(ctx, "week", c.listTTL, func(ctx context.Context) ([]ScheduleItem, error) {
		var items []ScheduleItem
		if err := c.getJSON(ctx, "/anime/schedule/week", &items); err != nil {
			return nil, err
		}
		c.normalizeScheduleItems(items)
		return items, nil
	})
}

// Genres fetches all catalog genres.
func (c *Client) Genres(ctx context.Context) ([]Genre, error) {
	refs, err := c.References(ctx)
	if err != nil {
		return nil, err
	}
	return refs.Genres, nil
}

// References fetches genres, sorting options, release types, and seasons.
func (c *Client) References(ctx context.Context) (References, error) {
	return c.referencesCache.Do(ctx, "all", c.refTTL, func(ctx context.Context) (References, error) {
		var refs References
		if err := c.getJSON(ctx, "/anime/catalog/references/genres", &refs.Genres); err != nil {
			return References{}, err
		}
		for i := range refs.Genres {
			c.normalizeImage(refs.Genres[i].Image)
		}
		if err := c.getJSON(ctx, "/anime/catalog/references/sorting", &refs.Sorting); err != nil {
			return References{}, err
		}
		if err := c.getJSON(ctx, "/anime/catalog/references/types", &refs.Types); err != nil {
			return References{}, err
		}
		if err := c.getJSON(ctx, "/anime/catalog/references/seasons", &refs.Seasons); err != nil {
			return References{}, err
		}
		return refs, nil
	})
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	bases := []string{c.baseURL}
	if c.mirrorURL != "" && c.mirrorURL != c.baseURL {
		bases = append(bases, c.mirrorURL)
	}

	var lastErr error
	for _, base := range bases {
		endpoint := base + path
		for attempt := 0; attempt < maxRetryAttempts; attempt++ {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			if err != nil {
				return fmt.Errorf("aniliberty: create request: %w", err)
			}
			req.Header.Set("Accept", "application/json")
			req.Header.Set("User-Agent", c.userAgent)

			resp, err := c.http.Do(req)
			if err != nil {
				lastErr = fmt.Errorf("aniliberty: request %s failed: %w", path, err)
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return err
				}
				continue
			}

			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
			resp.Body.Close()

			if resp.StatusCode == http.StatusNotFound {
				return ErrNotFound
			}
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				lastErr = fmt.Errorf("aniliberty: %s status %d", path, resp.StatusCode)
				timer := time.NewTimer(time.Duration(250*(attempt+1)) * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
				continue
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("aniliberty: %s status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
			}
			if readErr != nil {
				return fmt.Errorf("aniliberty: read body: %w", readErr)
			}
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("aniliberty: decode %s: %w", path, err)
			}
			return nil
		}
	}

	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("aniliberty: %s failed", path)
}

func (c *Client) normalizeScheduleItems(items []ScheduleItem) {
	for i := range items {
		c.normalizeRelease(&items[i].Release)
		if items[i].PublishedEpisode != nil {
			c.normalizeImage(items[i].PublishedEpisode.Preview)
		}
	}
}

func (c *Client) normalizeRelease(r *Release) {
	c.normalizeImage(r.Poster)
	for i := range r.Genres {
		c.normalizeImage(r.Genres[i].Image)
	}
	for i := range r.BackgroundCovers {
		c.normalizeImage(&r.BackgroundCovers[i])
	}
	if r.LatestEpisode != nil {
		c.normalizeImage(r.LatestEpisode.Preview)
	}
	for i := range r.Episodes {
		c.normalizeImage(r.Episodes[i].Preview)
	}
}

func (c *Client) normalizeImage(img *MediaImage) {
	if img == nil {
		return
	}
	img.Src = c.resolveMediaURL(img.Src)
	img.Preview = c.resolveMediaURL(img.Preview)
	img.Thumbnail = c.resolveMediaURL(img.Thumbnail)
	if img.Optimized != nil {
		img.Optimized.Src = c.resolveMediaURL(img.Optimized.Src)
		img.Optimized.Preview = c.resolveMediaURL(img.Optimized.Preview)
		img.Optimized.Thumbnail = c.resolveMediaURL(img.Optimized.Thumbnail)
	}
}

func (c *Client) resolveMediaURL(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if strings.HasPrefix(raw, "/") && c.mediaRoot != "" {
		return c.mediaRoot + raw
	}
	return raw
}

func originFromBase(rawBase string) string {
	u, err := url.Parse(rawBase)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
