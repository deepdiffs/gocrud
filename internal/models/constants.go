package models

// Item type constants for domain-specific documents.
const (
	ItemTypeWorkout    = "Workout"
	ItemTypeHealthData = "HealthData"
)

// Well-known workout types. These mirror common HealthAutoExport workout names.
const (
	WorkoutTypeRunning             = "running"
	WorkoutTypeOutdoorWalk         = "outdoor_walk"
	WorkoutTypeTraditionalStrength = "traditional_strength_training"
	WorkoutTypeCycling             = "cycling"
	WorkoutTypeMindAndBody         = "mind_and_body"
	WorkoutTypeFunctionalStrength  = "functional_strength_training"
	WorkoutTypeUnknown             = "unknown_workout"
)

// Health metric names we care about. These map directly to HealthAutoExport metric names.
const (
	HealthMetricSleepAnalysis          = "sleep_analysis"
	HealthMetricHeartRateVariability   = "heart_rate_variability"
	HealthMetricVO2Max                 = "vo2_max"
	HealthMetricExerciseTime           = "apple_exercise_time"
	HealthMetricTimeInDaylight         = "time_in_daylight"
	HealthMetricWalkingRunningDistance = "walking_running_distance"
)
