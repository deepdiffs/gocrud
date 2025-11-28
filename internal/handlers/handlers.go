package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"gocrud/internal/errors"
	"gocrud/internal/models"
	"gocrud/internal/store"

	"github.com/google/uuid"
)

// Handler handles HTTP requests for items.
type Handler struct {
	store  store.Store
	logger *log.Logger
}

// NewHandler creates a Handler with dependencies.
func NewHandler(store store.Store, logger *log.Logger) *Handler {
	return &Handler{store: store, logger: logger}
}

// ItemsHandler routes requests without ID: GET for list, POST for create.
func (h *Handler) ItemsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleListItems(w, r)
	case http.MethodPost:
		h.handleCreateItem(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// ItemHandler routes requests with ID: GET, PUT, DELETE.
func (h *Handler) ItemHandler(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path (last segment after the final /)
	parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
	id := parts[len(parts)-1]
	if id == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.handleGetItem(w, r, id)
	case http.MethodPut:
		h.handleUpdateItem(w, r, id)
	case http.MethodDelete:
		h.handleDeleteItem(w, r, id)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// handleCreateItem processes POST /items.
func (h *Handler) handleCreateItem(w http.ResponseWriter, r *http.Request) {
	var req models.CreateItemRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request payload: %v", err), http.StatusBadRequest)
		return
	}
	if err := ensureSingleJSON(dec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	item, status, err := h.buildAndSaveItem(r.Context(), req)
	if err != nil {
		if status == http.StatusBadRequest {
			http.Error(w, err.Error(), status)
			return
		}
		h.logger.Printf("error saving item: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", fmt.Sprintf("%s/%s", r.URL.Path, item.ID))
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(item)
}

// handleGetItem processes GET /items/{id}.
func (h *Handler) handleGetItem(w http.ResponseWriter, r *http.Request, id string) {
	item, err := h.store.GetItem(r.Context(), id)
	if err != nil {
		if err == errors.ErrNotFound {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		} else {
			h.logger.Printf("error getting item: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

// handleUpdateItem processes PUT /items/{id}.
func (h *Handler) handleUpdateItem(w http.ResponseWriter, r *http.Request, id string) {
	var req models.UpdateItemRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request payload: %v", err), http.StatusBadRequest)
		return
	}
	if err := ensureSingleJSON(dec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Type) == "" || strings.TrimSpace(req.Name) == "" || req.Timestamp.IsZero() || len(req.Data) == 0 {
		http.Error(w, "type, name, timestamp, and data are required", http.StatusBadRequest)
		return
	}
	// validate JSON data
	var js interface{}
	if err := json.Unmarshal(req.Data, &js); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON data: %v", err), http.StatusBadRequest)
		return
	}

	item, err := h.store.GetItem(r.Context(), id)
	if err != nil {
		if err == errors.ErrNotFound {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		} else {
			h.logger.Printf("error fetching item for update: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}

	item.Type = req.Type
	item.Name = req.Name
	item.Timestamp = req.Timestamp.UTC()
	item.Tags = req.Tags
	item.Data = js
	item.LastModified = time.Now().UTC()

	if err := h.store.SaveItem(r.Context(), item); err != nil {
		h.logger.Printf("error updating item: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

// handleDeleteItem processes DELETE /items/{id}.
func (h *Handler) handleDeleteItem(w http.ResponseWriter, r *http.Request, id string) {
	err := h.store.DeleteItem(r.Context(), id)
	if err != nil {
		if err == errors.ErrNotFound {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		} else {
			h.logger.Printf("error deleting item: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListItems processes GET /items.
func (h *Handler) handleListItems(w http.ResponseWriter, r *http.Request) {
	typeFilter := r.URL.Query().Get("type")

	// Parse tag filters - support both comma-separated and multiple params
	var tagFilters []string
	if tagParam := r.URL.Query().Get("tags"); tagParam != "" {
		// Handle comma-separated tags: ?tags=tag1,tag2,tag3
		for _, tag := range strings.Split(tagParam, ",") {
			if trimmed := strings.TrimSpace(tag); trimmed != "" {
				tagFilters = append(tagFilters, trimmed)
			}
		}
	}
	// Also handle multiple tag parameters: ?tag=tag1&tag=tag2&tag=tag3
	if tagParams := r.URL.Query()["tag"]; len(tagParams) > 0 {
		for _, tag := range tagParams {
			if trimmed := strings.TrimSpace(tag); trimmed != "" {
				tagFilters = append(tagFilters, trimmed)
			}
		}
	}

	items, err := h.store.ListItems(r.Context(), typeFilter, tagFilters)
	if err != nil {
		h.logger.Printf("error listing items: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// ensureSingleJSON ensures only a single JSON object is in the request body.
func ensureSingleJSON(dec *json.Decoder) error {
	// Check for extra JSON tokens
	if t, err := dec.Token(); err != io.EOF || t != nil {
		return fmt.Errorf("request body must only contain a single JSON object")
	}
	return nil
}

// buildAndSaveItem validates a create request, stamps metadata, and persists it.
func (h *Handler) buildAndSaveItem(ctx context.Context, req models.CreateItemRequest) (*models.Item, int, error) {
	if strings.TrimSpace(req.Type) == "" || strings.TrimSpace(req.Name) == "" || req.Timestamp.IsZero() || len(req.Data) == 0 {
		return nil, http.StatusBadRequest, fmt.Errorf("type, name, timestamp, and data are required")
	}

	// validate JSON data
	var js interface{}
	if err := json.Unmarshal(req.Data, &js); err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("invalid JSON data: %w", err)
	}

	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = uuid.NewString()
	}

	now := time.Now().UTC()
	item := &models.Item{
		ID:           id,
		Type:         req.Type,
		Name:         req.Name,
		Timestamp:    req.Timestamp.UTC(),
		Tags:         req.Tags,
		Data:         js,
		CreatedAt:    now,
		LastModified: now,
	}

	if existing, err := h.store.GetItem(ctx, id); err == nil && existing != nil {
		item.CreatedAt = existing.CreatedAt
	} else if err != nil && err != errors.ErrNotFound {
		return nil, http.StatusInternalServerError, err
	}

	if err := h.store.SaveItem(ctx, item); err != nil {
		return nil, http.StatusInternalServerError, err
	}

	return item, 0, nil
}
