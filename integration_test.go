// integration_test.go contains an end-to-end integration test suite for the CRUD API.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gocrud/internal/handlers"
	"gocrud/internal/middleware"
	"gocrud/internal/models"
	"gocrud/internal/store"
)

var (
	testCtx    = context.Background()
	testAPIKey = "test-api-key-123"
)

// makeAuthenticatedRequest creates an HTTP request with the X-API-Key header set.
func makeAuthenticatedRequest(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", testAPIKey)
	return req, nil
}

// setupTestServer creates a test HTTP server with the specified storage backend.
// Returns the server URL and a cleanup function.
func setupTestServer(t *testing.T, backend string) (string, func()) {
	// Save original STORAGE_BACKEND
	originalBackend := os.Getenv("STORAGE_BACKEND")

	// Set the backend for this test
	os.Setenv("STORAGE_BACKEND", backend)

	logger := newTestLogger()
	storeInstance, err := store.NewStore(testCtx, logger)
	if err != nil {
		t.Fatalf("failed to create store with backend %s: %v", backend, err)
	}
	sharedHealthStore, err := store.NewStoreWithCollection(testCtx, logger, "health")
	if err != nil {
		t.Fatalf("failed to create health store with backend %s: %v", backend, err)
	}

	itemsHandler := handlers.NewHandler(storeInstance, logger)
	workoutsHandler := handlers.NewHandler(sharedHealthStore, logger)
	healthHandler := handlers.NewHandler(sharedHealthStore, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/items", itemsHandler.ItemsHandler)
	mux.HandleFunc("/items/", itemsHandler.ItemHandler)
	mux.HandleFunc("/workouts", workoutsHandler.WorkoutsHandler)
	mux.HandleFunc("/workouts/", workoutsHandler.ItemHandler)
	mux.HandleFunc("/health", healthHandler.HealthHandler)
	mux.HandleFunc("/health/", healthHandler.ItemHandler)

	// Set up authentication for tests
	validKeys := middleware.ParseAPIKeys(testAPIKey)
	authMux := middleware.AuthMiddleware(validKeys, logger)(mux)
	srv := httptest.NewServer(middleware.LoggingMiddleware(logger)(authMux))

	cleanup := func() {
		srv.Close()
		// Restore original STORAGE_BACKEND
		if originalBackend == "" {
			os.Unsetenv("STORAGE_BACKEND")
		} else {
			os.Setenv("STORAGE_BACKEND", originalBackend)
		}
	}

	return srv.URL, cleanup
}

// TestCRUDIntegration exercises Create, Read, Update, List (with and without type filter), and Delete.
// Tests memory storage backend.
func TestCRUDIntegration(t *testing.T) {
	backends := []string{"memory"}

	for _, backend := range backends {
		t.Run(backend, func(t *testing.T) {
			testServerURL, cleanup := setupTestServer(t, backend)
			defer cleanup()

			// load create payloads
			createFiles := []string{
				"create_item_request.json",
				"create_user_request.json",
				"create_task_request.json",
			}
			type createCase struct {
				file string
				req  models.CreateItemRequest
				itm  models.Item
			}
			var cases []createCase
			for _, fn := range createFiles {
				path := filepath.Join("mockdata", fn)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("reading %s: %v", path, err)
				}
				var req models.CreateItemRequest
				if err := json.Unmarshal(data, &req); err != nil {
					t.Fatalf("unmarshal %s: %v", fn, err)
				}
				cases = append(cases, createCase{file: fn, req: req})
			}

			client := &http.Client{}
			// CREATE
			for i := range cases {
				path := filepath.Join("mockdata", cases[i].file)
				data, _ := os.ReadFile(path)
				req, err := makeAuthenticatedRequest(http.MethodPost, testServerURL+"/items", bytes.NewReader(data))
				if err != nil {
					t.Fatalf("creating POST request (%s): %v", cases[i].file, err)
				}
				req.Header.Set("Content-Type", "application/json")
				resp, err := client.Do(req)
				if err != nil {
					t.Fatalf("POST /items (%s) error: %v", cases[i].file, err)
				}
				if resp.StatusCode != http.StatusCreated {
					body, _ := io.ReadAll(resp.Body)
					t.Fatalf("POST /items (%s) status %d, body: %s", cases[i].file, resp.StatusCode, body)
				}
				var out models.Item
				if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
					t.Fatalf("decode created (%s): %v", cases[i].file, err)
				}
				resp.Body.Close()
				if out.ID == "" {
					t.Fatalf("empty ID for %s", cases[i].file)
				}
				if out.Name != cases[i].req.Name {
					t.Fatalf("name mismatch: want %s got %s", cases[i].req.Name, out.Name)
				}
				if !out.Timestamp.Equal(cases[i].req.Timestamp) {
					t.Fatalf("timestamp mismatch: want %s got %s", cases[i].req.Timestamp, out.Timestamp)
				}
				cases[i].itm = out
			}

			// READ each
			for _, c := range cases {
				req, err := makeAuthenticatedRequest(http.MethodGet, testServerURL+"/items/"+c.itm.ID, nil)
				if err != nil {
					t.Fatalf("creating GET request: %v", err)
				}
				resp, err := client.Do(req)
				if err != nil {
					t.Fatalf("GET /items/%s error: %v", c.itm.ID, err)
				}
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("GET /items/%s status %d", c.itm.ID, resp.StatusCode)
				}
				var got models.Item
				if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
					t.Fatalf("decode GET %s: %v", c.itm.ID, err)
				}
				resp.Body.Close()
				if got.ID != c.itm.ID {
					t.Errorf("expected ID %s, got %s", c.itm.ID, got.ID)
				}
			}

			// UPDATE first item
			updatePath := filepath.Join("mockdata", "update_item_request.json")
			updData, err := os.ReadFile(updatePath)
			if err != nil {
				t.Fatalf("reading update payload: %v", err)
			}
			var updReq models.UpdateItemRequest
			if err := json.Unmarshal(updData, &updReq); err != nil {
				t.Fatalf("unmarshal update payload: %v", err)
			}
			targetID := cases[0].itm.ID

			req, err := makeAuthenticatedRequest(http.MethodPut, testServerURL+"/items/"+targetID, bytes.NewReader(updData))
			if err != nil {
				t.Fatalf("creating PUT request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("PUT /items/%s error: %v", targetID, err)
			}
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("PUT /items/%s status %d, body: %s", targetID, resp.StatusCode, body)
			}
			var updated models.Item
			if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
				t.Fatalf("decode updated item: %v", err)
			}
			resp.Body.Close()
			if updated.ID != targetID {
				t.Errorf("update ID mismatch: want %s, got %s", targetID, updated.ID)
			}
			if updated.Type != updReq.Type {
				t.Errorf("update Type mismatch: want %s, got %s", updReq.Type, updated.Type)
			}
			if updated.Name != updReq.Name {
				t.Errorf("update Name mismatch: want %s, got %s", updReq.Name, updated.Name)
			}
			if !updated.Timestamp.Equal(updReq.Timestamp) {
				t.Errorf("update Timestamp mismatch: want %s, got %s", updReq.Timestamp, updated.Timestamp)
			}
			updatedData, ok := updated.Data.(map[string]interface{})
			if !ok {
				t.Fatalf("unexpected updated data type %T", updated.Data)
			}
			if price, ok := updatedData["price"].(float64); !ok || price != 899.99 {
				t.Errorf("updated data not applied: %#v", updated.Data)
			}

			// VERIFY update via GET
			req, err = makeAuthenticatedRequest(http.MethodGet, testServerURL+"/items/"+targetID, nil)
			if err != nil {
				t.Fatalf("creating GET request after update: %v", err)
			}
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("GET after update error: %v", err)
			}
			var after models.Item
			if err := json.NewDecoder(resp.Body).Decode(&after); err != nil {
				t.Fatalf("decode after update: %v", err)
			}
			resp.Body.Close()
			if after.LastModified.Equal(after.CreatedAt) {
				t.Errorf("LastModified not updated: created %s, lastModified %s", after.CreatedAt, after.LastModified)
			}

			// LIST all
			req, err = makeAuthenticatedRequest(http.MethodGet, testServerURL+"/items", nil)
			if err != nil {
				t.Fatalf("creating GET request for list: %v", err)
			}
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("GET /items error: %v", err)
			}
			var list []models.Item
			if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
				t.Fatalf("decode list: %v", err)
			}
			resp.Body.Close()
			if len(list) != len(cases) {
				t.Errorf("expected %d items, got %d", len(cases), len(list))
			}

			// LIST by type filter
			req, err = makeAuthenticatedRequest(http.MethodGet, testServerURL+"/items?type="+updReq.Type, nil)
			if err != nil {
				t.Fatalf("creating GET request for filtered list: %v", err)
			}
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("GET /items?type=%s error: %v", updReq.Type, err)
			}
			var filtered []models.Item
			if err := json.NewDecoder(resp.Body).Decode(&filtered); err != nil {
				t.Fatalf("decode filtered list: %v", err)
			}
			resp.Body.Close()
			if len(filtered) != 1 {
				t.Errorf("expected 1 filtered item, got %d", len(filtered))
			}
			if filtered[0].ID != targetID {
				t.Errorf("filtered ID mismatch: want %s, got %s", targetID, filtered[0].ID)
			}

			// WORKOUTS: create and list
			workoutPayload := []byte(`{"data":{"workouts":[{"id":"w1","user_id":"user-123","name":"running","start":"2024-01-01T00:00:00Z","end":"2024-01-01T00:30:00Z","duration":30,"activeEnergyBurned":{"qty":210.5,"units":"kcal"},"intensity":{"qty":7,"units":"rpe"}}]}}`)
			req, err = makeAuthenticatedRequest(http.MethodPost, testServerURL+"/workouts", bytes.NewReader(workoutPayload))
			if err != nil {
				t.Fatalf("creating POST /workouts request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("POST /workouts error: %v", err)
			}
			if resp.StatusCode != http.StatusCreated {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("POST /workouts status %d, body: %s", resp.StatusCode, body)
			}
			var workoutResp map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&workoutResp); err != nil {
				t.Fatalf("decode workouts response: %v", err)
			}
			resp.Body.Close()

			createdVal, ok := workoutResp["created"].(float64)
			if !ok || int(createdVal) != 1 {
				t.Fatalf("unexpected created count in workouts response: %v", workoutResp)
			}

			req, err = makeAuthenticatedRequest(http.MethodGet, testServerURL+"/workouts", nil)
			if err != nil {
				t.Fatalf("creating GET request for workouts list: %v", err)
			}
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("GET /workouts error: %v", err)
			}
			var workouts []models.Item
			if err := json.NewDecoder(resp.Body).Decode(&workouts); err != nil {
				t.Fatalf("decode workouts list: %v", err)
			}
			resp.Body.Close()
			if len(workouts) != 1 {
				t.Fatalf("expected 1 workout item, got %d", len(workouts))
			}
			if workouts[0].Type != models.ItemTypeWorkout {
				t.Errorf("unexpected workout item type: %s", workouts[0].Type)
			}
			if workouts[0].Name != models.WorkoutTypeRunning {
				t.Errorf("unexpected workout name: %s", workouts[0].Name)
			}
			workoutID := workouts[0].ID

			// Re-post the same workout payload to verify idempotent creates
			req, err = makeAuthenticatedRequest(http.MethodPost, testServerURL+"/workouts", bytes.NewReader(workoutPayload))
			if err != nil {
				t.Fatalf("creating POST /workouts (idempotent) request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("POST /workouts idempotent error: %v", err)
			}
			if resp.StatusCode != http.StatusCreated {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("POST /workouts idempotent status %d, body: %s", resp.StatusCode, body)
			}
			resp.Body.Close()

			req, err = makeAuthenticatedRequest(http.MethodGet, testServerURL+"/workouts", nil)
			if err != nil {
				t.Fatalf("creating GET request for workouts list after idempotent POST: %v", err)
			}
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("GET /workouts after idempotent POST error: %v", err)
			}
			var workoutsAgain []models.Item
			if err := json.NewDecoder(resp.Body).Decode(&workoutsAgain); err != nil {
				t.Fatalf("decode workouts list after idempotent POST: %v", err)
			}
			resp.Body.Close()
			if len(workoutsAgain) != 1 {
				t.Fatalf("expected 1 workout item after idempotent POST, got %d", len(workoutsAgain))
			}
			if workoutsAgain[0].ID != workoutID {
				t.Fatalf("workout ID changed after idempotent POST: before %s after %s", workoutID, workoutsAgain[0].ID)
			}

			// WORKOUTS: time window queries
			req, err = makeAuthenticatedRequest(http.MethodGet, testServerURL+"/workouts?startTime=2024-01-01T00:00:00Z&endTime=2024-01-01T00:10:00Z", nil)
			if err != nil {
				t.Fatalf("creating GET request for workouts time range: %v", err)
			}
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("GET /workouts time range error: %v", err)
			}
			var workoutsWindow []models.Item
			if err := json.NewDecoder(resp.Body).Decode(&workoutsWindow); err != nil {
				t.Fatalf("decode workouts time range: %v", err)
			}
			resp.Body.Close()
			if len(workoutsWindow) != 1 {
				t.Fatalf("expected 1 workout in time window, got %d", len(workoutsWindow))
			}
			if !workoutsWindow[0].Timestamp.Equal(workoutsAgain[0].Timestamp) {
				t.Fatalf("workout timestamp mismatch in window: want %s got %s", workoutsAgain[0].Timestamp, workoutsWindow[0].Timestamp)
			}

			req, err = makeAuthenticatedRequest(http.MethodGet, testServerURL+"/workouts?startTime=2024-01-01T00:01:00Z", nil)
			if err != nil {
				t.Fatalf("creating GET request for workouts filtered window: %v", err)
			}
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("GET /workouts filtered window error: %v", err)
			}
			var workoutsFiltered []models.Item
			if err := json.NewDecoder(resp.Body).Decode(&workoutsFiltered); err != nil {
				t.Fatalf("decode workouts filtered window: %v", err)
			}
			resp.Body.Close()
			if len(workoutsFiltered) != 0 {
				t.Fatalf("expected 0 workouts after startTime filter, got %d", len(workoutsFiltered))
			}

			req, err = makeAuthenticatedRequest(http.MethodGet, testServerURL+"/workouts?startTime=1704067200&endTime=1704069000", nil)
			if err != nil {
				t.Fatalf("creating GET request for workouts epoch window: %v", err)
			}
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("GET /workouts epoch window error: %v", err)
			}
			var workoutsEpoch []models.Item
			if err := json.NewDecoder(resp.Body).Decode(&workoutsEpoch); err != nil {
				t.Fatalf("decode workouts epoch window: %v", err)
			}
			resp.Body.Close()
			if len(workoutsEpoch) != 1 {
				t.Fatalf("expected 1 workout in epoch window, got %d", len(workoutsEpoch))
			}

			// HEALTH: aggregate and list
			healthPayload := []byte(`{"data":{"metrics":[{"name":"heart_rate_variability","units":"ms","data":[{"qty":50,"date":"2024-02-01T00:00:00Z"},{"qty":100,"date":"2024-02-01T01:00:00Z"}]},{"name":"sleep_analysis","units":"hr","data":[{"sleepStart":"2024-02-01T00:00:00Z","sleepEnd":"2024-02-01T08:00:00Z","rem":2.0,"deep":1.5,"core":4.0,"awake":0.5,"totalSleep":8.0,"inBed":8.5,"inBedStart":"2024-02-01T00:00:00Z","inBedEnd":"2024-02-01T08:30:00Z","date":"2024-02-01","source":"watch"}]}]}}`)
			req, err = makeAuthenticatedRequest(http.MethodPost, testServerURL+"/health", bytes.NewReader(healthPayload))
			if err != nil {
				t.Fatalf("creating POST /health request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("POST /health error: %v", err)
			}
			if resp.StatusCode != http.StatusCreated {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("POST /health status %d, body: %s", resp.StatusCode, body)
			}
			var healthResp map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
				t.Fatalf("decode health response: %v", err)
			}
			resp.Body.Close()
			if created, ok := healthResp["created"].(float64); !ok || int(created) != 2 {
				t.Fatalf("unexpected health created count: %v", healthResp)
			}

			req, err = makeAuthenticatedRequest(http.MethodGet, testServerURL+"/health", nil)
			if err != nil {
				t.Fatalf("creating GET request for health list: %v", err)
			}
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("GET /health error: %v", err)
			}
			var healthItems []models.Item
			if err := json.NewDecoder(resp.Body).Decode(&healthItems); err != nil {
				t.Fatalf("decode health list: %v", err)
			}
			resp.Body.Close()
			if len(healthItems) != 2 {
				t.Fatalf("expected 2 health items, got %d", len(healthItems))
			}

			idsByName := map[string]string{}
			for _, itm := range healthItems {
				idsByName[itm.Name] = itm.ID
			}

			// Re-post identical payload to confirm idempotent creates
			req, err = makeAuthenticatedRequest(http.MethodPost, testServerURL+"/health", bytes.NewReader(healthPayload))
			if err != nil {
				t.Fatalf("creating POST /health idempotent request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("POST /health idempotent error: %v", err)
			}
			if resp.StatusCode != http.StatusCreated {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("POST /health idempotent status %d, body: %s", resp.StatusCode, body)
			}
			resp.Body.Close()

			req, err = makeAuthenticatedRequest(http.MethodGet, testServerURL+"/health", nil)
			if err != nil {
				t.Fatalf("creating GET request for health list after idempotent POST: %v", err)
			}
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("GET /health after idempotent POST error: %v", err)
			}
			var healthAgain []models.Item
			if err := json.NewDecoder(resp.Body).Decode(&healthAgain); err != nil {
				t.Fatalf("decode health list after idempotent POST: %v", err)
			}
			resp.Body.Close()
			if len(healthAgain) != 2 {
				t.Fatalf("expected 2 health items after idempotent POST, got %d", len(healthAgain))
			}
			for _, itm := range healthAgain {
				if prev, ok := idsByName[itm.Name]; !ok || prev != itm.ID {
					t.Fatalf("health item changed after idempotent POST for %s: before %s after %s", itm.Name, prev, itm.ID)
				}
			}
			healthItems = healthAgain

			byName := map[string]models.Item{}
			var names []string
			for _, itm := range healthItems {
				byName[itm.Name] = itm
				names = append(names, itm.Name)
			}

			hrvItem, ok := byName[models.HealthMetricHeartRateVariability]
			if !ok {
				t.Fatalf("health items missing hrv entry; names=%v", names)
			}
			var summary models.HealthMetricSummary
			summaryBytes, err := json.Marshal(hrvItem.Data)
			if err != nil {
				t.Fatalf("marshal hrv data: %v", err)
			}
			if err := json.Unmarshal(summaryBytes, &summary); err != nil {
				t.Fatalf("unmarshal hrv summary: %v", err)
			}
			if summary.Count != 2 || summary.Total != 150 || summary.Average != 75 {
				t.Fatalf("unexpected hrv summary: %+v", summary)
			}

			sleepItem, ok := byName[models.HealthMetricSleepAnalysis]
			if !ok {
				t.Fatalf("health items missing sleep entry; names=%v", names)
			}
			var sleep models.SleepSummary
			sleepBytes, err := json.Marshal(sleepItem.Data)
			if err != nil {
				t.Fatalf("marshal sleep data: %v", err)
			}
			if err := json.Unmarshal(sleepBytes, &sleep); err != nil {
				t.Fatalf("unmarshal sleep summary: %v", err)
			}
			if sleep.DurationHours < 7.9 || sleep.DurationHours > 8.1 {
				t.Fatalf("unexpected sleep duration: %+v", sleep)
			}

			// HEALTH: time window queries
			req, err = makeAuthenticatedRequest(http.MethodGet, testServerURL+"/health?startTime=2024-02-01T00:30:00Z", nil)
			if err != nil {
				t.Fatalf("creating GET request for health startTime filter: %v", err)
			}
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("GET /health startTime filter error: %v", err)
			}
			var healthWindow []models.Item
			if err := json.NewDecoder(resp.Body).Decode(&healthWindow); err != nil {
				t.Fatalf("decode health startTime filter: %v", err)
			}
			resp.Body.Close()
			if len(healthWindow) != 1 {
				t.Fatalf("expected 1 health item after startTime filter, got %d", len(healthWindow))
			}
			if healthWindow[0].Name != models.HealthMetricHeartRateVariability {
				t.Fatalf("unexpected health item returned for startTime filter: %s", healthWindow[0].Name)
			}

			req, err = makeAuthenticatedRequest(http.MethodGet, testServerURL+"/health?startTime=2024-02-01T00:00:00Z&endTime=2024-02-01T00:10:00Z", nil)
			if err != nil {
				t.Fatalf("creating GET request for health time range filter: %v", err)
			}
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("GET /health time range filter error: %v", err)
			}
			var healthRange []models.Item
			if err := json.NewDecoder(resp.Body).Decode(&healthRange); err != nil {
				t.Fatalf("decode health time range filter: %v", err)
			}
			resp.Body.Close()
			if len(healthRange) != 1 {
				t.Fatalf("expected 1 health item in time range, got %d", len(healthRange))
			}
			if healthRange[0].Name != models.HealthMetricSleepAnalysis {
				t.Fatalf("unexpected health item returned for time range filter: %s", healthRange[0].Name)
			}

			req, err = makeAuthenticatedRequest(http.MethodGet, testServerURL+"/health?startTime=1706745600000&endTime=1706752800000", nil)
			if err != nil {
				t.Fatalf("creating GET request for health epoch window: %v", err)
			}
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("GET /health epoch window error: %v", err)
			}
			var healthEpoch []models.Item
			if err := json.NewDecoder(resp.Body).Decode(&healthEpoch); err != nil {
				t.Fatalf("decode health epoch window: %v", err)
			}
			resp.Body.Close()
			if len(healthEpoch) != 2 {
				t.Fatalf("expected 2 health items in epoch window, got %d", len(healthEpoch))
			}

			// DELETE all
			for _, c := range cases {
				req, err := makeAuthenticatedRequest(http.MethodDelete, testServerURL+"/items/"+c.itm.ID, nil)
				if err != nil {
					t.Fatalf("creating DELETE request: %v", err)
				}
				resp, err := client.Do(req)
				if err != nil {
					t.Fatalf("DELETE /items/%s error: %v", c.itm.ID, err)
				}
				if resp.StatusCode != http.StatusNoContent {
					t.Errorf("DELETE /items/%s status %d", c.itm.ID, resp.StatusCode)
				}
				resp.Body.Close()
			}

			// FINAL LIST (should be empty)
			req, err = makeAuthenticatedRequest(http.MethodGet, testServerURL+"/items", nil)
			if err != nil {
				t.Fatalf("creating GET request for final list: %v", err)
			}
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("GET final /items error: %v", err)
			}
			var final []models.Item
			if err := json.NewDecoder(resp.Body).Decode(&final); err != nil {
				t.Fatalf("decode final list: %v", err)
			}
			resp.Body.Close()
			if len(final) != 0 {
				t.Errorf("expected 0 items after delete, got %d", len(final))
			}
		})
	}
}

// newTestLogger returns a logger that outputs to stdout for test visibility.
func newTestLogger() *log.Logger {
	return log.New(os.Stdout, "[TEST] ", log.LstdFlags)
}
