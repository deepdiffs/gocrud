package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gocrud/internal/models"
)

// JournalsHandler routes /journals requests, reusing list support and custom create handling.
func (h *Handler) JournalsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handleCreateJournal(w, r)
	case http.MethodGet:
		h.handleListJournals(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// handleCreateJournal processes POST /journals by parsing the request payload and saving to Firestore.
func (h *Handler) handleCreateJournal(w http.ResponseWriter, r *http.Request) {

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Log the entire request payload
	h.logger.Printf("Request payload: %s", string(body))

	var req models.JournalRequest
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		h.logger.Printf("failed to parse journal request: %v", err)
		http.Error(w, fmt.Sprintf("invalid request payload: %v", err), http.StatusBadRequest)
		return
	}
	if err := ensureSingleJSON(dec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Entry.Title == "" {
		http.Error(w, "entry.title is required", http.StatusBadRequest)
		return
	}
	if req.Entry.Date == "" {
		http.Error(w, "entry.date is required", http.StatusBadRequest)
		return
	}

	// Convert journal entry to item request
	itemReq, err := journalToItemRequest(req.Entry)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to process journal entry: %v", err), http.StatusBadRequest)
		return
	}

	// Save the item
	item, status, err := h.buildAndSaveItem(r.Context(), itemReq)
	if err != nil {
		if status == http.StatusBadRequest {
			http.Error(w, err.Error(), status)
			return
		}
		h.logger.Printf("error saving journal entry: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	h.logger.Printf("journal entry saved: title=%q, date=%q, id=%q", req.Entry.Title, req.Entry.Date, item.ID)

	// Convert item back to journal entry for response
	journalEntry, err := itemToJournalEntry(item)
	if err != nil {
		h.logger.Printf("error converting item to journal entry: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", fmt.Sprintf("/items/%s", item.ID))
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(journalEntry)
}

// journalToItemRequest converts a journal entry into a CreateItemRequest for storage.
func journalToItemRequest(entry models.JournalEntry) (models.CreateItemRequest, error) {
	// Parse the date to get timestamp
	timestamp, err := parseFlexibleTime(entry.Date)
	if err != nil {
		return models.CreateItemRequest{}, fmt.Errorf("invalid date format: %v", err)
	}
	if timestamp.IsZero() {
		return models.CreateItemRequest{}, fmt.Errorf("date is required")
	}

	// Combine People, Emotions, and Topics into tags with prefixes
	tags := []string{"journal"}

	// Add people as tags with "person:" prefix
	for _, person := range entry.People {
		if trimmed := strings.TrimSpace(person); trimmed != "" {
			tags = append(tags, fmt.Sprintf("person:%s", trimmed))
		}
	}

	// Add emotions as tags with "emotion:" prefix
	for _, emotion := range entry.Emotions {
		if trimmed := strings.TrimSpace(emotion); trimmed != "" {
			tags = append(tags, fmt.Sprintf("emotion:%s", trimmed))
		}
	}

	// Add topics as tags with "topic:" prefix
	for _, topic := range entry.Topics {
		if trimmed := strings.TrimSpace(topic); trimmed != "" {
			tags = append(tags, fmt.Sprintf("topic:%s", trimmed))
		}
	}

	// Create data payload with Content
	dataPayload := map[string]interface{}{
		"content": entry.Content,
	}
	dataJSON, err := json.Marshal(dataPayload)
	if err != nil {
		return models.CreateItemRequest{}, fmt.Errorf("failed to marshal journal data: %v", err)
	}

	// Generate deterministic ID for idempotency
	// Use type, title, and date to ensure same journal entry gets same ID
	dedupID := deterministicID(
		models.ItemTypeJournal,
		entry.Title,
		timestamp.UTC().Format(time.RFC3339Nano),
	)

	return models.CreateItemRequest{
		ID:        dedupID,
		Type:      models.ItemTypeJournal,
		Name:      entry.Title,
		Timestamp: timestamp.UTC(),
		Tags:      tags,
		Data:      dataJSON,
	}, nil
}

// handleListJournals processes GET /journals and returns JournalEntry objects.
func (h *Handler) handleListJournals(w http.ResponseWriter, r *http.Request) {
	// Use the existing list items logic but filter by journal type
	q := r.URL.Query()
	q.Set("type", models.ItemTypeJournal)
	clone := *r
	urlCopy := *r.URL
	urlCopy.RawQuery = q.Encode()
	clone.URL = &urlCopy

	// Get items from store
	typeFilter := models.ItemTypeJournal

	// Parse tag filters - support both comma-separated and multiple params
	var tagFilters []string
	if tagParam := r.URL.Query().Get("tags"); tagParam != "" {
		for _, tag := range strings.Split(tagParam, ",") {
			if trimmed := strings.TrimSpace(tag); trimmed != "" {
				tagFilters = append(tagFilters, trimmed)
			}
		}
	}
	if tagParams := r.URL.Query()["tag"]; len(tagParams) > 0 {
		for _, tag := range tagParams {
			if trimmed := strings.TrimSpace(tag); trimmed != "" {
				tagFilters = append(tagFilters, trimmed)
			}
		}
	}

	var startTimePtr, endTimePtr *time.Time
	if startStr := strings.TrimSpace(r.URL.Query().Get("startTime")); startStr != "" {
		start, err := parseFlexibleTime(startStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid startTime: %v", err), http.StatusBadRequest)
			return
		}
		start = start.UTC()
		startTimePtr = &start
	}
	if endStr := strings.TrimSpace(r.URL.Query().Get("endTime")); endStr != "" {
		end, err := parseFlexibleTime(endStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid endTime: %v", err), http.StatusBadRequest)
			return
		}
		end = end.UTC()
		endTimePtr = &end
	}
	if startTimePtr != nil && endTimePtr != nil && endTimePtr.Before(*startTimePtr) {
		http.Error(w, "endTime must be after startTime", http.StatusBadRequest)
		return
	}

	items, err := h.store.ListItems(r.Context(), typeFilter, tagFilters, startTimePtr, endTimePtr)
	if err != nil {
		h.logger.Printf("error listing journal entries: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	// Convert items to journal entries, filtering out old data without prefixed tags
	journalEntries := make([]models.JournalEntry, 0, len(items))
	for _, item := range items {
		journalEntry, err := itemToJournalEntry(item)
		if err != nil {
			// Skip items that can't be converted (old data without prefixed tags)
			h.logger.Printf("skipping item %s: %v", item.ID, err)
			continue
		}
		journalEntries = append(journalEntries, journalEntry)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(journalEntries)
}

// itemToJournalEntry converts an Item back to a JournalEntry, parsing prefixed tags.
// Returns an error if the item doesn't have the expected structure (old data).
func itemToJournalEntry(item *models.Item) (models.JournalEntry, error) {
	// Extract content from data
	var content string
	if dataMap, ok := item.Data.(map[string]interface{}); ok {
		if contentVal, ok := dataMap["content"].(string); ok {
			content = contentVal
		}
	} else if dataBytes, ok := item.Data.([]byte); ok {
		// Try to unmarshal if Data is json.RawMessage or []byte
		var dataPayload map[string]interface{}
		if err := json.Unmarshal(dataBytes, &dataPayload); err == nil {
			if contentVal, ok := dataPayload["content"].(string); ok {
				content = contentVal
			}
		}
	} else if dataStr, ok := item.Data.(string); ok {
		// Try to unmarshal if Data is a JSON string
		var dataPayload map[string]interface{}
		if err := json.Unmarshal([]byte(dataStr), &dataPayload); err == nil {
			if contentVal, ok := dataPayload["content"].(string); ok {
				content = contentVal
			}
		}
	}

	// Image fields are top-level fields in the document, read directly from Item
	var imageData string
	var imageFormat string
	var imageSizeBytes int64
	if item.ImageData != nil {
		imageData = *item.ImageData
	}
	if item.ImageFormat != nil {
		imageFormat = *item.ImageFormat
	}
	if item.ImageSizeBytes != nil {
		imageSizeBytes = *item.ImageSizeBytes
	}

	// Parse tags into People, Emotions, and Topics
	var people []string
	var emotions []string
	var topics []string
	hasPrefixedTags := false

	for _, tag := range item.Tags {
		// Skip the "journal" tag
		if tag == "journal" {
			continue
		}

		// Check for prefixed tags
		if strings.HasPrefix(tag, "person:") {
			hasPrefixedTags = true
			person := strings.TrimPrefix(tag, "person:")
			if person != "" {
				people = append(people, person)
			}
		} else if strings.HasPrefix(tag, "emotion:") {
			hasPrefixedTags = true
			emotion := strings.TrimPrefix(tag, "emotion:")
			if emotion != "" {
				emotions = append(emotions, emotion)
			}
		} else if strings.HasPrefix(tag, "topic:") {
			hasPrefixedTags = true
			topic := strings.TrimPrefix(tag, "topic:")
			if topic != "" {
				topics = append(topics, topic)
			}
		}
		// Ignore tags without prefixes (old data)
	}

	// If no prefixed tags found but there are tags beyond "journal", this is old data - return error to skip it
	// Allow items with only "journal" tag (empty People, Emotions, Topics)
	nonJournalTags := 0
	for _, tag := range item.Tags {
		if tag != "journal" {
			nonJournalTags++
		}
	}
	if !hasPrefixedTags && nonJournalTags > 0 {
		return models.JournalEntry{}, fmt.Errorf("item %s has no prefixed tags (old data format)", item.ID)
	}

	// Format date from timestamp
	date := item.Timestamp.Format(time.RFC3339)

	return models.JournalEntry{
		Title:          item.Name,
		Date:           date,
		Emotions:       emotions,
		People:         people,
		Topics:         topics,
		Content:        content,
		ImageData:      imageData,
		ImageFormat:    imageFormat,
		ImageSizeBytes: imageSizeBytes,
	}, nil
}
