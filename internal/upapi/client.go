package upapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.up.com.au/api/v1"
const defaultPageSize = 15
const accountsPageSize = 50

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

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, out, http.StatusOK)
}

func (c *Client) getURL(ctx context.Context, fullURL string, out any) error {
	return c.doURL(ctx, http.MethodGet, fullURL, out, http.StatusOK)
}

func (c *Client) do(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	out any,
	okStatus ...int,
) error {
	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL = fullURL + "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return fmt.Errorf("build %s request: %w", method, err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	statusOK := false
	for _, status := range okStatus {
		if resp.StatusCode == status {
			statusOK = true
			break
		}
	}
	if !statusOK {
		return fmt.Errorf(
			"%s %s failed with status %d: %s",
			method,
			path,
			resp.StatusCode,
			strings.TrimSpace(string(respBody)),
		)
	}

	if out == nil || len(respBody) == 0 {
		return nil
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}

	return nil
}

func (c *Client) doURL(
	ctx context.Context,
	method string,
	fullURL string,
	out any,
	okStatus ...int,
) error {
	req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return fmt.Errorf("build %s request: %w", method, err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call %s %s: %w", method, fullURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	statusOK := false
	for _, status := range okStatus {
		if resp.StatusCode == status {
			statusOK = true
			break
		}
	}
	if !statusOK {
		return fmt.Errorf(
			"%s %s failed with status %d: %s",
			method,
			fullURL,
			resp.StatusCode,
			strings.TrimSpace(string(respBody)),
		)
	}

	if out == nil || len(respBody) == 0 {
		return nil
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}

	return nil
}
