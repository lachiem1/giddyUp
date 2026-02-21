//go:build integration
// +build integration

package upapi

import (
	"fmt"
	"testing"

	"github.com/lachiem1/giddyUp/internal/auth"
)

func integrationClient(t *testing.T) *Client {
	t.Helper()
	pat, err := auth.LoadPAT()
	if err != nil {
		t.Fatalf("failed to load PAT: %v", err)
	}
	return New(pat)
}

func Example_integrationRoutesCommand() {
	fmt.Println("go test -tags=integration ./internal/upapi -v")
	// Output: go test -tags=integration ./internal/upapi -v
}
