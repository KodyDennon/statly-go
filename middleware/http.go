// Package middleware provides HTTP middleware for Statly error tracking.
package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/KodyDennon/statly-go"
)

// Options configures the HTTP middleware.
type Options struct {
	// Repanic controls whether to re-panic after capturing.
	Repanic bool

	// WaitForDelivery waits for the event to be sent before continuing.
	WaitForDelivery bool

	// Timeout is the time to wait for delivery.
	Timeout time.Duration
}

// DefaultOptions returns sensible default options.
func DefaultOptions() Options {
	return Options{
		Repanic:         true,
		WaitForDelivery: false,
		Timeout:         2 * time.Second,
	}
}

// Recovery returns an HTTP middleware that recovers from panics.
func Recovery(options Options) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					// Build request info
					requestInfo := extractRequestInfo(r)

					// Add breadcrumb
					statly.AddBreadcrumb(statly.Breadcrumb{
						Message:  fmt.Sprintf("%s %s", r.Method, r.URL.Path),
						Category: "http",
						Level:    statly.LevelInfo,
						Data: map[string]interface{}{
							"method": r.Method,
							"url":    r.URL.String(),
						},
					})

					// Set tags
					statly.SetTag("http.method", r.Method)
					statly.SetTag("http.url", r.URL.Path)

					// Convert panic to error
					var captureErr error
					switch v := err.(type) {
					case error:
						captureErr = v
					case string:
						captureErr = fmt.Errorf("%s", v)
					default:
						captureErr = fmt.Errorf("%v", v)
					}

					// Capture with context
					statly.CaptureExceptionWithContext(captureErr, map[string]interface{}{
						"request":    requestInfo,
						"stacktrace": string(debug.Stack()),
					})

					if options.WaitForDelivery {
						statly.Flush()
					}

					// Write error response
					w.WriteHeader(http.StatusInternalServerError)

					if options.Repanic {
						panic(err)
					}
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// RequestLogger returns middleware that logs requests as breadcrumbs.
func RequestLogger() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Add request breadcrumb
			statly.AddBreadcrumb(statly.Breadcrumb{
				Message:  fmt.Sprintf("%s %s", r.Method, r.URL.Path),
				Category: "http",
				Level:    statly.LevelInfo,
				Data: map[string]interface{}{
					"method": r.Method,
					"url":    r.URL.String(),
				},
			})

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			// Add response breadcrumb
			duration := time.Since(start)
			level := statly.LevelInfo
			if wrapped.statusCode >= 400 {
				level = statly.LevelError
			}

			statly.AddBreadcrumb(statly.Breadcrumb{
				Message:  fmt.Sprintf("Response %d", wrapped.statusCode),
				Category: "http",
				Level:    level,
				Data: map[string]interface{}{
					"status_code": wrapped.statusCode,
					"duration_ms": float64(duration.Nanoseconds()) / 1e6,
				},
			})
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (w *responseWriter) WriteHeader(code int) {
	if !w.written {
		w.statusCode = code
		w.written = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.written = true
	}
	return w.ResponseWriter.Write(b)
}

// extractRequestInfo extracts request information from an HTTP request.
func extractRequestInfo(r *http.Request) map[string]interface{} {
	info := map[string]interface{}{
		"method":       r.Method,
		"url":          r.URL.String(),
		"path":         r.URL.Path,
		"query_string": r.URL.RawQuery,
		"host":         r.Host,
		"remote_addr":  r.RemoteAddr,
	}

	// Sanitize headers
	headers := make(map[string]string)
	sensitiveHeaders := map[string]bool{
		"authorization": true,
		"cookie":        true,
		"x-api-key":     true,
		"x-auth-token":  true,
	}

	for key, values := range r.Header {
		lowerKey := strings.ToLower(key)
		if sensitiveHeaders[lowerKey] {
			headers[key] = "[Filtered]"
		} else if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	info["headers"] = headers

	return info
}

// SetUserFromRequest is a helper to set user context from a request.
// You can customize this based on how your application handles authentication.
func SetUserFromRequest(r *http.Request, getUserFunc func(*http.Request) *statly.User) {
	if getUserFunc != nil {
		if user := getUserFunc(r); user != nil {
			statly.SetUser(*user)
		}
	}
}
