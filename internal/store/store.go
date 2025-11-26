package store

import (
	"context"
	"fmt"
	"log"
	"os"

	"gocrud/internal/models"
)

// Store defines the interface for item persistence operations.
type Store interface {
	SaveItem(ctx context.Context, item *models.Item) error
	GetItem(ctx context.Context, id string) (*models.Item, error)
	DeleteItem(ctx context.Context, id string) error
	ListItems(ctx context.Context, typeFilter string, tagFilters []string) ([]*models.Item, error)
}

// NewStore creates a Store implementation based on the STORAGE_BACKEND environment variable.
// Supports "memory" (default) backend.
// Returns an error if STORAGE_BACKEND is set to an unsupported value.
func NewStore(ctx context.Context, logger *log.Logger) (Store, error) {
	backend := os.Getenv("STORAGE_BACKEND")
	if backend == "" {
		backend = "memory" // Default to in-memory store
	}

	switch backend {
	case "memory":
		return NewMemoryStore(logger), nil

	default:
		return nil, fmt.Errorf("unsupported STORAGE_BACKEND: %s (supported: memory)", backend)
	}
}

