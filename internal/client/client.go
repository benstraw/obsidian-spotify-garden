package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ErrNotFound is returned when the API responds with 404.
var ErrNotFound = errors.New("not found")

const defaultBaseURL = "https://api.spotify.com/v1"

// Client is an authenticated Spotify API client.
type Client struct {
	accessToken string
	baseURL     string
	httpClient  *http.Client
}

// NewClient creates a new Client with the given access token.
func NewClient(token string) *Client {
	return &Client{
		accessToken: token,
		baseURL:     defaultBaseURL,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Get performs a GET request to the Spotify API.
// It retries on HTTP 429 with exponential backoff (1s, 2s, 4s).
func (c *Client) Get(path string, params url.Values) ([]byte, error) {
	backoff := time.Second
	for attempt := 0; attempt <= 3; attempt++ {
		body, statusCode, err := c.doGet(path, params)
		if err != nil {
			return nil, err
		}
		if statusCode == http.StatusTooManyRequests {
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		if statusCode == http.StatusNotFound {
			return nil, ErrNotFound
		}
		if statusCode < 200 || statusCode >= 300 {
			return nil, fmt.Errorf("Spotify API returned %d for %s", statusCode, path)
		}
		return body, nil
	}
	return nil, fmt.Errorf("Spotify API rate limit exceeded for %s after retries", path)
}

// doGet executes a single GET request and returns body, status code, and error.
func (c *Client) doGet(path string, params url.Values) ([]byte, int, error) {
	reqURL := c.baseURL + path
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, resp.StatusCode, nil
}
