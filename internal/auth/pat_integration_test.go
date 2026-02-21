//go:build integration
// +build integration

package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestKeyringPATCanPingUpAPI(t *testing.T) {
	t.Setenv("UP_PAT", "")

	pat, err := loadFromKeyring()
	if err != nil {
		t.Fatalf("failed to load PAT from keyring: %v", err)
	}
	if pat == "" {
		t.Fatal("keyring returned an empty PAT")
	}

	req, err := http.NewRequest(http.MethodGet, "https://api.up.com.au/api/v1/util/ping", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+pat)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("ping request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Meta struct {
			ID          string `json:"id"`
			StatusEmoji string `json:"statusEmoji"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if payload.Meta.ID == "" {
		t.Fatal("response missing meta.id")
	}
	if payload.Meta.StatusEmoji != "⚡️" {
		t.Fatalf("unexpected statusEmoji: got %q, want %q", payload.Meta.StatusEmoji, "⚡️")
	}

	if !strings.Contains(payload.Meta.ID, "-") {
		t.Fatalf("response meta.id does not look like a UUID: %q", payload.Meta.ID)
	}

	t.Logf("ping succeeded with meta.id=%s", payload.Meta.ID)
}

func Example_integrationTestCommand() {
	fmt.Println("go test -tags=integration ./internal/auth -run TestKeyringPATCanPingUpAPI -v")
	// Output: go test -tags=integration ./internal/auth -run TestKeyringPATCanPingUpAPI -v
}
