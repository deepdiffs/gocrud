package main

import (
	"encoding/json"
	"time"
)

// Item represents a generic item with metadata and raw JSON data.
type Item struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Tags         []string        `json:"tags"`
	Data         json.RawMessage `json:"data"`
	CreatedAt    time.Time       `json:"createdAt"`
	LastModified time.Time       `json:"lastModified"`
}

// CreateItemRequest is the payload for creating a new item.
type CreateItemRequest struct {
	Type string          `json:"type"`
	Tags []string        `json:"tags"`
	Data json.RawMessage `json:"data"`
}

// UpdateItemRequest is the payload for updating an existing item.
type UpdateItemRequest struct {
	Type string          `json:"type"`
	Tags []string        `json:"tags"`
	Data json.RawMessage `json:"data"`
}
