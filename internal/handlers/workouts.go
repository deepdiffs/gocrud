package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gocrud/internal/models"
)

// WorkoutsHandler routes /workouts requests, reusing list support and custom create handling.
func (h *Handler) WorkoutsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handleCreateWorkouts(w, r)
	case http.MethodGet:
		h.handleListItems(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// handleCreateWorkouts processes POST /workouts by translating each workout into an item.
func (h *Handler) handleCreateWorkouts(w http.ResponseWriter, r *http.Request) {
	// Read the entire request body for logging
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	r.Body.Close()

	// Log the entire request payload
	h.logger.Printf("Request payload: %s", string(bodyBytes))

	var req models.WorkoutRequest
	dec := json.NewDecoder(bytes.NewReader(bodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request payload: %v", err), http.StatusBadRequest)
		return
	}
	if err := ensureSingleJSON(dec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Data.Workouts) == 0 {
		http.Error(w, "data.workouts is required", http.StatusBadRequest)
		return
	}

	type errorEntry struct {
		Index int    `json:"index"`
		Error string `json:"error"`
	}

	createdCount := 0
	var errs []errorEntry

	for i, workoutRaw := range req.Data.Workouts {
		if !json.Valid(workoutRaw) {
			errs = append(errs, errorEntry{Index: i, Error: "workout payload is not valid JSON"})
			continue
		}

		var meta models.WorkoutMetadata
		if err := json.Unmarshal(workoutRaw, &meta); err != nil {
			errs = append(errs, errorEntry{Index: i, Error: fmt.Sprintf("failed to parse workout metadata: %v", err)})
			continue
		}

		itemReq, err := workoutToItemRequest(workoutRaw, meta)
		if err != nil {
			errMsg := err.Error()
			errMsg = strings.TrimPrefix(errMsg, "marshal workout: ")
			errs = append(errs, errorEntry{Index: i, Error: errMsg})
			continue
		}

		_, status, err := h.buildAndSaveItem(r.Context(), itemReq)
		if err != nil {
			if status == http.StatusBadRequest {
				errs = append(errs, errorEntry{Index: i, Error: err.Error()})
			} else {
				h.logger.Printf("error saving workout %d: %v", i, err)
				errs = append(errs, errorEntry{Index: i, Error: "failed to persist workout"})
			}
			continue
		}

		createdCount++
	}

	status := http.StatusCreated
	if createdCount == 0 {
		status = http.StatusBadRequest
	} else if len(errs) > 0 {
		status = http.StatusMultiStatus
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"created": createdCount,
		"errors":  errs,
	})
}

// workoutToItemRequest converts a workout payload into a CreateItemRequest for storage.

func workoutToItemRequest(workoutRaw json.RawMessage, meta models.WorkoutMetadata) (models.CreateItemRequest, error) {
	if len(workoutRaw) == 0 {
		return models.CreateItemRequest{}, fmt.Errorf("marshal workout: empty payload")
	}

	itemType := strings.TrimSpace(meta.WorkoutType)
	if itemType == "" {
		itemType = "workout"
	}

	tags := []string{"workout"}
	if meta.WorkoutType != "" {
		tags = append(tags, "type:"+meta.WorkoutType)
	}
	if meta.WorkoutID != "" {
		tags = append(tags, "workoutId:"+meta.WorkoutID)
	}

	return models.CreateItemRequest{
		Type: itemType,
		Tags: tags,
		Data: workoutRaw,
	}, nil
}
