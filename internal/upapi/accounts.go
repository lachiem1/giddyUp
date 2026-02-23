package upapi

import (
	"context"
	"fmt"
	"net/url"
)

// ListAccounts calls GET /accounts with page[size]=50 and follows pagination.
func (c *Client) ListAccounts(ctx context.Context) (*ListResponse, error) {
	var page ListResponse
	if err := c.get(ctx, "/accounts", pageSizeQueryWithSize(accountsPageSize), &page); err != nil {
		return nil, err
	}

	out := &ListResponse{
		Data: append([]Resource{}, page.Data...),
	}
	out.Links = page.Links

	nextURL := page.Links.Next
	for nextURL != nil && *nextURL != "" {
		resolvedURL, err := resolveListURL(c.baseURL, *nextURL)
		if err != nil {
			return nil, err
		}

		page = ListResponse{}
		if err := c.getURL(ctx, resolvedURL, &page); err != nil {
			return nil, err
		}
		out.Data = append(out.Data, page.Data...)
		out.Links = page.Links
		nextURL = page.Links.Next
	}

	return out, nil
}

// GetAccount calls GET /accounts/{id}.
func (c *Client) GetAccount(ctx context.Context, id string) (*ResourceResponse, error) {
	var out ResourceResponse
	if err := c.get(ctx, "/accounts/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func resolveListURL(baseURL, next string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}
	ref, err := url.Parse(next)
	if err != nil {
		return "", fmt.Errorf("parse next page URL: %w", err)
	}
	return base.ResolveReference(ref).String(), nil
}
