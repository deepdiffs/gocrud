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
)

var (
	testCtx = context.Background()
)

// setupTestServer creates a test HTTP server with the specified storage backend.
// Returns the server URL and a cleanup function.
func setupTestServer(t *testing.T, backend string) (string, func()) {
	// Save original STORAGE_BACKEND
	originalBackend := os.Getenv("STORAGE_BACKEND")

	// Set the backend for this test
	if backend == "redis" {
		os.Setenv("STORAGE_BACKEND", "redis")
		redisAddr := os.Getenv("REDIS_ADDR")
		if redisAddr == "" {
			redisAddr = "localhost:6379"
		}
		os.Setenv("REDIS_ADDR", redisAddr)
		os.Setenv("REDIS_DB", "15") // Use DB 15 for testing
	} else {
		os.Setenv("STORAGE_BACKEND", "memory")
		// Clear Redis env vars to ensure memory store is used
		os.Unsetenv("REDIS_ADDR")
		os.Unsetenv("REDIS_DB")
	}

	logger := newTestLogger()
	store, err := NewStore(testCtx, logger)
	if err != nil {
		t.Fatalf("failed to create store with backend %s: %v", backend, err)
	}

	// Clean up Redis DB if using Redis backend
	if backend == "redis" {
		if redisStore, ok := store.(*RedisStore); ok {
			if err := redisStore.client.FlushDB(testCtx).Err(); err != nil {
				t.Fatalf("failed to flush test redis DB: %v", err)
			}
		}
	}

	handler := NewHandler(store, logger)
	mux := http.NewServeMux()
	mux.HandleFunc("/items", handler.itemsHandler)
	mux.HandleFunc("/items/", handler.itemHandler)
	srv := httptest.NewServer(loggingMiddleware(logger)(mux))

	cleanup := func() {
		srv.Close()
		// Restore original STORAGE_BACKEND
		if originalBackend == "" {
			os.Unsetenv("STORAGE_BACKEND")
		} else {
			os.Setenv("STORAGE_BACKEND", originalBackend)
		}
		// Clean up Redis DB if using Redis backend
		if backend == "redis" {
			if redisStore, ok := store.(*RedisStore); ok {
				_ = redisStore.client.FlushDB(testCtx)
			}
		}
	}

	return srv.URL, cleanup
}

// TestCRUDIntegration exercises Create, Read, Update, List (with and without type filter), and Delete.
// Tests both memory and Redis storage backends.
func TestCRUDIntegration(t *testing.T) {
	backends := []string{"memory", "redis"}

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
				req  CreateItemRequest
				itm  Item
			}
			var cases []createCase
			for _, fn := range createFiles {
				path := filepath.Join("mockdata", fn)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("reading %s: %v", path, err)
				}
				var req CreateItemRequest
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
				resp, err := http.Post(testServerURL+"/items", "application/json", bytes.NewReader(data))
				if err != nil {
					t.Fatalf("POST /items (%s) error: %v", cases[i].file, err)
				}
				if resp.StatusCode != http.StatusCreated {
					body, _ := io.ReadAll(resp.Body)
					t.Fatalf("POST /items (%s) status %d, body: %s", cases[i].file, resp.StatusCode, body)
				}
				var out Item
				if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
					t.Fatalf("decode created (%s): %v", cases[i].file, err)
				}
				resp.Body.Close()
				if out.ID == "" {
					t.Fatalf("empty ID for %s", cases[i].file)
				}
				cases[i].itm = out
			}

			// READ each
			for _, c := range cases {
				resp, err := http.Get(testServerURL + "/items/" + c.itm.ID)
				if err != nil {
					t.Fatalf("GET /items/%s error: %v", c.itm.ID, err)
				}
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("GET /items/%s status %d", c.itm.ID, resp.StatusCode)
				}
				var got Item
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
			var updReq UpdateItemRequest
			if err := json.Unmarshal(updData, &updReq); err != nil {
				t.Fatalf("unmarshal update payload: %v", err)
			}
			targetID := cases[0].itm.ID

			req, err := http.NewRequest(http.MethodPut, testServerURL+"/items/"+targetID, bytes.NewReader(updData))
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
			var updated Item
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
			if !bytes.Contains(updated.Data, []byte(`"price":899.99`)) {
				t.Errorf("updated data not applied: %s", updated.Data)
			}

			// VERIFY update via GET
			resp, err = http.Get(testServerURL + "/items/" + targetID)
			if err != nil {
				t.Fatalf("GET after update error: %v", err)
			}
			var after Item
			if err := json.NewDecoder(resp.Body).Decode(&after); err != nil {
				t.Fatalf("decode after update: %v", err)
			}
			resp.Body.Close()
			if after.LastModified.Equal(after.CreatedAt) {
				t.Errorf("LastModified not updated: created %s, lastModified %s", after.CreatedAt, after.LastModified)
			}

			// LIST all
			resp, err = http.Get(testServerURL + "/items")
			if err != nil {
				t.Fatalf("GET /items error: %v", err)
			}
			var list []Item
			if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
				t.Fatalf("decode list: %v", err)
			}
			resp.Body.Close()
			if len(list) != len(cases) {
				t.Errorf("expected %d items, got %d", len(cases), len(list))
			}

			// LIST by type filter
			resp, err = http.Get(testServerURL + "/items?type=" + updReq.Type)
			if err != nil {
				t.Fatalf("GET /items?type=%s error: %v", updReq.Type, err)
			}
			var filtered []Item
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

			// DELETE all
			for _, c := range cases {
				req, err := http.NewRequest(http.MethodDelete, testServerURL+"/items/"+c.itm.ID, nil)
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
			resp, err = http.Get(testServerURL + "/items")
			if err != nil {
				t.Fatalf("GET final /items error: %v", err)
			}
			var final []Item
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
