package models

import (
	"encoding/json"
	"time"
)

// Item represents a generic item with metadata and raw JSON data.
type Item struct {
	ID             string      `json:"id"`
	Type           string      `json:"type"`
	Name           string      `json:"name"`
	Timestamp      time.Time   `json:"timestamp"`
	Tags           []string    `json:"tags"`
	Data           interface{} `json:"data"`
	CreatedAt      time.Time   `json:"createdAt"`
	LastModified   time.Time   `json:"lastModified"`
	ImageData      *string     `json:"image_data,omitempty" firestore:"image_data,omitempty"`
	ImageFormat    *string     `json:"image_format,omitempty" firestore:"image_format,omitempty"`
	ImageSizeBytes *int64      `json:"image_size_bytes,omitempty" firestore:"image_size_bytes,omitempty"`
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
