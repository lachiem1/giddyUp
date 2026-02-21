//go:build integration
// +build integration

package upapi

import (
	"context"
	"testing"
)

func TestTagsRoutesIntegration(t *testing.T) {
	client := integrationClient(t)

	tags, err := client.ListTags(context.Background())
	if err != nil {
		t.Fatalf("ListTags() failed: %v", err)
	}
	if tags == nil {
		t.Fatal("ListTags() returned nil response")
	}
}
