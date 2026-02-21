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
