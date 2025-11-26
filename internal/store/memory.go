package store

import (
	"context"
	"log"
	"sync"

	"gocrud/internal/errors"
	"gocrud/internal/models"
)

// MemoryStore provides item persistence in memory using maps.
type MemoryStore struct {
	mu        sync.RWMutex
	items     map[string]*models.Item
	typeIndex map[string]map[string]bool // type -> set of item IDs
	tagIndex  map[string]map[string]bool // tag -> set of item IDs
	logger    *log.Logger
}

// NewMemoryStore creates a new MemoryStore.
func NewMemoryStore(logger *log.Logger) *MemoryStore {
	logger.Printf("INFO: Creating new MemoryStore instance")
	return &MemoryStore{
		items:     make(map[string]*models.Item),
		typeIndex: make(map[string]map[string]bool),
		tagIndex:  make(map[string]map[string]bool),
		logger:    logger,
	}
}

// SaveItem stores a new or updated item in memory.
func (s *MemoryStore) SaveItem(ctx context.Context, item *models.Item) error {
	s.logger.Printf("INFO: Starting SaveItem operation for item ID: %s", item.ID)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if item exists for cleanup of old indexes
	oldItem, exists := s.items[item.ID]
	if exists {
		s.logger.Printf("INFO: Updating existing item (ID: %s), cleaning up old indexes", item.ID)
		// Remove from old type index if type changed
		if oldItem.Type != item.Type {
			s.logger.Printf("INFO: Item type changed from %s to %s, removing from old type index", oldItem.Type, item.Type)
			if typeSet, ok := s.typeIndex[oldItem.Type]; ok {
				delete(typeSet, item.ID)
				if len(typeSet) == 0 {
					delete(s.typeIndex, oldItem.Type)
				}
			}
		}
		// Remove from old tag indexes
		for _, oldTag := range oldItem.Tags {
			s.logger.Printf("INFO: Removing item from old tag index: %s", oldTag)
			if tagSet, ok := s.tagIndex[oldTag]; ok {
				delete(tagSet, item.ID)
				if len(tagSet) == 0 {
					delete(s.tagIndex, oldTag)
				}
			}
		}
	} else {
		s.logger.Printf("INFO: Creating new item (ID: %s)", item.ID)
	}

	// Store the item
	s.items[item.ID] = item

	// Add to type index
	if s.typeIndex[item.Type] == nil {
		s.typeIndex[item.Type] = make(map[string]bool)
	}
	s.typeIndex[item.Type][item.ID] = true
	s.logger.Printf("INFO: Added item to type index: %s", item.Type)

	// Add to tag indexes
	for _, tag := range item.Tags {
		if s.tagIndex[tag] == nil {
			s.tagIndex[tag] = make(map[string]bool)
		}
		s.tagIndex[tag][item.ID] = true
		s.logger.Printf("INFO: Added item to tag index: %s", tag)
	}

	s.logger.Printf("INFO: Successfully completed SaveItem operation for item ID: %s", item.ID)
	return nil
}

// GetItem retrieves an item by ID.
func (s *MemoryStore) GetItem(ctx context.Context, id string) (*models.Item, error) {
	s.logger.Printf("INFO: Starting GetItem operation for item ID: %s", id)

	s.mu.RLock()
	defer s.mu.RUnlock()

	item, exists := s.items[id]
	if !exists {
		s.logger.Printf("INFO: Item not found (ID: %s)", id)
		return nil, errors.ErrNotFound
	}

	s.logger.Printf("INFO: Successfully completed GetItem operation for item ID: %s", id)
	return item, nil
}

// DeleteItem removes an item by ID.
func (s *MemoryStore) DeleteItem(ctx context.Context, id string) error {
	s.logger.Printf("INFO: Starting DeleteItem operation for item ID: %s", id)

	s.mu.Lock()
	defer s.mu.Unlock()

	item, exists := s.items[id]
	if !exists {
		s.logger.Printf("INFO: Item not found for deletion (ID: %s)", id)
		return errors.ErrNotFound
	}

	// Remove from type index
	if typeSet, ok := s.typeIndex[item.Type]; ok {
		delete(typeSet, id)
		if len(typeSet) == 0 {
			delete(s.typeIndex, item.Type)
		}
		s.logger.Printf("INFO: Removed item from type index: %s", item.Type)
	}

	// Remove from tag indexes
	for _, tag := range item.Tags {
		if tagSet, ok := s.tagIndex[tag]; ok {
			delete(tagSet, id)
			if len(tagSet) == 0 {
				delete(s.tagIndex, tag)
			}
			s.logger.Printf("INFO: Removed item from tag index: %s", tag)
		}
	}

	// Remove the item
	delete(s.items, id)

	s.logger.Printf("INFO: Successfully completed DeleteItem operation for item ID: %s", id)
	return nil
}

// ListItems returns all items in the store, optionally filtered by type and/or tags.
func (s *MemoryStore) ListItems(ctx context.Context, typeFilter string, tagFilters []string) ([]*models.Item, error) {
	s.logger.Printf("INFO: Starting ListItems operation - typeFilter: %q, tagFilters: %v", typeFilter, tagFilters)

	s.mu.RLock()
	defer s.mu.RUnlock()

	var candidateIDs map[string]bool

	// Build candidate set based on filters
	if typeFilter != "" {
		typeSet, ok := s.typeIndex[typeFilter]
		if !ok {
			s.logger.Printf("INFO: No items found for type filter: %s", typeFilter)
			return []*models.Item{}, nil
		}
		candidateIDs = make(map[string]bool)
		for id := range typeSet {
			candidateIDs[id] = true
		}
		s.logger.Printf("INFO: Type filter applied, %d candidates", len(candidateIDs))
	} else {
		// No type filter, start with all items
		candidateIDs = make(map[string]bool)
		for id := range s.items {
			candidateIDs[id] = true
		}
		s.logger.Printf("INFO: No type filter, starting with all %d items", len(candidateIDs))
	}

	// Apply tag filters (intersection)
	for _, tag := range tagFilters {
		tagSet, ok := s.tagIndex[tag]
		if !ok {
			s.logger.Printf("INFO: No items found for tag filter: %s", tag)
			return []*models.Item{}, nil
		}

		// Intersect with candidate set
		for id := range candidateIDs {
			if !tagSet[id] {
				delete(candidateIDs, id)
			}
		}
		s.logger.Printf("INFO: Tag filter %s applied, %d candidates remaining", tag, len(candidateIDs))
	}

	if len(candidateIDs) == 0 {
		s.logger.Printf("INFO: No items found matching filters")
		return []*models.Item{}, nil
	}

	// Build result list
	items := make([]*models.Item, 0, len(candidateIDs))
	for id := range candidateIDs {
		if item, exists := s.items[id]; exists {
			items = append(items, item)
		}
	}

	s.logger.Printf("INFO: Successfully completed ListItems operation - retrieved %d items", len(items))
	return items, nil
}
