package aniskip

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"mila/internal/cache"
	"mila/internal/httpx"
)

const (
	defaultBaseURL     = "https://api.aniskip.com/v2"
	defaultUserAgent   = "mila-desktop/1.0"
	defaultCacheTTL    = 7 * 24 * time.Hour
	defaultNegativeTTL = cache.TTLMetadata
)

// SkipRange mirrors the player's skip interval `{ start, stop }` in seconds.
type SkipRange struct {
	Start *float64 `json:"start,omitempty"`
	Stop  *float64 `json:"stop,omitempty"`
}

// SkipTimes represents normalized opening and ending skip ranges for one episode.
type SkipTimes struct {
	MalID   int        `json:"mal_id"`
	Episode int        `json:"episode"`
	Found   bool       `json:"found"`
	Opening *SkipRange `json:"opening,omitempty"`
	Ending  *SkipRange `json:"ending,omitempty"`
}

// Config configures the AniSkip API v2 client.
type Config struct {
	BaseURL     string
	UserAgent   string
	HTTPClient  *http.Client
	CacheTTL    time.Duration
	NegativeTTL time.Duration
	CacheDir    string
}

// Client fetches OP/ED skip intervals from AniSkip API v2 with L1/L2 caching and singleflight deduplication.
type Client struct {
	baseURL     string
	userAgent   string
	http        *http.Client
	limiter     *httpx.DomainLimiter
	flights     httpx.FlightGroup[SkipTimes]
	httpFlights httpx.FlightGroup[httpx.ResponseResult]
	cacheTTL    time.Duration
	negativeTTL time.Duration
	cache       *cache.Cache[string, SkipTimes]
}

// New constructs an AniSkip client with rate limiting, retries, and optional disk cache persistence.
func New(cfg Config) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		if env := strings.TrimRight(strings.TrimSpace(os.Getenv("ANISKIP_BASE_URL")), "/"); env != "" {
			baseURL = env
		} else {
			baseURL = defaultBaseURL
		}
	}
	userAgent := strings.TrimSpace(cfg.UserAgent)
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = httpx.NewClient(8 * time.Second)
	}
	cacheTTL := cfg.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}
	negativeTTL := cfg.NegativeTTL
	if negativeTTL <= 0 {
		negativeTTL = defaultNegativeTTL
	}

	opts := []cache.Option[string, SkipTimes]{
		cache.WithMaxEntries[string, SkipTimes](2048),
		cache.WithMaxBytes[string, SkipTimes](8 << 20),
	}
	if dir := strings.TrimSpace(cfg.CacheDir); dir != "" {
		opts = append(opts, cache.WithDiskDir[string, SkipTimes](dir))
	}

	return &Client{
		baseURL:     baseURL,
		userAgent:   userAgent,
		http:        httpClient,
		limiter:     httpx.NewDomainLimiter(httpx.HostRule{RatePerSec: 6, Burst: 4}, nil),
		cacheTTL:    cacheTTL,
		negativeTTL: negativeTTL,
		cache:       cache.New[string, SkipTimes](opts...),
	}
}

type apiResponse struct {
	Found      bool        `json:"found"`
	StatusCode int         `json:"statusCode"`
	Results    []apiResult `json:"results"`
}

type apiResult struct {
	SkipType string `json:"skipType"`
	Interval struct {
		StartTime float64 `json:"startTime"`
		EndTime   float64 `json:"endTime"`
	} `json:"interval"`
}

// GetSkipTimes returns normalized OP/ED timings for a MAL anime ID and 1-based episode number.
func (c *Client) GetSkipTimes(ctx context.Context, malID, episode int) (SkipTimes, error) {
	if malID <= 0 || episode <= 0 {
		return SkipTimes{MalID: malID, Episode: episode, Found: false}, nil
	}

	key := fmt.Sprintf("%d:%d", malID, episode)
	if cached, ok := c.cache.Get(key); ok {
		return cached, nil
	}

	return c.flights.Do(ctx, key, func(flightCtx context.Context) (SkipTimes, error) {
		if cached, ok := c.cache.Get(key); ok {
			return cached, nil
		}

		endpoint := fmt.Sprintf(
			"%s/skip-times/%d/%d?types=op&types=mixed-op&types=ed&types=mixed-ed&episodeLength=0",
			c.baseURL, malID, episode,
		)
		res, err := httpx.GetBytes(flightCtx, c.http, c.limiter, &c.httpFlights, endpoint, map[string]string{
			"Accept":     "application/json",
			"User-Agent": c.userAgent,
		}, httpx.RetryConfig{
			MaxAttempts: 2,
			BaseDelay:   250 * time.Millisecond,
			MaxDelay:    3 * time.Second,
			MaxBody:     1 << 20,
		})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return SkipTimes{}, err
			}
			return SkipTimes{}, fmt.Errorf("aniskip: request %d/%d failed: %w", malID, episode, err)
		}

		if res.StatusCode == http.StatusNotFound {
			negative := SkipTimes{MalID: malID, Episode: episode, Found: false}
			c.cache.Set(key, negative, c.negativeTTL)
			return negative, nil
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return SkipTimes{}, fmt.Errorf("aniskip: %d/%d returned HTTP %d", malID, episode, res.StatusCode)
		}

		var parsed apiResponse
		if err := json.Unmarshal(res.Body, &parsed); err != nil {
			return SkipTimes{}, fmt.Errorf("aniskip: decode %d/%d failed: %w", malID, episode, err)
		}

		out := parseSkipResults(malID, episode, parsed.Results)
		ttl := c.negativeTTL
		if out.Found {
			ttl = c.cacheTTL
		}
		c.cache.Set(key, out, ttl)
		return out, nil
	})
}

func parseSkipResults(malID, episode int, results []apiResult) SkipTimes {
	out := SkipTimes{
		MalID:   malID,
		Episode: episode,
	}
	var opExact, opMixed *SkipRange
	var edExact, edMixed *SkipRange

	for _, item := range results {
		start := item.Interval.StartTime
		stop := item.Interval.EndTime
		if math.IsNaN(start) || math.IsNaN(stop) || math.IsInf(start, 0) || math.IsInf(stop, 0) {
			continue
		}
		if start < 0 {
			start = 0
		}
		if stop <= start+1 {
			continue
		}
		sCopy := start
		eCopy := stop
		rng := &SkipRange{Start: &sCopy, Stop: &eCopy}

		switch strings.ToLower(strings.TrimSpace(item.SkipType)) {
		case "op":
			if opExact == nil {
				opExact = rng
			}
		case "mixed-op":
			if opMixed == nil {
				opMixed = rng
			}
		case "ed":
			if edExact == nil {
				edExact = rng
			}
		case "mixed-ed":
			if edMixed == nil {
				edMixed = rng
			}
		}
	}

	if opExact != nil {
		out.Opening = opExact
	} else {
		out.Opening = opMixed
	}
	if edExact != nil {
		out.Ending = edExact
	} else {
		out.Ending = edMixed
	}
	out.Found = out.Opening != nil || out.Ending != nil
	return out
}
