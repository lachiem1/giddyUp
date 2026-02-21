package upapi

import "context"

// ListAttachments calls GET /attachments with page[size]=15.
func (c *Client) ListAttachments(ctx context.Context) (*ListResponse, error) {
	var out ListResponse
	if err := c.get(ctx, "/attachments", pageSizeQuery(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAttachment calls GET /attachments/{id}.
func (c *Client) GetAttachment(ctx context.Context, id string) (*ResourceResponse, error) {
	var out ResourceResponse
	if err := c.get(ctx, "/attachments/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
