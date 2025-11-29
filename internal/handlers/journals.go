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
		h.handleListItemsForType(w, r, models.ItemTypeJournal)
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

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", fmt.Sprintf("/items/%s", item.ID))
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(item)
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

	// Combine People, Emotions, and Topics into tags
	tags := []string{"journal"}

	// Add people as tags
	for _, person := range entry.People {
		if trimmed := strings.TrimSpace(person); trimmed != "" {
			tags = append(tags, trimmed)
		}
	}

	// Add emotions as tags
	for _, emotion := range entry.Emotions {
		if trimmed := strings.TrimSpace(emotion); trimmed != "" {
			tags = append(tags, trimmed)
		}
	}

	// Add topics as tags
	for _, topic := range entry.Topics {
		if trimmed := strings.TrimSpace(topic); trimmed != "" {
			tags = append(tags, trimmed)
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
