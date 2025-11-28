package models

import (
	"encoding/json"
	"time"
)

// Item represents a generic item with metadata and raw JSON data.
type Item struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Name         string    `json:"name"`
	Timestamp    time.Time `json:"timestamp"`
	Tags         []string  `json:"tags"`
	Data         string    `json:"data"`
	CreatedAt    time.Time `json:"createdAt"`
	LastModified time.Time `json:"lastModified"`
}

// CreateItemRequest is the payload for creating a new item.
type CreateItemRequest struct {
	ID        string          `json:"id,omitempty"`
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Timestamp time.Time       `json:"timestamp"`
	Tags      []string        `json:"tags"`
	Data      json.RawMessage `json:"data"`
}

// UpdateItemRequest is the payload for updating an existing item.
type UpdateItemRequest struct {
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Timestamp time.Time       `json:"timestamp"`
	Tags      []string        `json:"tags"`
	Data      json.RawMessage `json:"data"`
}
