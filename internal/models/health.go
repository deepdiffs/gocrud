package models

import "encoding/json"

// HealthRequest mirrors the HealthAutoExport payload for health metrics.
type HealthRequest struct {
	Data struct {
		Metrics []HealthMetricPayload `json:"metrics"`
	} `json:"data"`
}

// HealthMetricPayload keeps the metric data raw so handlers can aggregate or
// transform without losing unknown fields.
type HealthMetricPayload struct {
	Name  string            `json:"name"`
	Units string            `json:"units,omitempty"`
	Data  []json.RawMessage `json:"data"`
}

// HealthDataPoint is a typed view for metrics that are simple (qty + date).
type HealthDataPoint struct {
	Qty    float64 `json:"qty"`
	Date   string  `json:"date"`
	Source string  `json:"source,omitempty"`
}

// SleepSample matches the sleep_analysis entries from HealthAutoExport.
type SleepSample struct {
	SleepStart string  `json:"sleepStart"`
	SleepEnd   string  `json:"sleepEnd"`
	Rem        float64 `json:"rem,omitempty"`
	Awake      float64 `json:"awake,omitempty"`
	Asleep     float64 `json:"asleep,omitempty"`
	InBedStart string  `json:"inBedStart"`
	InBedEnd   string  `json:"inBedEnd"`
	TotalSleep float64 `json:"totalSleep,omitempty"`
	Core       float64 `json:"core,omitempty"`
	Deep       float64 `json:"deep,omitempty"`
	InBed      float64 `json:"inBed,omitempty"`
	Date       string  `json:"date"`
	Source     string  `json:"source,omitempty"`
}

// HealthMetricSummary is the stored representation for aggregated metrics.
type HealthMetricSummary struct {
	Name    string   `json:"name"`
	Units   string   `json:"units,omitempty"`
	Count   int      `json:"count"`
	Total   float64  `json:"total"`
	Average float64  `json:"average"`
	Min     float64  `json:"min"`
	Max     float64  `json:"max"`
	Start   string   `json:"start,omitempty"`
	End     string   `json:"end,omitempty"`
	Sources []string `json:"sources,omitempty"`
}

// SleepSummary captures nightly sleep in a single document.
type SleepSummary struct {
	Date          string  `json:"date"`
	SleepStart    string  `json:"sleepStart"`
	SleepEnd      string  `json:"sleepEnd"`
	InBedStart    string  `json:"inBedStart,omitempty"`
	InBedEnd      string  `json:"inBedEnd,omitempty"`
	DurationHours float64 `json:"durationHours,omitempty"`
	TotalSleep    float64 `json:"totalSleepHours,omitempty"`
	RemHours      float64 `json:"remHours,omitempty"`
	CoreHours     float64 `json:"coreHours,omitempty"`
	DeepHours     float64 `json:"deepHours,omitempty"`
	AwakeHours    float64 `json:"awakeHours,omitempty"`
	InBedHours    float64 `json:"inBedHours,omitempty"`
	Quality       string  `json:"quality,omitempty"`
	Source        string  `json:"source,omitempty"`
}
