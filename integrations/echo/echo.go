// Package echo provides Echo middleware for Statly error tracking.
package echo

import (
	"fmt"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/KodyDennon/statly-go"
)

// Options configures the Echo middleware.
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

// Recovery returns an Echo middleware that recovers from panics.
func Recovery(options Options) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			defer func() {
				if r := recover(); r != nil {
					// Build request info
					requestInfo := extractRequestInfo(c)

					// Add breadcrumb
					statly.AddBreadcrumb(statly.Breadcrumb{
						Message:  fmt.Sprintf("%s %s", c.Request().Method, c.Request().URL.Path),
						Category: "http",
						Level:    statly.LevelInfo,
						Data: map[string]interface{}{
							"method": c.Request().Method,
							"url":    c.Request().URL.String(),
						},
					})

					// Set tags
					statly.SetTag("http.method", c.Request().Method)
					statly.SetTag("http.url", c.Request().URL.Path)
					statly.SetTag("transaction", c.Path())

					// Convert panic to error
					var captureErr error
					switch v := r.(type) {
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

					if options.Repanic {
						panic(r)
					}

					c.Error(captureErr)
				}
			}()

			return next(c)
		}
	}
}

// Logger returns middleware that logs requests as breadcrumbs.
func Logger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			// Add request breadcrumb
			statly.AddBreadcrumb(statly.Breadcrumb{
				Message:  fmt.Sprintf("%s %s", c.Request().Method, c.Request().URL.Path),
				Category: "http",
				Level:    statly.LevelInfo,
				Data: map[string]interface{}{
					"method": c.Request().Method,
					"url":    c.Request().URL.String(),
				},
			})

			err := next(c)

			// Add response breadcrumb
			duration := time.Since(start)
			level := statly.LevelInfo
			if c.Response().Status >= 400 {
				level = statly.LevelError
			}

			statly.AddBreadcrumb(statly.Breadcrumb{
				Message:  fmt.Sprintf("Response %d", c.Response().Status),
				Category: "http",
				Level:    level,
				Data: map[string]interface{}{
					"status_code": c.Response().Status,
					"duration_ms": float64(duration.Nanoseconds()) / 1e6,
				},
			})

			return err
		}
	}
}

// ErrorHandler returns a custom error handler that captures errors.
func ErrorHandler(defaultHandler echo.HTTPErrorHandler) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		requestInfo := extractRequestInfo(c)

		// Set tags
		statly.SetTag("http.method", c.Request().Method)
		statly.SetTag("http.url", c.Request().URL.Path)

		// Capture the error
		statly.CaptureExceptionWithContext(err, map[string]interface{}{
			"request": requestInfo,
		})

		// Call default handler
		if defaultHandler != nil {
			defaultHandler(err, c)
		}
	}
}

// extractRequestInfo extracts request information from an Echo context.
func extractRequestInfo(c echo.Context) map[string]interface{} {
	r := c.Request()

	info := map[string]interface{}{
		"method":       r.Method,
		"url":          r.URL.String(),
		"path":         r.URL.Path,
		"route":        c.Path(),
		"query_string": r.URL.RawQuery,
		"host":         r.Host,
		"remote_addr":  c.RealIP(),
	}

	// Add path parameters
	paramNames := c.ParamNames()
	if len(paramNames) > 0 {
		params := make(map[string]string)
		for _, name := range paramNames {
			params[name] = c.Param(name)
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
func SetUserFromContext(c echo.Context, user statly.User) {
	statly.SetUser(user)
}
