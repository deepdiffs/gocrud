package store

import (
	"context"
	"log"
	"os"

	"gocrud/internal/errors"
	"gocrud/internal/models"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// FirestoreStore provides item persistence using Google Cloud Firestore.
type FirestoreStore struct {
	client         *firestore.Client
	collectionName string
	logger         *log.Logger
}

// NewFirestoreStore creates a new FirestoreStore with default collection name.
// Requires GOOGLE_CLOUD_PROJECT environment variable.
// Optional: FIRESTORE_COLLECTION (defaults to "items").
func NewFirestoreStore(ctx context.Context, logger *log.Logger) (*FirestoreStore, error) {
	collectionName := os.Getenv("FIRESTORE_COLLECTION")
	if collectionName == "" {
		collectionName = "items"
	}
	return NewFirestoreStoreWithCollection(ctx, logger, collectionName)
}

// NewFirestoreStoreWithCollection creates a new FirestoreStore with a specific collection name.
// Requires GOOGLE_CLOUD_PROJECT environment variable.
func NewFirestoreStoreWithCollection(ctx context.Context, logger *log.Logger, collectionName string) (*FirestoreStore, error) {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		return nil, errors.ErrMissingConfig
	}

	logger.Printf("INFO: Creating Firestore client for project: %s, collection: %s", projectID, collectionName)

	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		logger.Printf("ERROR: Failed to create Firestore client: %v", err)
		return nil, err
	}

	logger.Printf("INFO: Successfully created Firestore client")
	return &FirestoreStore{
		client:         client,
		collectionName: collectionName,
		logger:         logger,
	}, nil
}

// Close closes the Firestore client connection.
func (s *FirestoreStore) Close() error {
	s.logger.Printf("INFO: Closing Firestore client")
	return s.client.Close()
}

// SaveItem stores a new or updated item in Firestore.
func (s *FirestoreStore) SaveItem(ctx context.Context, item *models.Item) error {
	s.logger.Printf("INFO: Starting SaveItem operation for item ID: %s in collection: %s", item.ID, s.collectionName)

	_, err := s.client.Collection(s.collectionName).Doc(item.ID).Set(ctx, item)
	if err != nil {
		s.logger.Printf("ERROR: Failed to save item (ID: %s): %v", item.ID, err)
		return err
	}

	s.logger.Printf("INFO: Successfully completed SaveItem operation for item ID: %s", item.ID)
	return nil
}

// GetItem retrieves an item by ID from Firestore.
func (s *FirestoreStore) GetItem(ctx context.Context, id string) (*models.Item, error) {
	s.logger.Printf("INFO: Starting GetItem operation for item ID: %s in collection: %s", id, s.collectionName)

	doc, err := s.client.Collection(s.collectionName).Doc(id).Get(ctx)
	if err != nil {
		s.logger.Printf("INFO: Item not found (ID: %s): %v", id, err)
		return nil, errors.ErrNotFound
	}

	var item models.Item
	if err := doc.DataTo(&item); err != nil {
		s.logger.Printf("ERROR: Failed to unmarshal item (ID: %s): %v", id, err)
		return nil, err
	}

	s.logger.Printf("INFO: Successfully completed GetItem operation for item ID: %s", id)
	return &item, nil
}

// DeleteItem removes an item by ID from Firestore.
func (s *FirestoreStore) DeleteItem(ctx context.Context, id string) error {
	s.logger.Printf("INFO: Starting DeleteItem operation for item ID: %s in collection: %s", id, s.collectionName)

	// Check if item exists first
	doc, err := s.client.Collection(s.collectionName).Doc(id).Get(ctx)
	if err != nil || !doc.Exists() {
		s.logger.Printf("INFO: Item not found for deletion (ID: %s)", id)
		return errors.ErrNotFound
	}

	_, err = s.client.Collection(s.collectionName).Doc(id).Delete(ctx)
	if err != nil {
		s.logger.Printf("ERROR: Failed to delete item (ID: %s): %v", id, err)
		return err
	}

	s.logger.Printf("INFO: Successfully completed DeleteItem operation for item ID: %s", id)
	return nil
}

// ListItems returns all items in the store, optionally filtered by type and/or tags.
func (s *FirestoreStore) ListItems(ctx context.Context, typeFilter string, tagFilters []string) ([]*models.Item, error) {
	s.logger.Printf("INFO: Starting ListItems operation in collection: %s - typeFilter: %q, tagFilters: %v", s.collectionName, typeFilter, tagFilters)

	query := s.client.Collection(s.collectionName).Query

	// Apply type filter
	if typeFilter != "" {
		query = query.Where("type", "==", typeFilter)
		s.logger.Printf("INFO: Applied type filter: %s", typeFilter)
	}

	// Apply tag filters (all tags must be present - intersection)
	for _, tag := range tagFilters {
		query = query.Where("tags", "array-contains", tag)
		s.logger.Printf("INFO: Applied tag filter: %s", tag)
	}

	// Note: Firestore only supports one array-contains per query.
	// If multiple tag filters are needed, we'll need to handle it differently.
	// For now, if more than one tag filter is provided, we'll filter in memory.

	iter := query.Documents(ctx)
	defer iter.Stop()

	var items []*models.Item
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			s.logger.Printf("ERROR: Failed to iterate documents: %v", err)
			return nil, err
		}

		var item models.Item
		if err := doc.DataTo(&item); err != nil {
			s.logger.Printf("ERROR: Failed to unmarshal item (ID: %s): %v", doc.Ref.ID, err)
			continue
		}

		// If we have multiple tag filters, filter in memory (Firestore limitation)
		if len(tagFilters) > 1 {
			if !containsAllTags(&item, tagFilters) {
				continue
			}
		}

		items = append(items, &item)
	}

	s.logger.Printf("INFO: Successfully completed ListItems operation - retrieved %d items", len(items))
	return items, nil
}

// containsAllTags checks if an item contains all the specified tags.
func containsAllTags(item *models.Item, tags []string) bool {
	tagSet := make(map[string]bool)
	for _, tag := range item.Tags {
		tagSet[tag] = true
	}

	for _, requiredTag := range tags {
		if !tagSet[requiredTag] {
			return false
		}
	}

	return true
}
