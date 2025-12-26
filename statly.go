// Package statly provides error tracking and monitoring for Go applications.
//
// Example usage:
//
//	import "github.com/KodyDennon/statly-go"
//
//	func main() {
//	    // Get your DSN from statly.live/dashboard/observe/setup
//	    err := statly.Init(statly.Options{
//	        DSN:         "https://sk_live_xxx@statly.live/your-org",
//	        Environment: "production",
//	        Release:     "1.0.0",
//	    })
//	    // Or auto-load DSN from STATLY_DSN environment variable
//	    err := statly.Init(statly.Options{})
//
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    defer statly.Flush()
//
//	    // Errors are captured automatically with recover middleware
//
//	    // Manual capture
//	    err := riskyOperation()
//	    if err != nil {
//	        statly.CaptureException(err)
//	    }
//
//	    // Capture a message
//	    statly.CaptureMessage("Something happened", statly.LevelWarning)
//
//	    // Set user context
//	    statly.SetUser(statly.User{ID: "user-123", Email: "user@example.com"})
//	}
package statly

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
)

// Level represents the severity level of an event.
type Level string

const (
	LevelDebug   Level = "debug"
	LevelInfo    Level = "info"
	LevelWarning Level = "warning"
	LevelError   Level = "error"
	LevelFatal   Level = "fatal"
)

// Options configures the Statly SDK.
type Options struct {
	// DSN is the Data Source Name.
	// Can be omitted if STATLY_DSN or STATLY_OBSERVE_DSN is set in environment.
	DSN string

	// Environment is the environment name (e.g., "production", "staging").
	// If not set, auto-loads from STATLY_ENVIRONMENT, GO_ENV, or APP_ENV.
	Environment string

	// Release is the release version of your application.
	Release string

	// Debug enables debug logging.
	Debug bool

	// SampleRate is the sample rate for events (0.0 to 1.0).
	SampleRate float64

	// MaxBreadcrumbs is the maximum number of breadcrumbs to store.
	MaxBreadcrumbs int

	// BeforeSend is a callback to modify or drop events before sending.
	BeforeSend func(*Event) *Event

	// Transport is a custom transport for sending events.
	Transport Transport

	// ServerName overrides the default server name.
	ServerName string

	// FlushTimeout is the timeout for flushing events on close.
	FlushTimeout time.Duration
}

// User represents user context attached to events.
type User struct {
	ID       string
	Email    string
	Username string
	IPAddr   string
	Data     map[string]interface{}
}

// Breadcrumb represents a trail event leading up to an error.
type Breadcrumb struct {
	Message   string
	Category  string
	Level     Level
	Type      string
	Data      map[string]interface{}
	Timestamp time.Time
}

var (
	globalClient *Client
	globalMu     sync.RWMutex
)

// loadDSNFromEnv loads DSN from environment variables.
func loadDSNFromEnv() string {
	if dsn := os.Getenv("STATLY_DSN"); dsn != "" {
		return dsn
	}
	return os.Getenv("STATLY_OBSERVE_DSN")
}

// loadEnvironmentFromEnv loads environment from environment variables.
func loadEnvironmentFromEnv() string {
	if env := os.Getenv("STATLY_ENVIRONMENT"); env != "" {
		return env
	}
	if env := os.Getenv("GO_ENV"); env != "" {
		return env
	}
	return os.Getenv("APP_ENV")
}

// Init initializes the Statly SDK with the given options.
// DSN can be passed explicitly or loaded from environment variables:
//   - STATLY_DSN
//   - STATLY_OBSERVE_DSN
//
// Environment can also be loaded from:
//   - STATLY_ENVIRONMENT
//   - GO_ENV
//   - APP_ENV
func Init(options Options) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalClient != nil {
		return errors.New("statly: SDK already initialized, call Close() first")
	}

	// Auto-load DSN from environment if not provided
	if options.DSN == "" {
		options.DSN = loadDSNFromEnv()
	}
	if options.DSN == "" {
		return errors.New("statly: no DSN provided. Set STATLY_DSN environment variable or pass DSN in options.\nGet your DSN at https://statly.live/dashboard/observe/setup")
	}

	// Auto-load environment from env if not provided
	if options.Environment == "" {
		options.Environment = loadEnvironmentFromEnv()
	}

	client, err := NewClient(options)
	if err != nil {
		return err
	}

	globalClient = client
	return nil
}

// CaptureException captures an error and sends it to Statly.
func CaptureException(err error) string {
	globalMu.RLock()
	client := globalClient
	globalMu.RUnlock()

	if client == nil {
		return ""
	}
	return client.CaptureException(err)
}

// CaptureExceptionWithContext captures an error with additional context.
func CaptureExceptionWithContext(err error, ctx map[string]interface{}) string {
	globalMu.RLock()
	client := globalClient
	globalMu.RUnlock()

	if client == nil {
		return ""
	}
	return client.CaptureExceptionWithContext(err, ctx)
}

// CaptureMessage captures a message and sends it to Statly.
func CaptureMessage(message string, level Level) string {
	globalMu.RLock()
	client := globalClient
	globalMu.RUnlock()

	if client == nil {
		return ""
	}
	return client.CaptureMessage(message, level)
}

// CaptureMessageWithContext captures a message with additional context.
func CaptureMessageWithContext(message string, level Level, ctx map[string]interface{}) string {
	globalMu.RLock()
	client := globalClient
	globalMu.RUnlock()

	if client == nil {
		return ""
	}
	return client.CaptureMessageWithContext(message, level, ctx)
}

// SetUser sets the current user context.
func SetUser(user User) {
	globalMu.RLock()
	client := globalClient
	globalMu.RUnlock()

	if client != nil {
		client.SetUser(user)
	}
}

// SetTag sets a tag on the current scope.
func SetTag(key, value string) {
	globalMu.RLock()
	client := globalClient
	globalMu.RUnlock()

	if client != nil {
		client.SetTag(key, value)
	}
}

// SetTags sets multiple tags on the current scope.
func SetTags(tags map[string]string) {
	globalMu.RLock()
	client := globalClient
	globalMu.RUnlock()

	if client != nil {
		client.SetTags(tags)
	}
}

// SetExtra sets extra data on the current scope.
func SetExtra(key string, value interface{}) {
	globalMu.RLock()
	client := globalClient
	globalMu.RUnlock()

	if client != nil {
		client.SetExtra(key, value)
	}
}

// AddBreadcrumb adds a breadcrumb to the current scope.
func AddBreadcrumb(crumb Breadcrumb) {
	globalMu.RLock()
	client := globalClient
	globalMu.RUnlock()

	if client != nil {
		client.AddBreadcrumb(crumb)
	}
}

// Flush flushes pending events.
func Flush() {
	globalMu.RLock()
	client := globalClient
	globalMu.RUnlock()

	if client != nil {
		client.Flush()
	}
}

// Close closes the SDK and flushes pending events.
func Close() {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalClient != nil {
		globalClient.Close()
		globalClient = nil
	}
}

// GetClient returns the current client instance.
func GetClient() *Client {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalClient
}

// Recover captures any panic that occurs and re-panics.
// Use this in a deferred function call.
func Recover() {
	if r := recover(); r != nil {
		var err error
		switch v := r.(type) {
		case error:
			err = v
		case string:
			err = errors.New(v)
		default:
			err = fmt.Errorf("%v", v)
		}

		CaptureException(err)
		Flush()
		panic(r)
	}
}

// RecoverWithContext captures any panic with additional context.
func RecoverWithContext(ctx map[string]interface{}) {
	if r := recover(); r != nil {
		var err error
		switch v := r.(type) {
		case error:
			err = v
		case string:
			err = errors.New(v)
		default:
			err = fmt.Errorf("%v", v)
		}

		CaptureExceptionWithContext(err, ctx)
		Flush()
		panic(r)
	}
}

// CurrentScope returns a new scope that can be modified independently.
func CurrentScope() *Scope {
	globalMu.RLock()
	client := globalClient
	globalMu.RUnlock()

	if client == nil {
		return NewScope()
	}
	return client.scope.Clone()
}

// WithScope executes a function with a new scope.
func WithScope(f func(*Scope)) {
	globalMu.RLock()
	client := globalClient
	globalMu.RUnlock()

	if client != nil {
		scope := client.scope.Clone()
		f(scope)
	}
}

// getHostname returns the hostname of the current machine.
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// getRuntimeInfo returns runtime information.
func getRuntimeInfo() map[string]interface{} {
	return map[string]interface{}{
		"name":    "Go",
		"version": runtime.Version(),
		"arch":    runtime.GOARCH,
		"os":      runtime.GOOS,
		"cpus":    runtime.NumCPU(),
	}
}
