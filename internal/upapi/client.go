package upapi

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const defaultBaseURL = "https://api.up.com.au/api/v1"

// Client is a minimal Up API client.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// New creates a client using the default Up API base URL.
func New(token string) *Client {
	return &Client{
		baseURL: defaultBaseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// NewWithBaseURL creates a client with a custom base URL.
// Intended for tests and local stubs.
func NewWithBaseURL(token, baseURL string) *Client {
	c := New(token)
	c.baseURL = baseURL
	return c
}

// Ping calls GET /util/ping and returns nil only when status is 200.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/util/ping", nil)
	if err != nil {
		return fmt.Errorf("build ping request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call ping endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping failed with status %d", resp.StatusCode)
	}

	return nil
}
