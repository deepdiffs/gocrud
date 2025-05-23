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

	"github.com/go-redis/redis/v8"
)

var (
	testServerURL string
	redisClient   *redis.Client
	testCtx       = context.Background()
)

// TestMain sets up the Redis DB and HTTP server, then runs the tests.
func TestMain(m *testing.M) {
	// flush Redis DB for a clean slate
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	redisClient = redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := redisClient.FlushDB(testCtx).Err(); err != nil {
		panic("failed to flush redis DB: " + err.Error())
	}

	// start HTTP server using the real handlers
	store := NewRedisStore(redisClient)
	logger := newTestLogger()
	handler := NewHandler(store, logger)
	mux := http.NewServeMux()
	mux.HandleFunc("/items", handler.itemsHandler)
	mux.HandleFunc("/items/", handler.itemHandler)
	// wrap with API-key auth and logging middleware
	validKeys := map[string]struct{}{testAPIKey: {}}
	srv := httptest.NewServer(loggingMiddleware(logger)(authMiddleware(validKeys)(mux)))
	defer srv.Close()
	testServerURL = srv.URL

	code := m.Run()
	// clean up Redis
	_ = redisClient.FlushDB(testCtx)
	os.Exit(code)
}

// testAPIKey is the static API key used to authenticate integration test requests.
const testAPIKey = "test-integration-key"

// TestCRUDIntegration exercises Create, Read, Update, List (with and without type filter), and Delete.
func TestCRUDIntegration(t *testing.T) {
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

	// HTTP client that injects the test API key into each request
	client := &http.Client{Transport: &authTransport{token: testAPIKey, base: http.DefaultTransport}}
	// CREATE
	for i := range cases {
		path := filepath.Join("mockdata", cases[i].file)
		data, _ := os.ReadFile(path)
		req, err := http.NewRequest(http.MethodPost, testServerURL+"/items", bytes.NewReader(data))
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
		resp, err := client.Get(testServerURL + "/items/" + c.itm.ID)
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
	resp, err = client.Get(testServerURL + "/items/" + targetID)
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
	resp, err = client.Get(testServerURL + "/items")
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
	resp, err = client.Get(testServerURL + "/items?type=" + updReq.Type)
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
	resp, err = client.Get(testServerURL + "/items")
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
}

// authTransport injects the test API key into outgoing HTTP requests.
type authTransport struct {
	token string
	base  http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}

// newTestLogger returns a logger that outputs to stdout for test visibility.
func newTestLogger() *log.Logger {
	return log.New(os.Stdout, "[TEST] ", log.LstdFlags)
}
