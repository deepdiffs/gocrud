package models

// QuantityData represents a numeric value with units for workout measurements.
type QuantityData struct {
	Quantity float64 `json:"qty"`
	Units    string  `json:"units"`
}

// WorkoutData captures a single workout entry coming from the client.
type WorkoutData struct {
	WorkoutID       string                 `json:"id"`
	UserID          string                 `json:"user_id"`
	WorkoutType     string                 `json:"name"`
	StartTime       string                 `json:"start"`
	EndTime         string                 `json:"end"`
	DurationMinutes float64                `json:"duration"`
	Temperature     QuantityData           `json:"temperature,omitempty"`
	ActiveEnergy    QuantityData           `json:"activeEnergyBurned"`
	Humidity        QuantityData           `json:"humidity,omitempty"`
	Intensity       QuantityData           `json:"intensity"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	Data            map[string]interface{} `json:"data,omitempty"`
}

// WorkoutRequest wraps workout submissions under a data envelope.
type WorkoutRequest struct {
	Data struct {
		Workouts []WorkoutData `json:"workouts"`
	} `json:"data"`
}
