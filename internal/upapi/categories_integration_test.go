//go:build integration
// +build integration

package upapi

import (
	"context"
	"testing"
)

func TestCategoriesRoutesIntegration(t *testing.T) {
	client := integrationClient(t)

	categories, err := client.ListCategories(context.Background(), "")
	if err != nil {
		t.Fatalf("ListCategories() failed: %v", err)
	}
	if len(categories.Data) == 0 {
		t.Fatal("ListCategories() returned no categories")
	}

	categoryID := categories.Data[0].ID
	category, err := client.GetCategory(context.Background(), categoryID)
	if err != nil {
		t.Fatalf("GetCategory() failed: %v", err)
	}
	if category.Data.ID != categoryID {
		t.Fatalf("GetCategory() id mismatch: got %q want %q", category.Data.ID, categoryID)
	}

	_, err = client.ListCategories(context.Background(), categoryID)
	if err != nil {
		t.Fatalf("ListCategories(parent) failed: %v", err)
	}
}
