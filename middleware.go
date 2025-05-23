package main

import (
	"log"
	"net/http"
	"strings"
	"time"
)

// loggingMiddleware logs HTTP requests with method, path, status, and duration.
func loggingMiddleware(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{w, http.StatusOK}
			next.ServeHTTP(rw, r)
			logger.Printf("%s %s %d %s", r.Method, r.URL.Path, rw.statusCode, time.Since(start))
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code and writes the header.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// authMiddleware enforces API-key authentication via Bearer tokens.
func authMiddleware(validKeys map[string]struct{}) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(authHeader, prefix) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="gocrud"`)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}
			token := strings.TrimSpace(strings.TrimPrefix(authHeader, prefix))
			if _, ok := validKeys[token]; !ok {
				w.Header().Set("WWW-Authenticate", `Bearer realm="gocrud", error="invalid_token"`)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// parseAPIKeys parses a comma-separated list of API keys into a set.
func parseAPIKeys(s string) map[string]struct{} {
	keys := make(map[string]struct{})
	for _, k := range strings.Split(s, ",") {
		if v := strings.TrimSpace(k); v != "" {
			keys[v] = struct{}{}
		}
	}
	return keys
}
