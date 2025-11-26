package middleware

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// LoggingMiddleware logs full HTTP requests and responses including headers and bodies.
func LoggingMiddleware(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Log incoming request
			requestLog := logIncomingRequest(r)
			logger.Printf("[REQUEST] INCOMING REQUEST:\n%s", requestLog)

			// Wrap response writer to capture response data
			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
				body:           &bytes.Buffer{},
			}

			// Process request
			next.ServeHTTP(rw, r)

			// Log outgoing response
			duration := time.Since(start)
			responseLog := logOutgoingResponse(rw, duration)
			logger.Printf("[RESPONSE] OUTGOING RESPONSE:\n%s", responseLog)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code and response body.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

// WriteHeader captures the status code and writes the header.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures the response body and writes to the underlying writer.
func (rw *responseWriter) Write(data []byte) (int, error) {
	rw.body.Write(data)
	return rw.ResponseWriter.Write(data)
}

// logIncomingRequest creates a detailed log of the incoming HTTP request.
func logIncomingRequest(r *http.Request) string {
	var buf bytes.Buffer

	// Basic request info
	fmt.Fprintf(&buf, "%s %s %s\n", r.Method, r.URL.String(), r.Proto)
	fmt.Fprintf(&buf, "Host: %s\n", r.Host)

	// Headers
	fmt.Fprintf(&buf, "Headers:\n")
	for name, values := range r.Header {
		for _, value := range values {
			fmt.Fprintf(&buf, "  %s: %s\n", name, value)
		}
	}

	// Query parameters
	if len(r.URL.Query()) > 0 {
		fmt.Fprintf(&buf, "Query Parameters:\n")
		for name, values := range r.URL.Query() {
			for _, value := range values {
				fmt.Fprintf(&buf, "  %s: %s\n", name, value)
			}
		}
	}

	// Request body (if present)
	if r.Body != nil && r.ContentLength > 0 {
		bodyBytes, err := io.ReadAll(r.Body)
		if err == nil {
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore body for handler
			fmt.Fprintf(&buf, "Body:\n%s\n", string(bodyBytes))
		} else {
			fmt.Fprintf(&buf, "Body: [error reading: %v]\n", err)
		}
	}

	return buf.String()
}

// logOutgoingResponse creates a detailed log of the outgoing HTTP response.
func logOutgoingResponse(rw *responseWriter, duration time.Duration) string {
	var buf bytes.Buffer

	// Basic response info
	fmt.Fprintf(&buf, "Status: %d %s\n", rw.statusCode, http.StatusText(rw.statusCode))
	fmt.Fprintf(&buf, "Duration: %s\n", duration)

	// Response headers
	fmt.Fprintf(&buf, "Headers:\n")
	for name, values := range rw.Header() {
		for _, value := range values {
			fmt.Fprintf(&buf, "  %s: %s\n", name, value)
		}
	}

	// Response body
	if rw.body.Len() > 0 {
		fmt.Fprintf(&buf, "Body:\n%s\n", rw.body.String())
	}

	return buf.String()
}

// AuthMiddleware enforces API-key authentication via a custom header.
func AuthMiddleware(validKeys map[string]struct{}, logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
			if apiKey == "" {
				logger.Printf("[AUTH] ERROR: API key is missing from request")
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			if _, ok := validKeys[apiKey]; !ok {
				logger.Printf("[AUTH] ERROR: Invalid API key provided for %s %s", r.Method, r.URL.Path)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ParseAPIKeys parses a comma-separated list of API keys into a set.
func ParseAPIKeys(s string) map[string]struct{} {
	keys := make(map[string]struct{})
	for _, k := range strings.Split(s, ",") {
		if v := strings.TrimSpace(k); v != "" {
			keys[v] = struct{}{}
		}
	}
	return keys
}
