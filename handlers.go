package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Handler handles HTTP requests for items.
type Handler struct {
	store  *RedisStore
	logger *log.Logger
}

// NewHandler creates a Handler with dependencies.
func NewHandler(store *RedisStore, logger *log.Logger) *Handler {
	return &Handler{store: store, logger: logger}
}

// itemsHandler routes requests without ID: GET for list, POST for create.
func (h *Handler) itemsHandler(w http.ResponseWriter, r *http.Request) {
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

// itemHandler routes requests with ID: GET, PUT, DELETE.
func (h *Handler) itemHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/items/")
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
	var req CreateItemRequest
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
	if strings.TrimSpace(req.Type) == "" || len(req.Data) == 0 {
		http.Error(w, "type and data are required", http.StatusBadRequest)
		return
	}
	// validate JSON data
	var js interface{}
	if err := json.Unmarshal(req.Data, &js); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON data: %v", err), http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	item := &Item{
		ID:           uuid.NewString(),
		Type:         req.Type,
		Tags:         req.Tags,
		Data:         req.Data,
		CreatedAt:    now,
		LastModified: now,
	}

	if err := h.store.SaveItem(r.Context(), item); err != nil {
		h.logger.Printf("error saving item: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", fmt.Sprintf("/items/%s", item.ID))
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(item)
}

// handleGetItem processes GET /items/{id}.
func (h *Handler) handleGetItem(w http.ResponseWriter, r *http.Request, id string) {
	item, err := h.store.GetItem(r.Context(), id)
	if err != nil {
		if err == ErrNotFound {
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
	var req UpdateItemRequest
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
	if strings.TrimSpace(req.Type) == "" || len(req.Data) == 0 {
		http.Error(w, "type and data are required", http.StatusBadRequest)
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
		if err == ErrNotFound {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		} else {
			h.logger.Printf("error fetching item for update: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}

	item.Type = req.Type
	item.Tags = req.Tags
	item.Data = req.Data
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
		if err == ErrNotFound {
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
	items, err := h.store.ListItems(r.Context(), typeFilter)
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
