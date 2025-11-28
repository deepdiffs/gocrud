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

// WorkoutsHandler routes /workouts requests, reusing list support and custom create handling.
func (h *Handler) WorkoutsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handleCreateWorkouts(w, r)
	case http.MethodGet:
		h.handleListItemsForType(w, r, models.ItemTypeWorkout)
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
// It extracts only essential fields to create a simplified WorkoutSummary, avoiding
// Firestore document size limits by not storing the entire raw payload.
func workoutToItemRequest(workoutRaw json.RawMessage, meta models.WorkoutMetadata) (models.CreateItemRequest, error) {
	if len(workoutRaw) == 0 {
		return models.CreateItemRequest{}, fmt.Errorf("marshal workout: empty payload")
	}

	// Parse the workout data to extract essential fields
	var workoutData models.WorkoutData
	if err := json.Unmarshal(workoutRaw, &workoutData); err != nil {
		return models.CreateItemRequest{}, fmt.Errorf("marshal workout: failed to parse workout data: %v", err)
	}

	start, _ := parseFlexibleTime(workoutData.StartTime)
	if start.IsZero() {
		start, _ = parseFlexibleTime(workoutData.EndTime)
	}
	if start.IsZero() {
		return models.CreateItemRequest{}, fmt.Errorf("marshal workout: start time is required")
	}

	// Create a simplified summary with only essential fields
	summary := models.WorkoutSummary{
		WorkoutID:       workoutData.WorkoutID,
		WorkoutType:     workoutData.WorkoutType,
		StartTime:       workoutData.StartTime,
		EndTime:         workoutData.EndTime,
		DurationMinutes: workoutData.DurationMinutes,
		Calories:        workoutData.ActiveEnergy,
		Intensity:       workoutData.Intensity,
	}

	normalizedName := normalizeWorkoutName(workoutData.WorkoutType)
	if normalizedName == "" {
		normalizedName = models.WorkoutTypeUnknown
	}
	summary.WorkoutType = normalizedName

	// Marshal the summary to JSON for storage
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return models.CreateItemRequest{}, fmt.Errorf("marshal workout: failed to marshal summary: %v", err)
	}

	tags := []string{"workout"}

	dedupID := meta.WorkoutID
	if dedupID == "" {
		dedupID = deterministicID(models.ItemTypeWorkout, normalizedName, start.UTC().Format(time.RFC3339Nano))
	}

	return models.CreateItemRequest{
		ID:        dedupID,
		Type:      models.ItemTypeWorkout,
		Name:      normalizedName,
		Timestamp: start.UTC(),
		Tags:      tags,
		Data:      summaryJSON,
	}, nil
}

// normalizeWorkoutName lowercases and snake_cases workout names, defaulting to a known constant.
func normalizeWorkoutName(name string) string {
	trimmed := strings.TrimSpace(strings.ToLower(name))
	if trimmed == "" {
		return models.WorkoutTypeUnknown
	}

	snake := strings.ReplaceAll(trimmed, " ", "_")
	switch snake {
	case "outdoor_walk":
		return models.WorkoutTypeOutdoorWalk
	case "running":
		return models.WorkoutTypeRunning
	case "traditional_strength_training":
		return models.WorkoutTypeTraditionalStrength
	case "functional_strength_training":
		return models.WorkoutTypeFunctionalStrength
	case "cycling":
		return models.WorkoutTypeCycling
	case "mind_and_body":
		return models.WorkoutTypeMindAndBody
	default:
		return snake
	}
}
