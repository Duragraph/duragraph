package dto

import "time"

// PutStoreItemRequest represents the request to store an item.
type PutStoreItemRequest struct {
	Namespace []string               `json:"namespace"`
	Key       string                 `json:"key"`
	Value     map[string]interface{} `json:"value"`
	TTL       *int                   `json:"ttl,omitempty"`
}

// GetStoreItemRequest represents query params for retrieving an item.
type GetStoreItemRequest struct {
	Namespace  string `query:"namespace"`
	Key        string `query:"key"`
	RefreshTTL bool   `query:"refresh_ttl"`
}

// DeleteStoreItemRequest represents the request to delete an item.
type DeleteStoreItemRequest struct {
	Namespace []string `json:"namespace"`
	Key       string   `json:"key"`
}

// SearchStoreItemsRequest represents the request to search items.
type SearchStoreItemsRequest struct {
	NamespacePrefix []string               `json:"namespace_prefix"`
	Filter          map[string]interface{} `json:"filter,omitempty"`
	Limit           int                    `json:"limit,omitempty"`
	Offset          int                    `json:"offset,omitempty"`
	Query           string                 `json:"query,omitempty"`
	RefreshTTL      *bool                  `json:"refresh_ttl,omitempty"`
}

// ListNamespacesRequest represents the request to list namespaces.
type ListNamespacesRequest struct {
	Prefix   []string `json:"prefix,omitempty"`
	Suffix   []string `json:"suffix,omitempty"`
	MaxDepth *int     `json:"max_depth,omitempty"`
	Limit    int      `json:"limit,omitempty"`
	Offset   int      `json:"offset,omitempty"`
}

// StoreItemResponse represents a stored item in responses.
type StoreItemResponse struct {
	Namespace []string               `json:"namespace"`
	Key       string                 `json:"key"`
	Value     map[string]interface{} `json:"value"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// SearchStoreItemsResponse represents the response from searching items.
type SearchStoreItemsResponse struct {
	Items []StoreItemResponse `json:"items"`
}
