package upapi

import (
	"context"
	"net/url"
)

// ListCategories calls GET /categories.
// This endpoint is not paginated in Up API docs.
func (c *Client) ListCategories(ctx context.Context, parentID string) (*ListResponse, error) {
	var query url.Values
	if parentID != "" {
		query = url.Values{}
		query.Set("filter[parent]", parentID)
	}

	var out ListResponse
	if err := c.get(ctx, "/categories", query, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetCategory calls GET /categories/{id}.
func (c *Client) GetCategory(ctx context.Context, id string) (*ResourceResponse, error) {
	var out ResourceResponse
	if err := c.get(ctx, "/categories/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
