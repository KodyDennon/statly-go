// Package gin provides Gin middleware for Statly error tracking.
package gin

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/KodyDennon/statly-go"
)

// Options configures the Gin middleware.
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

// Recovery returns a Gin middleware that recovers from panics.
func Recovery(options Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Build request info
				requestInfo := extractRequestInfo(c)

				// Add breadcrumb
				statly.AddBreadcrumb(statly.Breadcrumb{
					Message:  fmt.Sprintf("%s %s", c.Request.Method, c.Request.URL.Path),
					Category: "http",
					Level:    statly.LevelInfo,
					Data: map[string]interface{}{
						"method": c.Request.Method,
						"url":    c.Request.URL.String(),
					},
				})

				// Set tags
				statly.SetTag("http.method", c.Request.Method)
				statly.SetTag("http.url", c.Request.URL.Path)
				statly.SetTag("transaction", c.FullPath())

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
					"request": requestInfo,
				})

				if options.WaitForDelivery {
					statly.Flush()
				}

				// Set error on context
				c.Error(captureErr)

				// Abort with error
				c.AbortWithStatus(http.StatusInternalServerError)

				if options.Repanic {
					panic(err)
				}
			}
		}()

		c.Next()
	}
}

// Logger returns middleware that logs requests as breadcrumbs.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Add request breadcrumb
		statly.AddBreadcrumb(statly.Breadcrumb{
			Message:  fmt.Sprintf("%s %s", c.Request.Method, c.Request.URL.Path),
			Category: "http",
			Level:    statly.LevelInfo,
			Data: map[string]interface{}{
				"method": c.Request.Method,
				"url":    c.Request.URL.String(),
			},
		})

		c.Next()

		// Add response breadcrumb
		duration := time.Since(start)
		level := statly.LevelInfo
		if c.Writer.Status() >= 400 {
			level = statly.LevelError
		}

		statly.AddBreadcrumb(statly.Breadcrumb{
			Message:  fmt.Sprintf("Response %d", c.Writer.Status()),
			Category: "http",
			Level:    level,
			Data: map[string]interface{}{
				"status_code": c.Writer.Status(),
				"duration_ms": float64(duration.Nanoseconds()) / 1e6,
			},
		})
	}
}

// ErrorHandler returns middleware that captures errors from c.Error().
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Check for errors
		if len(c.Errors) > 0 {
			requestInfo := extractRequestInfo(c)

			for _, ginErr := range c.Errors {
				statly.CaptureExceptionWithContext(ginErr.Err, map[string]interface{}{
					"request": requestInfo,
					"meta":    ginErr.Meta,
				})
			}
		}
	}
}

// extractRequestInfo extracts request information from a Gin context.
func extractRequestInfo(c *gin.Context) map[string]interface{} {
	r := c.Request

	info := map[string]interface{}{
		"method":       r.Method,
		"url":          r.URL.String(),
		"path":         r.URL.Path,
		"full_path":    c.FullPath(),
		"query_string": r.URL.RawQuery,
		"host":         r.Host,
		"remote_addr":  c.ClientIP(),
	}

	// Add path parameters
	if len(c.Params) > 0 {
		params := make(map[string]string)
		for _, p := range c.Params {
			params[p.Key] = p.Value
		}
		info["params"] = params
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

// SetUserFromContext is a helper to set user context.
func SetUserFromContext(c *gin.Context, user statly.User) {
	statly.SetUser(user)
}
