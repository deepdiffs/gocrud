package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-redis/redis/v8"
)

// RedisStore provides item persistence in Redis.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore creates a new RedisStore.
func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}

// SaveItem stores a new or updated item in Redis.
func (s *RedisStore) SaveItem(ctx context.Context, item *Item) error {
	key := fmt.Sprintf("item:%s", item.ID)

	// For updates, we need to clean up old indexes first
	oldItem, err := s.GetItem(ctx, item.ID)
	if err != nil && err != ErrNotFound {
		return err
	}

	data, err := json.Marshal(item)
	if err != nil {
		return err
	}

	pipe := s.client.Pipeline()
	pipe.Set(ctx, key, data, 0)
	pipe.SAdd(ctx, "items", item.ID)

	// Clean up old indexes if this is an update
	if oldItem != nil {
		// Remove from old type index if type changed
		if oldItem.Type != item.Type {
			pipe.SRem(ctx, fmt.Sprintf("items:type:%s", oldItem.Type), item.ID)
		}
		// Remove from old tag indexes
		for _, oldTag := range oldItem.Tags {
			pipe.SRem(ctx, fmt.Sprintf("items:tag:%s", oldTag), item.ID)
		}
	}

	// Add to new indexes
	pipe.SAdd(ctx, fmt.Sprintf("items:type:%s", item.Type), item.ID)
	for _, tag := range item.Tags {
		pipe.SAdd(ctx, fmt.Sprintf("items:tag:%s", tag), item.ID)
	}

	_, err = pipe.Exec(ctx)
	return err
}

// GetItem retrieves an item by ID.
func (s *RedisStore) GetItem(ctx context.Context, id string) (*Item, error) {
	key := fmt.Sprintf("item:%s", id)
	data, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var item Item
	if err := json.Unmarshal([]byte(data), &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// DeleteItem removes an item by ID.
func (s *RedisStore) DeleteItem(ctx context.Context, id string) error {
	// First get the item to know its type and tags for cleanup
	item, err := s.GetItem(ctx, id)
	if err != nil {
		return err // This will return ErrNotFound if item doesn't exist
	}

	key := fmt.Sprintf("item:%s", id)
	pipe := s.client.Pipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, "items", id)
	pipe.SRem(ctx, fmt.Sprintf("items:type:%s", item.Type), id)

	// Remove from all tag indexes
	for _, tag := range item.Tags {
		pipe.SRem(ctx, fmt.Sprintf("items:tag:%s", tag), id)
	}

	_, err = pipe.Exec(ctx)
	return err
}

// ListItems returns all items in the store, optionally filtered by type and/or tags.
func (s *RedisStore) ListItems(ctx context.Context, typeFilter string, tagFilters []string) ([]*Item, error) {
	var setKeys []string

	// Build list of sets to intersect
	if typeFilter != "" {
		setKeys = append(setKeys, fmt.Sprintf("items:type:%s", typeFilter))
	}

	for _, tag := range tagFilters {
		setKeys = append(setKeys, fmt.Sprintf("items:tag:%s", tag))
	}

	var ids []string
	var err error

	if len(setKeys) == 0 {
		// No filters, return all items
		ids, err = s.client.SMembers(ctx, "items").Result()
	} else if len(setKeys) == 1 {
		// Single filter
		ids, err = s.client.SMembers(ctx, setKeys[0]).Result()
	} else {
		// Multiple filters - use intersection
		ids, err = s.client.SInter(ctx, setKeys...).Result()
	}

	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*Item{}, nil
	}

	pipe := s.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(ids))
	for i, id := range ids {
		key := fmt.Sprintf("item:%s", id)
		cmds[i] = pipe.Get(ctx, key)
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}
	items := make([]*Item, 0, len(ids))
	for _, cmd := range cmds {
		data, err := cmd.Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			return nil, err
		}
		var item Item
		if err := json.Unmarshal([]byte(data), &item); err != nil {
			return nil, err
		}
		items = append(items, &item)
	}
	return items, nil
}
