package upapi

import "context"

// Ping calls GET /util/ping and returns nil only when status is 200.
func (c *Client) Ping(ctx context.Context) error {
	return c.get(ctx, "/util/ping", nil, nil)
}
