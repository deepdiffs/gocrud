package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

func main() {
	logger := log.New(os.Stdout, "go-crud ", log.LstdFlags|log.Lmicroseconds)
	ctx := context.Background()

	logger.Printf("INFO: Starting go-crud application")

	redisAddr := os.Getenv("REDIS_ADDR")

	logger.Printf("INFO: Initializing Redis client")
	// pick the right redis DB from the env var
	redisDB := os.Getenv("REDIS_DB")
	if redisDB == "" {
		redisDB = "0"
	}
	redisDBInt, err := strconv.Atoi(redisDB)
	if err != nil {
		logger.Fatalf("FATAL: could not convert REDIS_DB to int: %v", err)
	}

	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr, DB: redisDBInt})

	logger.Printf("INFO: Testing Redis connection...")
	start := time.Now()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Printf("ERROR: Failed to connect to Redis (%s): %v", redisAddr, err)
		logger.Fatalf("FATAL: could not connect to redis (%s): %v", redisAddr, err)
	}
	connectionTime := time.Since(start)
	logger.Printf("INFO: Successfully connected to Redis (%s) in %v", redisAddr, connectionTime)

	// Test Redis operations to ensure full functionality
	logger.Printf("INFO: Testing Redis operations...")
	if err := testRedisOperations(ctx, redisClient, logger); err != nil {
		logger.Printf("ERROR: Redis operations test failed: %v", err)
		logger.Fatalf("FATAL: Redis operations test failed: %v", err)
	}
	logger.Printf("INFO: Redis operations test completed successfully")

	logger.Printf("INFO: Initializing Redis store")
	store := NewRedisStore(redisClient)
	logger.Printf("INFO: Redis store initialized successfully")

	logger.Printf("INFO: Initializing HTTP handler")
	handler := NewHandler(store, logger)
	logger.Printf("INFO: HTTP handler initialized successfully")

	mux := http.NewServeMux()
	mux.HandleFunc("/items", handler.itemsHandler)
	mux.HandleFunc("/items/", handler.itemHandler)
	logger.Printf("INFO: HTTP routes configured")

	// Load API keys for authentication (comma-separated list in API_KEYS env var).
	keysEnv := os.Getenv("API_KEYS")
	if keysEnv == "" {
		logger.Printf("ERROR: API_KEYS environment variable is not set")
		logger.Fatal("FATAL: environment variable API_KEYS is required for authentication")
	}
	validKeys := parseAPIKeys(keysEnv)
	logger.Printf("INFO: Loaded %d API keys for authentication", len(validKeys))
	authMux := authMiddleware(validKeys)(mux)
	loggedMux := loggingMiddleware(logger)(authMux)
	logger.Printf("INFO: Middleware configured (authentication and logging)")

	// allow overriding HTTP listen address via HTTP_ADDR env var, default to :9090
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":9090"
		logger.Printf("INFO: HTTP_ADDR not set, using default: %s", httpAddr)
	} else {
		logger.Printf("INFO: Using HTTP address from environment: %s", httpAddr)
	}
	server := &http.Server{
		Addr:         httpAddr,
		Handler:      loggedMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Printf("INFO: Starting HTTP server on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Printf("ERROR: HTTP server failed: %v", err)
			logger.Fatalf("FATAL: could not listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	logger.Printf("INFO: Application startup completed, waiting for shutdown signal")
	<-quit
	logger.Printf("INFO: Received shutdown signal, initiating graceful shutdown")

	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctxShutdown); err != nil {
		logger.Printf("ERROR: Graceful shutdown failed: %v", err)
		logger.Fatalf("FATAL: server forced to shutdown: %v", err)
	}

	logger.Printf("INFO: Application shutdown completed successfully")
}

// testRedisOperations performs basic Redis operations to ensure the connection is fully functional
func testRedisOperations(ctx context.Context, client *redis.Client, logger *log.Logger) error {
	logger.Printf("INFO: Testing Redis SET operation")
	if err := client.Set(ctx, "test:startup", "ok", time.Minute).Err(); err != nil {
		return fmt.Errorf("SET operation failed: %w", err)
	}

	logger.Printf("INFO: Testing Redis GET operation")
	val, err := client.Get(ctx, "test:startup").Result()
	if err != nil {
		return fmt.Errorf("GET operation failed: %w", err)
	}
	if val != "ok" {
		return fmt.Errorf("GET operation returned unexpected value: %s", val)
	}

	logger.Printf("INFO: Testing Redis DEL operation")
	if err := client.Del(ctx, "test:startup").Err(); err != nil {
		return fmt.Errorf("DEL operation failed: %w", err)
	}

	logger.Printf("INFO: Testing Redis SADD operation")
	if err := client.SAdd(ctx, "test:set", "item1").Err(); err != nil {
		return fmt.Errorf("SADD operation failed: %w", err)
	}

	logger.Printf("INFO: Testing Redis SMEMBERS operation")
	members, err := client.SMembers(ctx, "test:set").Result()
	if err != nil {
		return fmt.Errorf("SMEMBERS operation failed: %w", err)
	}
	if len(members) != 1 || members[0] != "item1" {
		return fmt.Errorf("SMEMBERS operation returned unexpected values: %v", members)
	}

	logger.Printf("INFO: Testing Redis SREM operation")
	if err := client.SRem(ctx, "test:set", "item1").Err(); err != nil {
		return fmt.Errorf("SREM operation failed: %w", err)
	}

	logger.Printf("INFO: Testing Redis SINTER operation")
	_, err = client.SInter(ctx, "test:set").Result()
	if err != nil {
		return fmt.Errorf("SINTER operation failed: %w", err)
	}

	logger.Printf("INFO: Cleaning up test data")
	if err := client.Del(ctx, "test:set").Err(); err != nil {
		logger.Printf("WARN: Failed to clean up test set: %v", err)
	}

	return nil
}
