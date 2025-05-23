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
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	pipe := s.client.Pipeline()
	pipe.Set(ctx, key, data, 0)
	pipe.SAdd(ctx, "items", item.ID)
	pipe.SAdd(ctx, fmt.Sprintf("items:type:%s", item.Type), item.ID)
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
	// First get the item to know its type for cleanup
	item, err := s.GetItem(ctx, id)
	if err != nil {
		return err // This will return ErrNotFound if item doesn't exist
	}

	key := fmt.Sprintf("item:%s", id)
	pipe := s.client.Pipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, "items", id)
	pipe.SRem(ctx, fmt.Sprintf("items:type:%s", item.Type), id)
	_, err = pipe.Exec(ctx)
	return err
}

// ListItems returns all items in the store, optionally filtered by type.
func (s *RedisStore) ListItems(ctx context.Context, typeFilter string) ([]*Item, error) {
	var setKey string
	if typeFilter == "" {
		setKey = "items"
	} else {
		setKey = fmt.Sprintf("items:type:%s", typeFilter)
	}

	ids, err := s.client.SMembers(ctx, setKey).Result()
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
