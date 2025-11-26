package store

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"gocrud/internal/models"

	"github.com/go-redis/redis/v8"
)

// Store defines the interface for item persistence operations.
type Store interface {
	SaveItem(ctx context.Context, item *models.Item) error
	GetItem(ctx context.Context, id string) (*models.Item, error)
	DeleteItem(ctx context.Context, id string) error
	ListItems(ctx context.Context, typeFilter string, tagFilters []string) ([]*models.Item, error)
}

// NewStore creates a Store implementation based on the STORAGE_BACKEND environment variable.
// Supports "memory" (default) and "redis" backends.
// Returns an error if STORAGE_BACKEND is set to an unsupported value or connection fails.
func NewStore(ctx context.Context, logger *log.Logger) (Store, error) {
	backend := os.Getenv("STORAGE_BACKEND")
	if backend == "" {
		backend = "memory" // Default to in-memory store
	}

	switch backend {
	case "memory":
		return NewMemoryStore(logger), nil
	case "redis":
		return newRedisStoreFromEnv(ctx, logger)

	default:
		return nil, fmt.Errorf("unsupported STORAGE_BACKEND: %s (supported: memory, redis)", backend)
	}
}

// newRedisStoreFromEnv creates a RedisStore instance from environment variables.
func newRedisStoreFromEnv(ctx context.Context, logger *log.Logger) (Store, error) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		return nil, fmt.Errorf("REDIS_ADDR environment variable is required when STORAGE_BACKEND=redis")
	}

	redisDB := os.Getenv("REDIS_DB")
	if redisDB == "" {
		redisDB = "0"
	}
	redisDBInt, err := strconv.Atoi(redisDB)
	if err != nil {
		return nil, fmt.Errorf("could not convert REDIS_DB to int: %v", err)
	}

	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr, DB: redisDBInt})

	// Test Redis connection
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("could not connect to redis (%s): %v", redisAddr, err)
	}

	logger.Printf("INFO: Successfully connected to Redis at %s (DB: %d)", redisAddr, redisDBInt)
	return NewRedisStore(redisClient), nil
}

