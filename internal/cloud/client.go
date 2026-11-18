package cloud

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseURL    = "https://50.17.87.91"
	defaultCertSHA256 = "D764DF78A53B2D6E927C8CD2DF243AD81D0144E0307FDB14D6AF7118D798B388"
)

type Config struct {
	BaseURL    string
	CertSHA256 string
	Timeout    time.Duration
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("cloud HTTP %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("cloud HTTP %d", e.Status)
}

type PingResult struct {
	OK        bool   `json:"ok"`
	Service   string `json:"service"`
	Version   string `json:"version"`
	UptimeSec int64  `json:"uptime_sec"`
	Time      string `json:"time"`
}

// Client is the desktop controller for communicating with the mila-cloud server over pinned TLS.
type Client struct {
	baseURL    string
	serverHost string
	tlsCfg     *tls.Config
	httpClient *http.Client

	tokenMu sync.RWMutex
	token   string

	wtMu      sync.Mutex
	wtSession *WTSession
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeFingerprint(raw string) ([]byte, error) {
	cleaned := strings.ReplaceAll(strings.TrimSpace(raw), ":", "")
	if cleaned == "" {
		return nil, nil
	}
	decoded, err := hex.DecodeString(cleaned)
	if err != nil || len(decoded) != sha256.Size {
		return nil, errors.New("invalid sha256 cert fingerprint")
	}
	return decoded, nil
}

func NewDefaultClient() (*Client, error) {
	return NewClient(Config{
		BaseURL:    firstNonEmpty(os.Getenv("MILA_CLOUD_URL"), defaultBaseURL),
		CertSHA256: firstNonEmpty(os.Getenv("MILA_CLOUD_CERT_SHA256"), defaultCertSHA256),
	})
}

func NewClient(cfg Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, errors.New("cloud base URL is required")
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid cloud base URL: %q", baseURL)
	}

	pinnedBytes, err := normalizeFingerprint(cfg.CertSHA256)
	if err != nil {
		return nil, err
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if len(pinnedBytes) == sha256.Size {
		serverHost := parsedURL.Hostname()
		// Pinned self-signed IP cert: skip OS CA verification and enforce exact leaf SHA-256 + SAN + validity.
		tlsCfg.InsecureSkipVerify = true
		tlsCfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("server presented no TLS certificates")
			}
			sum := sha256.Sum256(rawCerts[0])
			if subtle.ConstantTimeCompare(sum[:], pinnedBytes) != 1 {
				return errors.New("TLS certificate fingerprint mismatch")
			}
			cert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return err
			}
			now := time.Now()
			if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
				return errors.New("pinned TLS certificate is expired or not yet valid")
			}
			if err := cert.VerifyHostname(serverHost); err != nil {
				return fmt.Errorf("pinned TLS certificate host mismatch: %w", err)
			}
			return nil
		}
	}

	return &Client{
		baseURL:    baseURL,
		serverHost: parsedURL.Hostname(),
		tlsCfg:     tlsCfg,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig:       tlsCfg,
				MaxIdleConns:          10,
				IdleConnTimeout:       60 * time.Second,
				TLSHandshakeTimeout:   6 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
	}, nil
}

func (c *Client) BaseURL() string {
	if c == nil {
		return ""
	}
	return c.baseURL
}

// SetToken stores an optional bearer token for future auth endpoints.
func (c *Client) SetToken(token string) {
	if c == nil {
		return
	}
	c.tokenMu.Lock()
	c.token = strings.TrimSpace(token)
	c.tokenMu.Unlock()
}

func (c *Client) getToken() string {
	if c == nil {
		return ""
	}
	c.tokenMu.RLock()
	tok := c.token
	c.tokenMu.RUnlock()
	return tok
}

func (c *Client) DoJSON(ctx context.Context, method, path string, reqBody any, out any) error {
	var bodyBytes []byte
	if reqBody != nil {
		encoded, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		bodyBytes = encoded
	}
	raw, err := c.doBytes(ctx, method, path, bodyBytes)
	if err != nil {
		return err
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// DoRawJSON sends a raw JSON request body (when non-empty) and returns raw response bytes.
func (c *Client) DoRawJSON(ctx context.Context, method, path, bodyJSON string) ([]byte, error) {
	trimmed := strings.TrimSpace(bodyJSON)
	var bodyBytes []byte
	if trimmed != "" {
		bodyBytes = []byte(trimmed)
	}
	return c.doBytes(ctx, method, path, bodyBytes)
}

func (c *Client) doBytes(ctx context.Context, method, path string, bodyBytes []byte) ([]byte, error) {
	if c == nil {
		return nil, errors.New("cloud client is nil")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	var bodyReader io.Reader
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if len(bodyBytes) > 0 {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	if tok := c.getToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{
			Status:  resp.StatusCode,
			Message: strings.TrimSpace(string(raw)),
		}
	}
	return raw, nil
}

func (c *Client) GetJSON(ctx context.Context, path string, out any) error {
	return c.DoJSON(ctx, http.MethodGet, path, nil, out)
}

func (c *Client) PostJSON(ctx context.Context, path string, reqBody any, out any) error {
	return c.DoJSON(ctx, http.MethodPost, path, reqBody, out)
}

func (c *Client) DeleteJSON(ctx context.Context, path string, out any) error {
	return c.DoJSON(ctx, http.MethodDelete, path, nil, out)
}

func (c *Client) Ping(ctx context.Context) (*PingResult, error) {
	var res PingResult
	if err := c.GetJSON(ctx, "/api/v1/ping", &res); err != nil {
		return nil, err
	}
	return &res, nil
}
