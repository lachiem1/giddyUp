package upapi

import (
	"context"
	"net/url"
)

// TransactionListOptions supports list filters in Up docs.
type TransactionListOptions struct {
	Status   string
	SinceRFC string
	UntilRFC string
	Category string
	Tag      string
}

// ListTransactions calls GET /transactions with page[size]=15.
func (c *Client) ListTransactions(ctx context.Context, opts TransactionListOptions) (*ListResponse, error) {
	query := pageSizeQuery()
	applyTransactionFilters(query, opts)

	var out ListResponse
	if err := c.get(ctx, "/transactions", query, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTransaction calls GET /transactions/{id}.
func (c *Client) GetTransaction(ctx context.Context, id string) (*ResourceResponse, error) {
	var out ResourceResponse
	if err := c.get(ctx, "/transactions/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListTransactionsByAccount calls GET /accounts/{accountId}/transactions with page[size]=15.
func (c *Client) ListTransactionsByAccount(
	ctx context.Context,
	accountID string,
	opts TransactionListOptions,
) (*ListResponse, error) {
	query := pageSizeQuery()
	applyTransactionFilters(query, opts)

	var out ListResponse
	if err := c.get(ctx, "/accounts/"+accountID+"/transactions", query, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func applyTransactionFilters(query url.Values, opts TransactionListOptions) {
	if opts.Status != "" {
		query.Set("filter[status]", opts.Status)
	}
	if opts.SinceRFC != "" {
		query.Set("filter[since]", opts.SinceRFC)
	}
	if opts.UntilRFC != "" {
		query.Set("filter[until]", opts.UntilRFC)
	}
	if opts.Category != "" {
		query.Set("filter[category]", opts.Category)
	}
	if opts.Tag != "" {
		query.Set("filter[tag]", opts.Tag)
	}
}
