package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/go-redis/redis/v8"
)

// RedisStore provides item persistence in Redis.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore creates a new RedisStore.
func NewRedisStore(client *redis.Client) *RedisStore {
	log.Printf("INFO: Creating new RedisStore instance")
	return &RedisStore{client: client}
}

// SaveItem stores a new or updated item in Redis.
func (s *RedisStore) SaveItem(ctx context.Context, item *Item) error {
	log.Printf("INFO: Starting SaveItem operation for item ID: %s", item.ID)

	key := fmt.Sprintf("item:%s", item.ID)
	log.Printf("INFO: Using Redis key: %s", key)

	// For updates, we need to clean up old indexes first
	log.Printf("INFO: Checking if item exists for cleanup of old indexes")
	oldItem, err := s.GetItem(ctx, item.ID)
	if err != nil && err != ErrNotFound {
		log.Printf("ERROR: Failed to get existing item for cleanup: %v", err)
		return err
	}

	if oldItem != nil {
		log.Printf("INFO: Updating existing item (ID: %s), cleaning up old indexes", item.ID)
	} else {
		log.Printf("INFO: Creating new item (ID: %s)", item.ID)
	}

	data, err := json.Marshal(item)
	if err != nil {
		log.Printf("ERROR: Failed to marshal item to JSON: %v", err)
		return err
	}
	log.Printf("INFO: Successfully marshaled item to JSON, size: %d bytes", len(data))

	pipe := s.client.Pipeline()
	pipe.Set(ctx, key, data, 0)
	pipe.SAdd(ctx, "items", item.ID)

	// Clean up old indexes if this is an update
	if oldItem != nil {
		// Remove from old type index if type changed
		if oldItem.Type != item.Type {
			log.Printf("INFO: Item type changed from %s to %s, removing from old type index", oldItem.Type, item.Type)
			pipe.SRem(ctx, fmt.Sprintf("items:type:%s", oldItem.Type), item.ID)
		}
		// Remove from old tag indexes
		for _, oldTag := range oldItem.Tags {
			log.Printf("INFO: Removing item from old tag index: items:tag:%s", oldTag)
			pipe.SRem(ctx, fmt.Sprintf("items:tag:%s", oldTag), item.ID)
		}
	}

	// Add to new indexes
	log.Printf("INFO: Adding item to type index: items:type:%s", item.Type)
	pipe.SAdd(ctx, fmt.Sprintf("items:type:%s", item.Type), item.ID)
	for _, tag := range item.Tags {
		log.Printf("INFO: Adding item to tag index: items:tag:%s", tag)
		pipe.SAdd(ctx, fmt.Sprintf("items:tag:%s", tag), item.ID)
	}

	log.Printf("INFO: Executing Redis pipeline for SaveItem operation")
	_, err = pipe.Exec(ctx)
	if err != nil {
		log.Printf("ERROR: Failed to execute Redis pipeline for SaveItem: %v", err)
		return err
	}

	log.Printf("INFO: Successfully completed SaveItem operation for item ID: %s", item.ID)
	return err
}

// GetItem retrieves an item by ID.
func (s *RedisStore) GetItem(ctx context.Context, id string) (*Item, error) {
	log.Printf("INFO: Starting GetItem operation for item ID: %s", id)

	key := fmt.Sprintf("item:%s", id)
	log.Printf("INFO: Using Redis key: %s", key)

	data, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			log.Printf("INFO: Item not found in Redis (ID: %s)", id)
			return nil, ErrNotFound
		}
		log.Printf("ERROR: Failed to get item from Redis (ID: %s): %v", id, err)
		return nil, err
	}

	log.Printf("INFO: Successfully retrieved item data from Redis (ID: %s), size: %d bytes", id, len(data))

	var item Item
	if err := json.Unmarshal([]byte(data), &item); err != nil {
		log.Printf("ERROR: Failed to unmarshal item JSON (ID: %s): %v", id, err)
		return nil, err
	}

	log.Printf("INFO: Successfully completed GetItem operation for item ID: %s", id)
	return &item, nil
}

// DeleteItem removes an item by ID.
func (s *RedisStore) DeleteItem(ctx context.Context, id string) error {
	log.Printf("INFO: Starting DeleteItem operation for item ID: %s", id)

	// First get the item to know its type and tags for cleanup
	log.Printf("INFO: Retrieving item details for cleanup of indexes")
	item, err := s.GetItem(ctx, id)
	if err != nil {
		log.Printf("ERROR: Failed to get item for deletion cleanup (ID: %s): %v", id, err)
		return err // This will return ErrNotFound if item doesn't exist
	}

	key := fmt.Sprintf("item:%s", id)
	log.Printf("INFO: Using Redis key for deletion: %s", key)

	pipe := s.client.Pipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, "items", id)
	pipe.SRem(ctx, fmt.Sprintf("items:type:%s", item.Type), id)

	log.Printf("INFO: Removing item from type index: items:type:%s", item.Type)

	// Remove from all tag indexes
	for _, tag := range item.Tags {
		log.Printf("INFO: Removing item from tag index: items:tag:%s", tag)
		pipe.SRem(ctx, fmt.Sprintf("items:tag:%s", tag), id)
	}

	log.Printf("INFO: Executing Redis pipeline for DeleteItem operation")
	_, err = pipe.Exec(ctx)
	if err != nil {
		log.Printf("ERROR: Failed to execute Redis pipeline for DeleteItem: %v", err)
		return err
	}

	log.Printf("INFO: Successfully completed DeleteItem operation for item ID: %s", id)
	return err
}

// ListItems returns all items in the store, optionally filtered by type and/or tags.
func (s *RedisStore) ListItems(ctx context.Context, typeFilter string, tagFilters []string) ([]*Item, error) {
	log.Printf("INFO: Starting ListItems operation - typeFilter: %q, tagFilters: %v", typeFilter, tagFilters)

	var setKeys []string

	// Build list of sets to intersect
	if typeFilter != "" {
		typeKey := fmt.Sprintf("items:type:%s", typeFilter)
		setKeys = append(setKeys, typeKey)
		log.Printf("INFO: Added type filter key: %s", typeKey)
	}

	for _, tag := range tagFilters {
		tagKey := fmt.Sprintf("items:tag:%s", tag)
		setKeys = append(setKeys, tagKey)
		log.Printf("INFO: Added tag filter key: %s", tagKey)
	}

	var ids []string
	var err error

	if len(setKeys) == 0 {
		// No filters, return all items
		log.Printf("INFO: No filters applied, retrieving all items")
		ids, err = s.client.SMembers(ctx, "items").Result()
	} else if len(setKeys) == 1 {
		// Single filter
		log.Printf("INFO: Using single filter: %s", setKeys[0])
		ids, err = s.client.SMembers(ctx, setKeys[0]).Result()
	} else {
		// Multiple filters - use intersection
		log.Printf("INFO: Using intersection of %d filters", len(setKeys))
		ids, err = s.client.SInter(ctx, setKeys...).Result()
	}

	if err != nil {
		log.Printf("ERROR: Failed to retrieve item IDs from Redis: %v", err)
		return nil, err
	}

	log.Printf("INFO: Retrieved %d item IDs from Redis", len(ids))

	if len(ids) == 0 {
		log.Printf("INFO: No items found matching filters")
		return []*Item{}, nil
	}

	log.Printf("INFO: Starting batch retrieval of %d items", len(ids))
	pipe := s.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(ids))
	for i, id := range ids {
		key := fmt.Sprintf("item:%s", id)
		cmds[i] = pipe.Get(ctx, key)
	}

	log.Printf("INFO: Executing Redis pipeline for batch item retrieval")
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		log.Printf("ERROR: Failed to execute Redis pipeline for batch retrieval: %v", err)
		return nil, err
	}

	items := make([]*Item, 0, len(ids))
	successCount := 0
	missingCount := 0
	errorCount := 0

	for i, cmd := range cmds {
		data, err := cmd.Result()
		if err != nil {
			if err == redis.Nil {
				log.Printf("WARN: Item not found in Redis (ID: %s)", ids[i])
				missingCount++
				continue
			}
			log.Printf("ERROR: Failed to get item data (ID: %s): %v", ids[i], err)
			errorCount++
			return nil, err
		}

		var item Item
		if err := json.Unmarshal([]byte(data), &item); err != nil {
			log.Printf("ERROR: Failed to unmarshal item JSON (ID: %s): %v", ids[i], err)
			errorCount++
			return nil, err
		}

		items = append(items, &item)
		successCount++
	}

	log.Printf("INFO: Successfully completed ListItems operation - retrieved: %d, missing: %d, errors: %d",
		successCount, missingCount, errorCount)
	return items, nil
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
