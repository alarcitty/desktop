package mila

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const animeVostBaseURL = "https://api.animevost.org/v1"

var providerHTTPClient = &http.Client{
	Timeout: 7 * time.Second,
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
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
