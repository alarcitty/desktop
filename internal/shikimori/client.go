package shikimori

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"mila/internal/cache"
)

const (
	defaultBaseURL     = "https://shikimori.io"
	defaultUserAgent   = "mila-desktop/1.0"
	defaultCacheTTL    = cache.TTLMetadata
	defaultSearchTTL   = cache.TTLSearch
	defaultMinInterval = 250 * time.Millisecond // ~4 req/s, safely under Shikimori's 5 RPS cap
	maxBatchIDs        = 50
	maxRetryAttempts   = 3
)

var (
	bbTagPattern = regexp.MustCompile(`\[/?[a-zA-Z0-9_*]+(?:=[^\]]*)?\]`)
	multiSpace   = regexp.MustCompile(`[ \t]{2,}`)
	multiNewline = regexp.MustCompile(`\n{3,}`)
)

// Config configures the Shikimori API client.
type Config struct {
	BaseURL     string
	UserAgent   string
	Token       string
	HTTPClient  *http.Client
	MinInterval time.Duration
	CacheTTL    time.Duration
	SearchTTL   time.Duration
	CacheDir    string
}

// Client communicates with Shikimori GraphQL API with rate limiting, retries, and caching.
type Client struct {
	baseURL     string
	userAgent   string
	token       string
	http        *http.Client
	minInterval time.Duration
	cacheTTL    time.Duration
	searchTTL   time.Duration

	rateMu  sync.Mutex
	nextAt  time.Time
	byID    *cache.Cache[int, Anime]
	byQuery *cache.Cache[string, []Anime]
}

// New creates a new Shikimori client with sensible production defaults.
func New(cfg Config) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	userAgent := strings.TrimSpace(cfg.UserAgent)
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 12 * time.Second}
	}
	minInterval := cfg.MinInterval
	if minInterval <= 0 {
		minInterval = defaultMinInterval
	}
	cacheTTL := cfg.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}
	searchTTL := cfg.SearchTTL
	if searchTTL <= 0 {
		searchTTL = defaultSearchTTL
	}
	byIDOpts := []cache.Option[int, Anime]{
		cache.WithMaxEntries[int, Anime](4096),
		cache.WithMaxBytes[int, Anime](32 << 20),
	}
	byQueryOpts := []cache.Option[string, []Anime]{
		cache.WithMaxEntries[string, []Anime](512),
		cache.WithMaxBytes[string, []Anime](16 << 20),
	}
	if dir := strings.TrimSpace(cfg.CacheDir); dir != "" {
		byIDOpts = append(byIDOpts, cache.WithDiskDir[int, Anime](filepath.Join(dir, "by_id")))
		byQueryOpts = append(byQueryOpts, cache.WithDiskDir[string, []Anime](filepath.Join(dir, "by_query")))
	}

	return &Client{
		baseURL:     baseURL,
		userAgent:   userAgent,
		token:       strings.TrimSpace(cfg.Token),
		http:        httpClient,
		minInterval: minInterval,
		cacheTTL:    cacheTTL,
		searchTTL:   searchTTL,
		byID:        cache.New[int, Anime](byIDOpts...),
		byQuery:     cache.New[string, []Anime](byQueryOpts...),
	}
}

const animeFields = `
	id
	malId
	name
	russian
	english
	url
	description
	score
	status
	kind
	rating
	episodes
	episodesAired
	duration
	airedOn { date year }
	nextEpisodeAt
	poster { id originalUrl mainUrl main2xUrl }
	screenshots { id originalUrl x332Url x166Url }
	videos { id name url imageUrl playerUrl kind }
	genres { id name russian }
	studios { id name imageUrl }
`

// GetByID fetches a single anime by its Shikimori ID.
func (c *Client) GetByID(ctx context.Context, id int) (*Anime, error) {
	if id <= 0 {
		return nil, fmt.Errorf("shikimori: invalid anime id %d", id)
	}
	res, err := c.GetByIDs(ctx, []int{id})
	if err != nil {
		return nil, err
	}
	a, ok := res[id]
	if !ok {
		return nil, nil
	}
	return &a, nil
}

// GetByIDs fetches multiple anime records in batched GraphQL requests, serving cached hits first.
func (c *Client) GetByIDs(ctx context.Context, ids []int) (map[int]Anime, error) {
	out := make(map[int]Anime, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	seen := make(map[int]struct{}, len(ids))
	missing := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if cached, ok := c.byID.Get(id); ok {
			out[id] = cached
			continue
		}
		missing = append(missing, id)
	}

	for start := 0; start < len(missing); start += maxBatchIDs {
		end := start + maxBatchIDs
		if end > len(missing) {
			end = len(missing)
		}
		batch := missing[start:end]

		strIDs := make([]string, len(batch))
		for i, id := range batch {
			strIDs[i] = strconv.Itoa(id)
		}

		query := fmt.Sprintf(
			`query($ids: String, $limit: PositiveInt) { animes(ids: $ids, limit: $limit, censored: false) { %s } }`,
			animeFields,
		)
		vars := map[string]any{
			"ids":   strings.Join(strIDs, ","),
			"limit": len(batch),
		}
		items, err := c.queryAnimes(ctx, query, vars)
		if err != nil {
			return out, err
		}
		for _, a := range items {
			c.byID.Set(a.ID, a, c.cacheTTL)
			out[a.ID] = a
		}
	}

	return out, nil
}

// Search queries Shikimori anime by title.
func (c *Client) Search(ctx context.Context, queryText string, limit int) ([]Anime, error) {
	q := strings.TrimSpace(queryText)
	if q == "" {
		return []Anime{}, nil
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > maxBatchIDs {
		limit = maxBatchIDs
	}

	cacheKey := fmt.Sprintf("search:%d:%s", limit, strings.ToLower(q))
	return c.byQuery.Do(ctx, cacheKey, c.searchTTL, func(ctx context.Context) ([]Anime, error) {
		gql := fmt.Sprintf(
			`query($search: String, $limit: PositiveInt) { animes(search: $search, limit: $limit, censored: false) { %s } }`,
			animeFields,
		)
		items, err := c.queryAnimes(ctx, gql, map[string]any{
			"search": q,
			"limit":  limit,
		})
		if err != nil {
			return nil, err
		}
		for _, a := range items {
			c.byID.Set(a.ID, a, c.cacheTTL)
		}
		return items, nil
	})
}

func (c *Client) waitRateLimit(ctx context.Context) error {
	c.rateMu.Lock()
	now := time.Now()
	wait := time.Until(c.nextAt)
	if wait < 0 {
		wait = 0
	}
	c.nextAt = now.Add(wait + c.minInterval)
	c.rateMu.Unlock()

	if wait == 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type gqlRawAnime struct {
	ID            string  `json:"id"`
	MalID         string  `json:"malId"`
	Name          string  `json:"name"`
	Russian       string  `json:"russian"`
	English       string  `json:"english"`
	URL           string  `json:"url"`
	Description   string  `json:"description"`
	Score         float64 `json:"score"`
	Status        string  `json:"status"`
	Kind          string  `json:"kind"`
	Rating        string  `json:"rating"`
	Episodes      int     `json:"episodes"`
	EpisodesAired int     `json:"episodesAired"`
	Duration      int     `json:"duration"`
	AiredOn       *struct {
		Date string `json:"date"`
		Year int    `json:"year"`
	} `json:"airedOn"`
	NextEpisodeAt string `json:"nextEpisodeAt"`
	Poster        *struct {
		ID          string `json:"id"`
		OriginalURL string `json:"originalUrl"`
		MainURL     string `json:"mainUrl"`
		Main2xURL   string `json:"main2xUrl"`
	} `json:"poster"`
	Screenshots []struct {
		ID          string `json:"id"`
		OriginalURL string `json:"originalUrl"`
		X332URL     string `json:"x332Url"`
		X166URL     string `json:"x166Url"`
	} `json:"screenshots"`
	Videos []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		URL       string `json:"url"`
		ImageURL  string `json:"imageUrl"`
		PlayerURL string `json:"playerUrl"`
		Kind      string `json:"kind"`
	} `json:"videos"`
	Genres  []Genre `json:"genres"`
	Studios []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		ImageURL string `json:"imageUrl"`
	} `json:"studios"`
}

type gqlResponse struct {
	Data struct {
		Animes []gqlRawAnime `json:"animes"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *Client) queryAnimes(ctx context.Context, query string, vars map[string]any) ([]Anime, error) {
	payload, err := json.Marshal(gqlRequest{
		Query:     query,
		Variables: vars,
	})
	if err != nil {
		return nil, fmt.Errorf("shikimori: marshal query: %w", err)
	}

	endpoint := c.baseURL + "/api/graphql"
	var lastErr error

	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		if err := c.waitRateLimit(ctx); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("shikimori: create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("shikimori: request failed: %w", err)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			if !sleepBackoff(ctx, retryDelay(attempt, "")) {
				return nil, ctx.Err()
			}
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("shikimori: upstream status %d", resp.StatusCode)
			if !sleepBackoff(ctx, retryDelay(attempt, resp.Header.Get("Retry-After"))) {
				return nil, ctx.Err()
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("shikimori: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		if readErr != nil {
			return nil, fmt.Errorf("shikimori: read body: %w", readErr)
		}

		var parsed gqlResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("shikimori: decode response: %w", err)
		}
		if len(parsed.Errors) > 0 {
			return nil, fmt.Errorf("shikimori: graphql error: %s", parsed.Errors[0].Message)
		}

		out := make([]Anime, 0, len(parsed.Data.Animes))
		for _, raw := range parsed.Data.Animes {
			out = append(out, c.normalizeAnime(raw))
		}
		return out, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("shikimori: request failed after retries")
}

func (c *Client) normalizeAnime(raw gqlRawAnime) Anime {
	id, _ := strconv.Atoi(raw.ID)
	malID, _ := strconv.Atoi(raw.MalID)

	a := Anime{
		ID:            id,
		MalID:         malID,
		Name:          raw.Name,
		Russian:       raw.Russian,
		English:       raw.English,
		URL:           c.resolveURL(raw.URL),
		Description:   CleanDescription(raw.Description),
		Score:         raw.Score,
		Status:        raw.Status,
		Kind:          raw.Kind,
		Rating:        raw.Rating,
		Episodes:      raw.Episodes,
		EpisodesAired: raw.EpisodesAired,
		Duration:      raw.Duration,
		NextEpisodeAt: raw.NextEpisodeAt,
		Genres:        raw.Genres,
	}
	if raw.AiredOn != nil {
		a.AiredOn = raw.AiredOn.Date
		a.Year = raw.AiredOn.Year
	}
	if raw.Poster != nil {
		a.Poster = &Poster{
			ID:          raw.Poster.ID,
			OriginalURL: c.resolveURL(raw.Poster.OriginalURL),
			MainURL:     c.resolveURL(raw.Poster.MainURL),
			Main2xURL:   c.resolveURL(raw.Poster.Main2xURL),
		}
	}
	if len(raw.Screenshots) > 0 {
		a.Screenshots = make([]Screenshot, len(raw.Screenshots))
		for i, s := range raw.Screenshots {
			a.Screenshots[i] = Screenshot{
				ID:          s.ID,
				OriginalURL: c.resolveURL(s.OriginalURL),
				X332URL:     c.resolveURL(s.X332URL),
				X166URL:     c.resolveURL(s.X166URL),
			}
		}
	}
	if len(raw.Videos) > 0 {
		a.Videos = make([]Video, len(raw.Videos))
		for i, v := range raw.Videos {
			a.Videos[i] = Video{
				ID:        v.ID,
				Name:      v.Name,
				URL:       v.URL,
				ImageURL:  c.resolveURL(v.ImageURL),
				PlayerURL: c.resolveURL(v.PlayerURL),
				Kind:      v.Kind,
			}
		}
	}
	if len(raw.Studios) > 0 {
		a.Studios = make([]Studio, len(raw.Studios))
		for i, st := range raw.Studios {
			a.Studios[i] = Studio{
				ID:       st.ID,
				Name:     st.Name,
				ImageURL: c.resolveURL(st.ImageURL),
			}
		}
	}
	return a
}

func (c *Client) resolveURL(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if strings.HasPrefix(raw, "/") {
		return c.baseURL + raw
	}
	return raw
}

// CleanDescription strips Shikimori BBCode tags and normalizes whitespace.
func CleanDescription(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "[br]", "\n")
	s = bbTagPattern.ReplaceAllString(s, "")
	s = multiSpace.ReplaceAllString(s, " ")
	s = multiNewline.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func retryDelay(attempt int, retryAfter string) time.Duration {
	if secs, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && secs > 0 && secs <= 30 {
		return time.Duration(secs) * time.Second
	}
	return time.Duration(400*(1<<attempt)) * time.Millisecond
}

func sleepBackoff(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
