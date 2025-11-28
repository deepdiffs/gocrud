package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"gocrud/internal/models"
)

// HealthHandler routes /health requests with custom create logic and shared list support.
func (h *Handler) HealthHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handleCreateHealth(w, r)
	case http.MethodGet:
		h.handleListItems(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// handleCreateHealth ingests HealthAutoExport payloads and writes normalized entries via /items.
func (h *Handler) handleCreateHealth(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	r.Body.Close()

	var req models.HealthRequest
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request payload: %v", err), http.StatusBadRequest)
		return
	}
	if err := ensureSingleJSON(dec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Data.Metrics) == 0 {
		http.Error(w, "data.metrics is required", http.StatusBadRequest)
		return
	}

	type errorEntry struct {
		Index int    `json:"index"`
		Error string `json:"error"`
	}

	created := 0
	var errs []errorEntry

	for i, metric := range req.Data.Metrics {
		entries, err := healthMetricToItems(metric)
		if err != nil {
			errs = append(errs, errorEntry{Index: i, Error: err.Error()})
			continue
		}

		for _, entry := range entries {
			_, status, err := h.buildAndSaveItem(r.Context(), entry)
			if err != nil {
				if status == http.StatusBadRequest {
					errs = append(errs, errorEntry{Index: i, Error: err.Error()})
				} else {
					h.logger.Printf("error saving health metric %s: %v", metric.Name, err)
					errs = append(errs, errorEntry{Index: i, Error: "failed to persist metric"})
				}
				continue
			}
			created++
		}
	}

	status := http.StatusCreated
	if created == 0 {
		status = http.StatusBadRequest
	} else if len(errs) > 0 {
		status = http.StatusMultiStatus
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"created": created,
		"errors":  errs,
	})
}

// healthMetricToItems converts a HealthAutoExport metric into one or more item requests.
func healthMetricToItems(metric models.HealthMetricPayload) ([]models.CreateItemRequest, error) {
	name := strings.TrimSpace(strings.ToLower(metric.Name))
	if name == "" {
		return nil, fmt.Errorf("metric name is required")
	}
	metric.Name = name
	if len(metric.Data) == 0 {
		return nil, fmt.Errorf("metric %s has no data", name)
	}

	if name == models.HealthMetricSleepAnalysis {
		return buildSleepItems(metric)
	}

	item, err := aggregateHealthMetric(metric)
	if err != nil {
		return nil, err
	}
	return []models.CreateItemRequest{item}, nil
}

// aggregateHealthMetric collapses granular metrics (hrv, vo2_max, exercise time, etc) into a single document.
func aggregateHealthMetric(metric models.HealthMetricPayload) (models.CreateItemRequest, error) {
	points := make([]models.HealthDataPoint, 0, len(metric.Data))
	min := math.Inf(1)
	max := math.Inf(-1)
	var sum float64
	var start, end time.Time
	sourceSet := map[string]struct{}{}

	for i, raw := range metric.Data {
		var p models.HealthDataPoint
		if err := json.Unmarshal(raw, &p); err != nil {
			return models.CreateItemRequest{}, fmt.Errorf("metric %s data[%d]: %v", metric.Name, i, err)
		}
		points = append(points, p)
		sum += p.Qty
		if p.Qty < min {
			min = p.Qty
		}
		if p.Qty > max {
			max = p.Qty
		}
		ts, _ := parseFlexibleTime(p.Date)
		if start.IsZero() || (!ts.IsZero() && ts.Before(start)) {
			start = ts
		}
		if end.IsZero() || (!ts.IsZero() && ts.After(end)) {
			end = ts
		}
		if p.Source != "" {
			sourceSet[p.Source] = struct{}{}
		}
	}

	if len(points) == 0 {
		return models.CreateItemRequest{}, fmt.Errorf("metric %s has no parsable data", metric.Name)
	}

	avg := sum / float64(len(points))
	sources := make([]string, 0, len(sourceSet))
	for s := range sourceSet {
		sources = append(sources, s)
	}
	sort.Strings(sources)

	summary := models.HealthMetricSummary{
		Name:    metric.Name,
		Units:   metric.Units,
		Count:   len(points),
		Total:   sum,
		Average: avg,
		Min:     min,
		Max:     max,
		Start:   formatIfNotZero(start),
		End:     formatIfNotZero(end),
		Sources: sources,
	}

	data, err := json.Marshal(summary)
	if err != nil {
		return models.CreateItemRequest{}, fmt.Errorf("marshal metric %s: %v", metric.Name, err)
	}

	tags := []string{"health"}

	ts := end
	if ts.IsZero() {
		ts = start
	}
	if ts.IsZero() {
		return models.CreateItemRequest{}, fmt.Errorf("metric %s missing timestamp", metric.Name)
	}

	return models.CreateItemRequest{
		ID:        deterministicID(models.ItemTypeHealthData, metric.Name, ts.UTC().Format(time.RFC3339Nano)),
		Type:      models.ItemTypeHealthData,
		Name:      metric.Name,
		Timestamp: ts.UTC(),
		Tags:      tags,
		Data:      data,
	}, nil
}

// buildSleepItems generates one item per night of sleep.
func buildSleepItems(metric models.HealthMetricPayload) ([]models.CreateItemRequest, error) {
	var items []models.CreateItemRequest

	for i, raw := range metric.Data {
		var s models.SleepSample
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("metric %s data[%d]: %v", metric.Name, i, err)
		}

		start, _ := parseFlexibleTime(s.SleepStart)
		end, _ := parseFlexibleTime(s.SleepEnd)

		duration := end.Sub(start).Hours()
		if duration < 0 {
			duration = 0
		}

		quality := computeSleepQuality(s)

		summary := models.SleepSummary{
			Date:          s.Date,
			SleepStart:    formatIfNotZero(start),
			SleepEnd:      formatIfNotZero(end),
			InBedStart:    s.InBedStart,
			InBedEnd:      s.InBedEnd,
			DurationHours: duration,
			TotalSleep:    s.TotalSleep,
			RemHours:      s.Rem,
			CoreHours:     s.Core,
			DeepHours:     s.Deep,
			AwakeHours:    s.Awake,
			InBedHours:    s.InBed,
			Quality:       quality,
			Source:        s.Source,
		}

		data, err := json.Marshal(summary)
		if err != nil {
			return nil, fmt.Errorf("marshal sleep summary: %v", err)
		}

		tags := []string{"health"}

		ts := start
		if ts.IsZero() {
			ts = end
		}
		if ts.IsZero() {
			return nil, fmt.Errorf("sleep entry missing timestamp")
		}

		items = append(items, models.CreateItemRequest{
			ID:        deterministicID(models.ItemTypeHealthData, models.HealthMetricSleepAnalysis, ts.UTC().Format(time.RFC3339Nano)),
			Type:      models.ItemTypeHealthData,
			Name:      models.HealthMetricSleepAnalysis,
			Timestamp: ts.UTC(),
			Tags:      tags,
			Data:      data,
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("metric %s has no parsable data", metric.Name)
	}

	return items, nil
}

// computeSleepQuality derives a coarse quality label from durations.
func computeSleepQuality(s models.SleepSample) string {
	if s.TotalSleep <= 0 {
		return "unknown"
	}
	restorative := (s.Rem + s.Deep) / s.TotalSleep
	switch {
	case restorative >= 0.35:
		return "great"
	case restorative >= 0.25:
		return "good"
	case restorative > 0:
		return "ok"
	default:
		return "unknown"
	}
}

// parseFlexibleTime handles the mix of RFC3339 and "2006-01-02 15:04:05 -0700" timestamps.
func parseFlexibleTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05 -0700", "2006-01-02 15:04:05 -0700 MST"}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts, nil
		}
	}
	return time.Time{}, fmt.Errorf("could not parse time %q", value)
}

func formatIfNotZero(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}
