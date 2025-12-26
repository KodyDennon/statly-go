package statly

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"time"
)

// Event represents a Statly event.
type Event struct {
	EventID     string                 `json:"event_id"`
	Timestamp   time.Time              `json:"timestamp"`
	Level       Level                  `json:"level"`
	Platform    string                 `json:"platform"`
	Message     string                 `json:"message,omitempty"`
	Exception   []ExceptionValue       `json:"exception,omitempty"`
	Contexts    map[string]interface{} `json:"contexts,omitempty"`
	Tags        map[string]string      `json:"tags,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
	User        *EventUser             `json:"user,omitempty"`
	Breadcrumbs []BreadcrumbValue      `json:"breadcrumbs,omitempty"`
	SDK         SDKInfo                `json:"sdk"`
	Environment string                 `json:"environment,omitempty"`
	Release     string                 `json:"release,omitempty"`
	ServerName  string                 `json:"server_name,omitempty"`
	Request     *RequestInfo           `json:"request,omitempty"`
}

// ExceptionValue represents an exception in an event.
type ExceptionValue struct {
	Type       string      `json:"type"`
	Value      string      `json:"value"`
	Module     string      `json:"module,omitempty"`
	Stacktrace *Stacktrace `json:"stacktrace,omitempty"`
	Mechanism  *Mechanism  `json:"mechanism,omitempty"`
}

// Stacktrace represents a stack trace.
type Stacktrace struct {
	Frames []StackFrame `json:"frames"`
}

// StackFrame represents a single frame in a stack trace.
type StackFrame struct {
	Filename    string                 `json:"filename"`
	Function    string                 `json:"function"`
	Lineno      int                    `json:"lineno,omitempty"`
	Colno       int                    `json:"colno,omitempty"`
	AbsPath     string                 `json:"abs_path,omitempty"`
	ContextLine string                 `json:"context_line,omitempty"`
	InApp       bool                   `json:"in_app"`
	Vars        map[string]interface{} `json:"vars,omitempty"`
}

// Mechanism describes the mechanism that generated the exception.
type Mechanism struct {
	Type    string `json:"type"`
	Handled bool   `json:"handled"`
}

// EventUser represents user information in an event.
type EventUser struct {
	ID       string                 `json:"id,omitempty"`
	Email    string                 `json:"email,omitempty"`
	Username string                 `json:"username,omitempty"`
	IPAddr   string                 `json:"ip_address,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"`
}

// BreadcrumbValue represents a breadcrumb in an event.
type BreadcrumbValue struct {
	Message   string                 `json:"message"`
	Category  string                 `json:"category,omitempty"`
	Level     Level                  `json:"level"`
	Type      string                 `json:"type"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp string                 `json:"timestamp"`
}

// SDKInfo contains SDK metadata.
type SDKInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// RequestInfo contains HTTP request information.
type RequestInfo struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	QueryString string            `json:"query_string,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Data        interface{}       `json:"data,omitempty"`
	Cookies     string            `json:"cookies,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
}

// SDK version
const Version = "0.1.0"

// generateEventID generates a unique event ID.
func generateEventID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// NewEvent creates a new event with default values.
func NewEvent() *Event {
	return &Event{
		EventID:   generateEventID(),
		Timestamp: time.Now().UTC(),
		Level:     LevelError,
		Platform:  "go",
		Contexts:  make(map[string]interface{}),
		Tags:      make(map[string]string),
		Extra:     make(map[string]interface{}),
		SDK: SDKInfo{
			Name:    "statly-observe-go",
			Version: Version,
		},
	}
}

// NewExceptionEvent creates a new event from an error.
func NewExceptionEvent(err error) *Event {
	event := NewEvent()
	event.Level = LevelError

	// Extract exception info
	exc := ExceptionValue{
		Type:  getErrorType(err),
		Value: err.Error(),
		Mechanism: &Mechanism{
			Type:    "generic",
			Handled: true,
		},
	}

	// Get stack trace
	exc.Stacktrace = captureStacktrace(3) // Skip this function and callers

	event.Exception = []ExceptionValue{exc}
	return event
}

// NewMessageEvent creates a new event from a message.
func NewMessageEvent(message string, level Level) *Event {
	event := NewEvent()
	event.Message = message
	event.Level = level
	return event
}

// getErrorType returns the type name of an error.
func getErrorType(err error) string {
	if err == nil {
		return "unknown"
	}

	// Try to unwrap to get underlying type
	var unwrapped error = err
	for {
		if u := errors.Unwrap(unwrapped); u != nil {
			unwrapped = u
		} else {
			break
		}
	}

	return fmt.Sprintf("%T", unwrapped)
}

// captureStacktrace captures the current stack trace.
func captureStacktrace(skip int) *Stacktrace {
	var frames []StackFrame

	// Get up to 50 frames
	pcs := make([]uintptr, 50)
	n := runtime.Callers(skip+1, pcs)
	pcs = pcs[:n]

	runtimeFrames := runtime.CallersFrames(pcs)

	for {
		frame, more := runtimeFrames.Next()

		// Skip runtime frames
		if frame.Function == "" {
			if !more {
				break
			}
			continue
		}

		// Determine if this is in-app code
		inApp := !isStandardLibrary(frame.Function)

		frames = append(frames, StackFrame{
			Filename: frame.File,
			Function: frame.Function,
			Lineno:   frame.Line,
			AbsPath:  frame.File,
			InApp:    inApp,
		})

		if !more {
			break
		}
	}

	// Reverse frames so innermost is first
	for i, j := 0, len(frames)-1; i < j; i, j = i+1, j-1 {
		frames[i], frames[j] = frames[j], frames[i]
	}

	return &Stacktrace{Frames: frames}
}

// isStandardLibrary checks if a function is from the Go standard library.
func isStandardLibrary(function string) bool {
	// Standard library functions typically start with common prefixes
	prefixes := []string{
		"runtime.",
		"reflect.",
		"sync.",
		"net/",
		"os.",
		"io.",
		"fmt.",
		"encoding/",
		"strings.",
		"bytes.",
		"bufio.",
		"context.",
		"database/",
		"crypto/",
		"compress/",
		"archive/",
		"time.",
		"math/",
		"testing.",
	}

	for _, prefix := range prefixes {
		if len(function) >= len(prefix) && function[:len(prefix)] == prefix {
			return true
		}
	}

	return false
}
