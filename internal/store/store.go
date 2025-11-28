package store

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"gocrud/internal/models"
)

// Store defines the interface for item persistence operations.
type Store interface {
	SaveItem(ctx context.Context, item *models.Item) error
	GetItem(ctx context.Context, id string) (*models.Item, error)
	DeleteItem(ctx context.Context, id string) error
	ListItems(ctx context.Context, typeFilter string, tagFilters []string, startTime, endTime *time.Time) ([]*models.Item, error)
}

// NewStore creates a Store implementation based on the STORAGE_BACKEND environment variable.
// Supports "memory" (default) and "firestore" backends.
// Returns an error if STORAGE_BACKEND is set to an unsupported value.
func NewStore(ctx context.Context, logger *log.Logger) (Store, error) {
	backend := os.Getenv("STORAGE_BACKEND")
	if backend == "" {
		backend = "memory" // Default to in-memory store
	}
	logger.Printf("Store initialization requested with backend=%q (defaulted if empty)", backend)

	switch backend {
	case "memory":
		return NewMemoryStore(logger), nil

	case "firestore":
		return NewFirestoreStore(ctx, logger)

	default:
		logger.Printf("ERROR: unsupported STORAGE_BACKEND value %q", backend)
		return nil, fmt.Errorf("unsupported STORAGE_BACKEND: %s (supported: memory, firestore)", backend)
	}
}

// NewStoreWithCollection creates a Store implementation with a specific collection name.
// For memory backend, this returns the same store as NewStore (memory store uses a single in-memory map).
// For firestore backend, this creates a store pointing to the specified collection.
func NewStoreWithCollection(ctx context.Context, logger *log.Logger, collectionName string) (Store, error) {
	backend := os.Getenv("STORAGE_BACKEND")
	if backend == "" {
		backend = "memory" // Default to in-memory store
	}
	logger.Printf("Store initialization with collection=%q using backend=%q", collectionName, backend)

	switch backend {
	case "memory":
		// Memory store doesn't support separate collections, return the default store
		return NewMemoryStore(logger), nil

	case "firestore":
		return NewFirestoreStoreWithCollection(ctx, logger, collectionName)

	default:
		logger.Printf("ERROR: unsupported STORAGE_BACKEND value %q", backend)
		return nil, fmt.Errorf("unsupported STORAGE_BACKEND: %s (supported: memory, firestore)", backend)
	}
}
