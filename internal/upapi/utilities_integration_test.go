//go:build integration
// +build integration

package upapi

import (
	"context"
	"testing"
)

func TestUtilitiesPingIntegration(t *testing.T) {
	client := integrationClient(t)
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() failed: %v", err)
	}
}
