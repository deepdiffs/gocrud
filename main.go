package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	logger := log.New(os.Stdout, "go-crud ", log.LstdFlags|log.Lmicroseconds)
	ctx := context.Background()

	logger.Printf("Starting go-crud application")

	store, err := NewStore(ctx, logger)
	if err != nil {
		logger.Fatalf("FATAL: failed to initialize store: %v", err)
	}

	handler := NewHandler(store, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/items", handler.itemsHandler)
	mux.HandleFunc("/items/", handler.itemHandler)

	// Load API keys for authentication (comma-separated list in API_KEYS env var).
	keysEnv := os.Getenv("API_KEYS")
	if keysEnv == "" {
		logger.Fatal("FATAL: environment variable API_KEYS is required for authentication")
	}
	validKeys := parseAPIKeys(keysEnv)
	authMux := authMiddleware(validKeys, logger)(mux)
	loggedMux := loggingMiddleware(logger)(authMux)

	// allow overriding HTTP listen address via HTTP_ADDR env var, default to :9090
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":9090"
	}
	server := &http.Server{
		Addr:         httpAddr,
		Handler:      loggedMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Printf("Starting HTTP server on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("FATAL: could not listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	logger.Printf("Received shutdown signal, initiating graceful shutdown")

	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctxShutdown); err != nil {
		logger.Fatalf("FATAL: server forced to shutdown: %v", err)
	}

	logger.Printf("Application shutdown completed successfully")
}
