package upapi

import (
	"net/url"
	"strconv"
)

// Resource models a generic JSON:API resource object.
type Resource struct {
	Type          string                            `json:"type"`
	ID            string                            `json:"id"`
	Attributes    map[string]any                    `json:"attributes,omitempty"`
	Relationships map[string]map[string]interface{} `json:"relationships,omitempty"`
	Links         map[string]string                 `json:"links,omitempty"`
}

// ResourceResponse models endpoints returning a single resource.
type ResourceResponse struct {
	Data Resource `json:"data"`
}

// ListResponse models paginated list endpoints.
type ListResponse struct {
	Data  []Resource `json:"data"`
	Links struct {
		Prev *string `json:"prev"`
		Next *string `json:"next"`
	} `json:"links"`
}

func pageSizeQuery() url.Values {
	query := url.Values{}
	query.Set("page[size]", strconv.Itoa(defaultPageSize))
	return query
}
