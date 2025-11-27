package models

import "encoding/json"

// QuantityData represents a numeric value with units for workout measurements.
type QuantityData struct {
	Quantity float64 `json:"qty"`
	Units    string  `json:"units"`
}

// WorkoutRequest wraps workout submissions under a data envelope. Workouts are
// kept as raw JSON so we can store the full payload without losing unknown
// fields such as heartRateData or route.
type WorkoutRequest struct {
	Data struct {
		Workouts []json.RawMessage `json:"workouts"`
	} `json:"data"`
}

// WorkoutMetadata captures the small set of fields we need to tag the stored
// items while leaving the full payload untouched.
type WorkoutMetadata struct {
	WorkoutID   string `json:"id"`
	WorkoutType string `json:"name"`
}

// WorkoutData captures a single workout entry in structured form. It remains
// available for callers that want a typed view, but the handler stores raw JSON
// from the request to preserve all fields.
type WorkoutData struct {
	WorkoutID       string                 `json:"id"`
	WorkoutType     string                 `json:"name"`
	StartTime       string                 `json:"start"`
	EndTime         string                 `json:"end"`
	DurationMinutes float64                `json:"duration"`
	ActiveEnergy    QuantityData           `json:"activeEnergyBurned"`
	Intensity       QuantityData           `json:"intensity"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	Data            map[string]interface{} `json:"data,omitempty"`
}

// WorkoutSummary represents a simplified workout document with only essential fields.
// This is stored in Firestore instead of the full raw payload to avoid document size limits.
type WorkoutSummary struct {
	WorkoutID       string       `json:"id"`
	WorkoutType     string       `json:"name"`
	StartTime       string       `json:"start,omitempty"`
	EndTime         string       `json:"end,omitempty"`
	DurationMinutes float64      `json:"duration,omitempty"`
	Calories        QuantityData `json:"activeEnergyBurned,omitempty"`
	Intensity       QuantityData `json:"intensity,omitempty"`
}
