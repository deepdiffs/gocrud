package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"gocrud/internal/handlers"
	"gocrud/internal/middleware"
	"gocrud/internal/store"
)

func main() {
	logger := log.New(os.Stdout, "go-crud ", log.LstdFlags|log.Lmicroseconds)
	ctx := context.Background()

	logger.Printf("Starting go-crud application")

	backend := os.Getenv("STORAGE_BACKEND")
	if backend == "" {
		backend = "memory"
	}
	firestoreCollection := os.Getenv("FIRESTORE_COLLECTION")
	if firestoreCollection == "" {
		firestoreCollection = "items"
	}
	logger.Printf(
		"Config snapshot -> STORAGE_BACKEND=%q FIRESTORE_COLLECTION=%q HTTP_ADDR=%q PORT=%q GOOGLE_CLOUD_PROJECT=%q GCLOUD_PROJECT=%q API_KEYS_present=%t",
		backend,
		firestoreCollection,
		os.Getenv("HTTP_ADDR"),
		os.Getenv("PORT"),
		os.Getenv("GOOGLE_CLOUD_PROJECT"),
		os.Getenv("GCLOUD_PROJECT"),
		os.Getenv("API_KEYS") != "",
	)

	// Create stores for different collections
	itemsStore, err := store.NewStore(ctx, logger)
	if err != nil {
		logger.Fatalf("FATAL: failed to initialize items store: %v", err)
	}

	workoutsStore, err := store.NewStoreWithCollection(ctx, logger, "workouts")
	if err != nil {
		logger.Fatalf("FATAL: failed to initialize workouts store: %v", err)
	}

	healthStore, err := store.NewStoreWithCollection(ctx, logger, "healthstuff")
	if err != nil {
		logger.Fatalf("FATAL: failed to initialize health store: %v", err)
	}

	// Create handlers for each endpoint
	itemsHandler := handlers.NewHandler(itemsStore, logger)
	workoutsHandler := handlers.NewHandler(workoutsStore, logger)
	healthHandler := handlers.NewHandler(healthStore, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/items", itemsHandler.ItemsHandler)
	mux.HandleFunc("/items/", itemsHandler.ItemHandler)
	mux.HandleFunc("/workouts", workoutsHandler.WorkoutsHandler)
	mux.HandleFunc("/workouts/", workoutsHandler.ItemHandler)
	mux.HandleFunc("/health", healthHandler.HealthHandler)
	mux.HandleFunc("/health/", healthHandler.ItemHandler)

	// Load API keys for authentication (comma-separated list in API_KEYS env var).
	keysEnv := os.Getenv("API_KEYS")
	if keysEnv == "" {
		logger.Fatal("FATAL: environment variable API_KEYS is required for authentication")
	}
	validKeys := middleware.ParseAPIKeys(keysEnv)
	logger.Printf("Authentication configured with %d API keys", len(validKeys))
	authMux := middleware.AuthMiddleware(validKeys, logger)(mux)
	loggedMux := middleware.LoggingMiddleware(logger)(authMux)

	// allow overriding HTTP listen address via HTTP_ADDR env var. On Cloud Run, PORT
	// is provided; fall back to :9090 locally.
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		if port := os.Getenv("PORT"); port != "" {
			httpAddr = ":" + port
		} else {
			httpAddr = ":9090"
		}
	}
	logger.Printf("HTTP server address resolved to %s", httpAddr)
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
