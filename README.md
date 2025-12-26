# Statly Observe SDK for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/KodyDennon/statly-go.svg)](https://pkg.go.dev/github.com/KodyDennon/statly-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/KodyDennon/statly-go)](https://goreportcard.com/report/github.com/KodyDennon/statly-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Error tracking and monitoring for Go applications. Capture panics and errors, track releases, and debug issues faster.

**[📚 Full Documentation](https://docs.statly.live/sdk/go/installation)** | **[🚀 Get Started](https://statly.live)** | **[💬 Support](mailto:support@mail.kodydennon.com)**

> **This SDK requires a [Statly](https://statly.live) account.** Sign up free at [statly.live](https://statly.live) to get your DSN and start tracking errors in minutes.

## Features

- Automatic panic recovery with stack traces
- Error capturing with context
- Breadcrumbs for debugging
- User context tracking
- Release tracking
- Framework integrations (Gin, Echo, Chi, net/http)
- Goroutine-safe
- Minimal overhead

## Installation

```bash
go get github.com/KodyDennon/statly-go
```

## Getting Your DSN

1. Go to [statly.live/dashboard/observe/setup](https://statly.live/dashboard/observe/setup)
2. Create an API key for Observe
3. Copy your DSN (format: `https://<api-key>@statly.live/<org-slug>`)
4. Add to your environment: `export STATLY_DSN=https://...`

## Quick Start

The SDK automatically loads DSN from environment variables, so you can simply:

```go
import statly "github.com/KodyDennon/statly-go"

func main() {
    // Auto-loads STATLY_DSN from environment
    err := statly.Init(statly.Options{})
    if err != nil {
        log.Fatal(err)
    }
    defer statly.Close()
}
```

Or pass it explicitly:

```go
package main

import (
    "log"

    statly "github.com/KodyDennon/statly-go"
)

func main() {
    // Initialize the SDK
    err := statly.Init(statly.Options{
        DSN:         "https://sk_live_xxx@statly.live/your-org",
        Environment: "production",
        Release:     "1.0.0",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer statly.Close()

    // Recover panics in main goroutine
    defer statly.Recover()

    // Manual error capture
    if err := riskyOperation(); err != nil {
        statly.CaptureException(err)
    }

    // Capture a message
    statly.CaptureMessage("User completed checkout", statly.LevelInfo)

    // Set user context
    statly.SetUser(statly.User{
        ID:    "user-123",
        Email: "user@example.com",
    })

    // Add breadcrumb for debugging
    statly.AddBreadcrumb(statly.Breadcrumb{
        Message:  "User logged in",
        Category: "auth",
        Level:    statly.LevelInfo,
    })
}
```

## Framework Integrations

### net/http (Standard Library)

```go
package main

import (
    "net/http"

    statly "github.com/KodyDennon/statly-go"
    "github.com/KodyDennon/statly-go/middleware"
)

func main() {
    statly.Init(statly.Options{
        DSN:         "https://sk_live_xxx@statly.live/your-org",
        Environment: "production",
    })
    defer statly.Close()

    mux := http.NewServeMux()

    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Hello World"))
    })

    mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
        panic("test panic") // Automatically captured
    })

    // Wrap with recovery and request logging middleware
    handler := middleware.Recovery(middleware.DefaultOptions())(
        middleware.RequestLogger()(mux),
    )

    http.ListenAndServe(":8080", handler)
}
```

### Gin

```go
package main

import (
    "github.com/gin-gonic/gin"
    statly "github.com/KodyDennon/statly-go"
    statlygin "github.com/KodyDennon/statly-go/integrations/gin"
)

func main() {
    statly.Init(statly.Options{
        DSN:         "https://sk_live_xxx@statly.live/your-org",
        Environment: "production",
    })
    defer statly.Close()

    r := gin.New()

    // Add Statly middleware (order matters!)
    r.Use(statlygin.Logger())                             // Request logging
    r.Use(statlygin.Recovery(statlygin.DefaultOptions())) // Panic recovery
    r.Use(statlygin.ErrorHandler())                       // Error capture

    r.GET("/", func(c *gin.Context) {
        c.JSON(200, gin.H{"message": "Hello World"})
    })

    r.GET("/error", func(c *gin.Context) {
        panic("test panic") // Automatically captured
    })

    r.Run(":8080")
}
```

### Echo

```go
package main

import (
    "github.com/labstack/echo/v4"
    statly "github.com/KodyDennon/statly-go"
    statlyecho "github.com/KodyDennon/statly-go/integrations/echo"
)

func main() {
    statly.Init(statly.Options{
        DSN:         "https://sk_live_xxx@statly.live/your-org",
        Environment: "production",
    })
    defer statly.Close()

    e := echo.New()

    // Add Statly middleware
    e.Use(statlyecho.Logger())
    e.Use(statlyecho.Recovery(statlyecho.DefaultOptions()))

    // Custom error handler for better error capture
    e.HTTPErrorHandler = statlyecho.ErrorHandler(e.DefaultHTTPErrorHandler)

    e.GET("/", func(c echo.Context) error {
        return c.JSON(200, map[string]string{"message": "Hello World"})
    })

    e.Start(":8080")
}
```

### Chi

```go
package main

import (
    "net/http"

    "github.com/go-chi/chi/v5"
    statly "github.com/KodyDennon/statly-go"
    statlychi "github.com/KodyDennon/statly-go/integrations/chi"
)

func main() {
    statly.Init(statly.Options{
        DSN:         "https://sk_live_xxx@statly.live/your-org",
        Environment: "production",
    })
    defer statly.Close()

    r := chi.NewRouter()

    // Add Statly middleware
    r.Use(statlychi.Logger)
    r.Use(statlychi.Recoverer)

    r.Get("/", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Hello World"))
    })

    http.ListenAndServe(":8080", r)
}
```

## Environment Variables

The SDK automatically loads configuration from environment variables:

| Variable | Description |
|----------|-------------|
| `STATLY_DSN` | Your project's DSN (primary) |
| `STATLY_OBSERVE_DSN` | Alternative DSN variable |
| `STATLY_ENVIRONMENT` | Environment name |
| `GO_ENV` or `APP_ENV` | Fallback for environment |

## Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `DSN` | `string` | `os.Getenv("STATLY_DSN")` | Your project's Data Source Name |
| `Environment` | `string` | `""` | Environment name (production, staging, development) |
| `Release` | `string` | `""` | Release/version identifier for tracking |
| `Debug` | `bool` | `false` | Enable debug logging to stderr |
| `SampleRate` | `float64` | `1.0` | Sample rate for events (0.0 to 1.0) |
| `MaxBreadcrumbs` | `int` | `100` | Maximum breadcrumbs to store |
| `BeforeSend` | `func(*Event) *Event` | `nil` | Callback to modify/filter events |
| `FlushTimeout` | `time.Duration` | `5s` | Timeout for flushing events on close |

### BeforeSend Example

```go
statly.Init(statly.Options{
    DSN: "...",
    BeforeSend: func(event *statly.Event) *statly.Event {
        // Filter out specific errors
        if strings.Contains(event.Message, "context canceled") {
            return nil // Drop the event
        }

        // Scrub sensitive data
        delete(event.Extra, "password")
        delete(event.Extra, "api_key")

        return event
    },
})
```

## API Reference

### statly.CaptureException(err error, contexts ...map[string]interface{})

Capture an error with optional additional context:

```go
if err := processPayment(order); err != nil {
    statly.CaptureException(err, map[string]interface{}{
        "order_id": order.ID,
        "amount":   order.Total,
        "tags": map[string]string{
            "payment_provider": "stripe",
        },
    })
}
```

### statly.CaptureMessage(message string, level Level)

Capture a message event:

```go
statly.CaptureMessage("User signed up", statly.LevelInfo)
statly.CaptureMessage("Payment failed after 3 retries", statly.LevelWarning)
statly.CaptureMessage("Database connection lost", statly.LevelError)
```

Levels: `LevelDebug` | `LevelInfo` | `LevelWarning` | `LevelError` | `LevelFatal`

### statly.SetUser(user User)

Set user context for all subsequent events:

```go
statly.SetUser(statly.User{
    ID:       "user-123",
    Email:    "user@example.com",
    Username: "johndoe",
    IPAddr:   "192.168.1.1",
    Data: map[string]interface{}{
        "subscription": "premium",
    },
})

// Clear user on logout
statly.SetUser(statly.User{})
```

### statly.SetTag(key, value string) / statly.SetTags(tags map[string]string)

Set tags for filtering and searching:

```go
statly.SetTag("version", "1.0.0")

statly.SetTags(map[string]string{
    "environment": "production",
    "server":      "web-1",
    "region":      "us-east-1",
})
```

### statly.AddBreadcrumb(breadcrumb Breadcrumb)

Add a breadcrumb for debugging context:

```go
statly.AddBreadcrumb(statly.Breadcrumb{
    Message:  "User clicked checkout button",
    Category: "ui.click",
    Level:    statly.LevelInfo,
    Data: map[string]interface{}{
        "button_id":  "checkout-btn",
        "cart_items": 3,
    },
})
```

### statly.Flush() / statly.Close()

```go
// Flush pending events (keeps SDK running)
statly.Flush()

// Flush and close (use before process exit)
statly.Close()
```

## Panic Recovery

### In Main Goroutine

```go
func main() {
    statly.Init(statly.Options{DSN: "..."})
    defer statly.Close()
    defer statly.Recover() // Captures panics

    // Your code
}
```

### In Goroutines

```go
go func() {
    defer statly.Recover() // Must be in each goroutine

    // This panic will be captured
    panic("something went wrong")
}()
```

### With Additional Context

```go
go func() {
    defer statly.RecoverWithContext(map[string]interface{}{
        "goroutine": "worker",
        "job_id":    "123",
    })

    doWork()
}()
```

## Scopes

Use scopes for temporary context:

```go
// Get current scope
scope := statly.CurrentScope()

// Work with a temporary scope
statly.WithScope(func(scope *statly.Scope) {
    scope.SetTag("operation", "batch-import")
    scope.SetUser(statly.User{ID: "batch-user"})

    // Events captured here will have this scope
    statly.CaptureMessage("Batch import started", statly.LevelInfo)
})
// Scope is automatically restored after the function
```

## HTTP Client Integration

Capture errors from HTTP clients:

```go
import "github.com/KodyDennon/statly-go/httpclient"

// Wrap your HTTP client
client := httpclient.Wrap(http.DefaultClient)

resp, err := client.Get("https://api.example.com/data")
if err != nil {
    // Error is automatically captured with request context
}
```

## gRPC Integration

```go
import (
    "google.golang.org/grpc"
    statlygrpc "github.com/KodyDennon/statly-go/integrations/grpc"
)

// Server interceptors
server := grpc.NewServer(
    grpc.UnaryInterceptor(statlygrpc.UnaryServerInterceptor()),
    grpc.StreamInterceptor(statlygrpc.StreamServerInterceptor()),
)

// Client interceptors
conn, err := grpc.Dial(
    "localhost:50051",
    grpc.WithUnaryInterceptor(statlygrpc.UnaryClientInterceptor()),
    grpc.WithStreamInterceptor(statlygrpc.StreamClientInterceptor()),
)
```

## Requirements

- Go 1.18+
- Works with any Go HTTP framework

## Resources

- **[Statly Platform](https://statly.live)** - Sign up and manage your error tracking
- **[Documentation](https://docs.statly.live/sdk/go/installation)** - Full SDK documentation
- **[API Reference](https://docs.statly.live/sdk/go/api-reference)** - Complete API reference
- **[Gin Guide](https://docs.statly.live/sdk/go/gin)** - Gin integration
- **[Echo Guide](https://docs.statly.live/sdk/go/echo)** - Echo integration
- **[MCP Server](https://github.com/KodyDennon/DD-StatusPage/tree/master/packages/mcp-docs-server)** - AI/Claude integration for docs

## Why Statly?

Statly is more than error tracking. Get:
- **Status Pages** - Beautiful public status pages for your users
- **Uptime Monitoring** - Multi-region HTTP/DNS checks every minute
- **Error Tracking** - SDKs for JavaScript, Python, and Go
- **Incident Management** - Track and communicate outages

All on Cloudflare's global edge network. [Start free →](https://statly.live)

## License

MIT
