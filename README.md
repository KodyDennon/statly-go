# Statly Observe SDK for Go

Error tracking and monitoring for Go applications.

## Installation

```bash
go get github.com/KodyDennon/statly-go
```

## Quick Start

```go
package main

import (
    "log"

    "github.com/KodyDennon/statly-go"
)

func main() {
    err := statly.Init(statly.Options{
        DSN:         "https://observe.statly.live/your-org",
        Environment: "production",
        Release:     "1.0.0",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer statly.Close()

    // Use recover in goroutines
    defer statly.Recover()

    // Manual capture
    if err := riskyOperation(); err != nil {
        statly.CaptureException(err)
    }

    // Capture a message
    statly.CaptureMessage("Something happened", statly.LevelWarning)

    // Set user context
    statly.SetUser(statly.User{
        ID:    "user-123",
        Email: "user@example.com",
    })

    // Add breadcrumb
    statly.AddBreadcrumb(statly.Breadcrumb{
        Message:  "User logged in",
        Category: "auth",
        Level:    statly.LevelInfo,
    })
}
```

## Standard Library HTTP Middleware

```go
package main

import (
    "net/http"

    "github.com/KodyDennon/statly-go"
    "github.com/KodyDennon/statly-go/middleware"
)

func main() {
    statly.Init(statly.Options{DSN: "..."})
    defer statly.Close()

    mux := http.NewServeMux()
    mux.HandleFunc("/", handler)

    // Add recovery and logging middleware
    handler := middleware.Recovery(middleware.DefaultOptions())(
        middleware.RequestLogger()(mux),
    )

    http.ListenAndServe(":8080", handler)
}
```

## Gin Integration

```go
package main

import (
    "github.com/gin-gonic/gin"
    "github.com/KodyDennon/statly-go"
    statlygin "github.com/KodyDennon/statly-go/integrations/gin"
)

func main() {
    statly.Init(statly.Options{DSN: "..."})
    defer statly.Close()

    r := gin.New()

    // Add Statly middleware
    r.Use(statlygin.Logger())
    r.Use(statlygin.Recovery(statlygin.DefaultOptions()))
    r.Use(statlygin.ErrorHandler())

    r.GET("/", func(c *gin.Context) {
        c.JSON(200, gin.H{"message": "Hello World"})
    })

    r.Run(":8080")
}
```

## Echo Integration

```go
package main

import (
    "github.com/labstack/echo/v4"
    "github.com/KodyDennon/statly-go"
    statlyecho "github.com/KodyDennon/statly-go/integrations/echo"
)

func main() {
    statly.Init(statly.Options{DSN: "..."})
    defer statly.Close()

    e := echo.New()

    // Add Statly middleware
    e.Use(statlyecho.Logger())
    e.Use(statlyecho.Recovery(statlyecho.DefaultOptions()))

    // Custom error handler
    e.HTTPErrorHandler = statlyecho.ErrorHandler(e.DefaultHTTPErrorHandler)

    e.GET("/", func(c echo.Context) error {
        return c.JSON(200, map[string]string{"message": "Hello World"})
    })

    e.Start(":8080")
}
```

## Configuration

### statly.Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `DSN` | `string` | Required | Data Source Name for your project |
| `Environment` | `string` | `""` | Environment name (production, staging, etc.) |
| `Release` | `string` | `""` | Release version of your application |
| `Debug` | `bool` | `false` | Enable debug logging |
| `SampleRate` | `float64` | `1.0` | Sample rate for events (0.0 to 1.0) |
| `MaxBreadcrumbs` | `int` | `100` | Maximum breadcrumbs to store |
| `BeforeSend` | `func(*Event) *Event` | `nil` | Callback to modify/filter events |
| `FlushTimeout` | `time.Duration` | `5s` | Timeout for flushing events |

### Before Send Callback

```go
statly.Init(statly.Options{
    DSN: "...",
    BeforeSend: func(event *statly.Event) *statly.Event {
        // Remove sensitive data
        delete(event.Extra, "password")
        return event

        // Or drop the event
        // return nil
    },
})
```

## Breadcrumbs

```go
// Default breadcrumb
statly.AddBreadcrumb(statly.Breadcrumb{
    Message: "User logged in",
})

// With category and data
statly.AddBreadcrumb(statly.Breadcrumb{
    Message:  "Database query",
    Category: "query",
    Level:    statly.LevelInfo,
    Data: map[string]interface{}{
        "query":       "SELECT * FROM users",
        "duration_ms": 15,
    },
})
```

## User Context

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
```

## Tags

```go
// Single tag
statly.SetTag("version", "1.0.0")

// Multiple tags
statly.SetTags(map[string]string{
    "environment": "production",
    "server":      "web-1",
})
```

## Panic Recovery

Use `statly.Recover()` in goroutines to capture panics:

```go
go func() {
    defer statly.Recover()

    // This panic will be captured
    panic("something went wrong")
}()
```

With additional context:

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
```

## License

MIT
