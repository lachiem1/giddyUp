package upapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPingSuccess(t *testing.T) {
	var seenReq *http.Request
	client := NewWithBaseURL("test-token", "https://example.test")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seenReq = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping() unexpected error: %v", err)
	}
	if seenReq == nil {
		t.Fatal("no request captured")
	}
	if seenReq.URL.Path != "/util/ping" {
		t.Fatalf("path = %q, want %q", seenReq.URL.Path, "/util/ping")
	}
	if seenReq.Header.Get("Authorization") != "Bearer test-token" {
		t.Fatalf(
			"Authorization header = %q, want %q",
			seenReq.Header.Get("Authorization"),
			"Bearer test-token",
		)
	}
}

func TestPingNon200Fails(t *testing.T) {
	client := NewWithBaseURL("test-token", "https://example.test")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	err := client.Ping(context.Background())
	if err == nil {
		t.Fatal("Ping() error = nil, want non-nil")
	}
}

func TestPaginatedRoutesUsePageSize15(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context, *Client) error
		path string
	}{
		{
			name: "attachments",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ListAttachments(ctx)
				return err
			},
			path: "/attachments",
		},
		{
			name: "tags",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ListTags(ctx)
				return err
			},
			path: "/tags",
		},
		{
			name: "transactions",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ListTransactions(ctx, TransactionListOptions{})
				return err
			},
			path: "/transactions",
		},
		{
			name: "transactions by account",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ListTransactionsByAccount(ctx, "account-id", TransactionListOptions{})
				return err
			},
			path: "/accounts/account-id/transactions",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var seenReq *http.Request
			client := NewWithBaseURL("test-token", "https://example.test")
			client.httpClient = &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					seenReq = req
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"data":[],"links":{"prev":null,"next":null}}`)),
						Header:     make(http.Header),
					}, nil
				}),
			}

			if err := tc.call(context.Background(), client); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if seenReq == nil {
				t.Fatal("no request captured")
			}
			if seenReq.URL.Path != tc.path {
				t.Fatalf("path = %q, want %q", seenReq.URL.Path, tc.path)
			}
			if seenReq.URL.Query().Get("page[size]") != "15" {
				t.Fatalf("page[size] = %q, want %q", seenReq.URL.Query().Get("page[size]"), "15")
			}
		})
	}
}

func TestListAccountsUsesPageSize50(t *testing.T) {
	var seenReq *http.Request
	client := NewWithBaseURL("test-token", "https://example.test")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seenReq = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(
					strings.NewReader(`{"data":[],"links":{"prev":null,"next":null}}`),
				),
				Header: make(http.Header),
			}, nil
		}),
	}

	if _, err := client.ListAccounts(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seenReq == nil {
		t.Fatal("no request captured")
	}
	if seenReq.URL.Path != "/accounts" {
		t.Fatalf("path = %q, want %q", seenReq.URL.Path, "/accounts")
	}
	if seenReq.URL.Query().Get("page[size]") != "50" {
		t.Fatalf("page[size] = %q, want %q", seenReq.URL.Query().Get("page[size]"), "50")
	}
}

func TestListAccountsFollowsPagination(t *testing.T) {
	requests := make([]*http.Request, 0, 2)
	client := NewWithBaseURL("test-token", "https://example.test/api/v1")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests = append(requests, req)
			body := `{"data":[{"type":"accounts","id":"id-1"}],"links":{"prev":null,"next":"https://example.test/api/v1/accounts?page[after]=cursor"}}`
			if req.URL.Query().Get("page[after]") == "cursor" {
				body = `{"data":[{"type":"accounts","id":"id-2"}],"links":{"prev":null,"next":null}}`
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	accounts, err := client.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(accounts.Data))
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
}
