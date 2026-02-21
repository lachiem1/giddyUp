package upapi

import "context"

// ListAccounts calls GET /accounts with page[size]=15.
func (c *Client) ListAccounts(ctx context.Context) (*ListResponse, error) {
	var out ListResponse
	if err := c.get(ctx, "/accounts", pageSizeQuery(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAccount calls GET /accounts/{id}.
func (c *Client) GetAccount(ctx context.Context, id string) (*ResourceResponse, error) {
	var out ResourceResponse
	if err := c.get(ctx, "/accounts/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
