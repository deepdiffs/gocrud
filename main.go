package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-redis/redis/v8"
)

func main() {
	logger := log.New(os.Stdout, "go-crud ", log.LstdFlags|log.Lmicroseconds)
	ctx := context.Background()

	// allow overriding Redis address via REDIS_ADDR env var, default to localhost:6379
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Fatalf("could not connect to redis (%s): %v", redisAddr, err)
	}

	store := NewRedisStore(redisClient)
	handler := NewHandler(store, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/items", handler.itemsHandler)
	mux.HandleFunc("/items/", handler.itemHandler)

	loggedMux := loggingMiddleware(logger)(mux)

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
		logger.Printf("server is listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("could not listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	logger.Println("server is shutting down")

	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctxShutdown); err != nil {
		logger.Fatalf("server forced to shutdown: %v", err)
	}

	logger.Println("server stopped")
}
