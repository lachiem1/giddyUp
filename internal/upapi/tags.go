package upapi

import "context"

// ListTags calls GET /tags with page[size]=15.
func (c *Client) ListTags(ctx context.Context) (*ListResponse, error) {
	var out ListResponse
	if err := c.get(ctx, "/tags", pageSizeQuery(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}
