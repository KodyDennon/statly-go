// Package logger provides a comprehensive logging framework for Go applications
// with multi-destination output, secret scrubbing, sampling, and AI-powered analysis.
//
// Example usage:
//
//	import "github.com/KodyDennon/statly-go/logger"
//
//	func main() {
//	    log := logger.New(logger.Config{
//	        DSN:         "https://sk_live_xxx@statly.live/your-org",
//	        Name:        "my-app",
//	        Environment: "production",
//	    })
//	    defer log.Close()
//
//	    log.Info("Application started", nil)
//	    log.Error("Something went wrong", map[string]interface{}{"error": "details"})
//
//	    // AI-powered analysis
//	    explanation, _ := log.ExplainError(err)
//	}
package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Level represents the severity level of a log entry.
type Level int

const (
	LevelTrace Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
	LevelAudit // Special: always logged, never sampled
)

var levelNames = map[Level]string{
	LevelTrace: "trace",
	LevelDebug: "debug",
	LevelInfo:  "info",
	LevelWarn:  "warn",
	LevelError: "error",
	LevelFatal: "fatal",
	LevelAudit: "audit",
}

var levelFromName = map[string]Level{
	"trace": LevelTrace,
	"debug": LevelDebug,
	"info":  LevelInfo,
	"warn":  LevelWarn,
	"error": LevelError,
	"fatal": LevelFatal,
	"audit": LevelAudit,
}

// Entry represents a single log entry.
type Entry struct {
	Level       Level                  `json:"level"`
	Message     string                 `json:"message"`
	Timestamp   time.Time              `json:"timestamp"`
	LoggerName  string                 `json:"loggerName,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
	Tags        map[string]string      `json:"tags,omitempty"`
	Source      *Source                `json:"source,omitempty"`
	TraceID     string                 `json:"traceId,omitempty"`
	SpanID      string                 `json:"spanId,omitempty"`
	SessionID   string                 `json:"sessionId,omitempty"`
	Environment string                 `json:"environment,omitempty"`
	Release     string                 `json:"release,omitempty"`
	SDKName     string                 `json:"sdkName"`
	SDKVersion  string                 `json:"sdkVersion"`
}

// Source represents source code location.
type Source struct {
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Function string `json:"function,omitempty"`
}

// ToMap converts an entry to a map for JSON serialization.
func (e *Entry) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"level":       levelNames[e.Level],
		"message":     e.Message,
		"timestamp":   e.Timestamp.UnixMilli(),
		"loggerName":  e.LoggerName,
		"context":     e.Context,
		"tags":        e.Tags,
		"source":      e.Source,
		"traceId":     e.TraceID,
		"spanId":      e.SpanID,
		"sessionId":   e.SessionID,
		"environment": e.Environment,
		"release":     e.Release,
		"sdkName":     e.SDKName,
		"sdkVersion":  e.SDKVersion,
	}
}

// Config configures the logger.
type Config struct {
	DSN         string
	Level       Level
	Name        string
	Environment string
	Release     string
	Console     *ConsoleConfig
	File        *FileConfig
	Observe     *ObserveConfig
	Scrubbing   *ScrubbingConfig
	Context     map[string]interface{}
	Tags        map[string]string
}

// ConsoleConfig configures the console destination.
type ConsoleConfig struct {
	Enabled    bool
	Colors     bool
	Format     string // "pretty" or "json"
	Timestamps bool
	Output     io.Writer
}

// FileConfig configures the file destination.
type FileConfig struct {
	Enabled          bool
	Path             string
	Format           string // "json" or "text"
	RotationType     string // "size" or "time"
	MaxSize          string // e.g., "10MB"
	MaxFiles         int
	RotationInterval string // "hourly", "daily", "weekly"
	RetentionDays    int
	Compress         bool
}

// ObserveConfig configures the Observe destination.
type ObserveConfig struct {
	Enabled       bool
	BatchSize     int
	FlushInterval time.Duration
	Sampling      map[Level]float64
}

// ScrubbingConfig configures secret scrubbing.
type ScrubbingConfig struct {
	Enabled       bool
	Patterns      []string
	CustomRegexps []*regexp.Regexp
	Allowlist     []string
}

// Destination is the interface for log destinations.
type Destination interface {
	Name() string
	Write(entry *Entry)
	Flush()
	Close()
}

// Logger is the main logger type.
type Logger struct {
	name         string
	config       Config
	minLevel     Level
	destinations []Destination
	scrubber     *Scrubber
	context      map[string]interface{}
	tags         map[string]string
	sessionID    string
	traceID      string
	spanID       string
	mu           sync.RWMutex
}

// New creates a new logger with the given configuration.
func New(config Config) *Logger {
	// Set defaults
	if config.Level == 0 {
		config.Level = LevelDebug
	}
	if config.Name == "" {
		config.Name = "default"
	}

	// Auto-load DSN from environment
	if config.DSN == "" {
		config.DSN = os.Getenv("STATLY_DSN")
	}

	// Auto-load environment from environment
	if config.Environment == "" {
		config.Environment = os.Getenv("STATLY_ENVIRONMENT")
		if config.Environment == "" {
			config.Environment = os.Getenv("GO_ENV")
		}
	}

	logger := &Logger{
		name:      config.Name,
		config:    config,
		minLevel:  config.Level,
		context:   make(map[string]interface{}),
		tags:      make(map[string]string),
		sessionID: uuid.New().String(),
	}

	// Copy initial context and tags
	if config.Context != nil {
		for k, v := range config.Context {
			logger.context[k] = v
		}
	}
	if config.Tags != nil {
		for k, v := range config.Tags {
			logger.tags[k] = v
		}
	}

	// Initialize scrubber
	logger.scrubber = NewScrubber(config.Scrubbing)

	// Initialize destinations
	logger.initDestinations()

	return logger
}

func (l *Logger) initDestinations() {
	// Console destination (default enabled)
	consoleConfig := l.config.Console
	if consoleConfig == nil {
		consoleConfig = &ConsoleConfig{Enabled: true, Colors: true, Format: "pretty", Timestamps: true}
	}
	if consoleConfig.Enabled {
		l.destinations = append(l.destinations, NewConsoleDestination(consoleConfig))
	}

	// File destination
	if l.config.File != nil && l.config.File.Enabled {
		l.destinations = append(l.destinations, NewFileDestination(l.config.File))
	}

	// Observe destination
	if l.config.DSN != "" {
		observeConfig := l.config.Observe
		if observeConfig == nil {
			observeConfig = &ObserveConfig{Enabled: true, BatchSize: 50, FlushInterval: 5 * time.Second}
		}
		if observeConfig.Enabled {
			l.destinations = append(l.destinations, NewObserveDestination(l.config.DSN, observeConfig))
		}
	}
}

func (l *Logger) shouldLog(level Level) bool {
	if level == LevelAudit {
		return true
	}
	return level >= l.minLevel
}

func (l *Logger) getSource(skip int) *Source {
	pc, file, line, ok := runtime.Caller(skip)
	if !ok {
		return nil
	}

	fn := runtime.FuncForPC(pc)
	funcName := ""
	if fn != nil {
		funcName = fn.Name()
		// Get just the function name, not the full path
		if idx := strings.LastIndex(funcName, "."); idx >= 0 {
			funcName = funcName[idx+1:]
		}
	}

	return &Source{
		File:     file,
		Line:     line,
		Function: funcName,
	}
}

func (l *Logger) createEntry(level Level, message string, ctx map[string]interface{}) *Entry {
	l.mu.RLock()
	mergedContext := make(map[string]interface{})
	for k, v := range l.context {
		mergedContext[k] = v
	}
	for k, v := range ctx {
		mergedContext[k] = v
	}
	tags := make(map[string]string)
	for k, v := range l.tags {
		tags[k] = v
	}
	traceID := l.traceID
	spanID := l.spanID
	l.mu.RUnlock()

	// Scrub message and context
	scrubbedMessage := l.scrubber.ScrubString(message)
	scrubbedContext := l.scrubber.Scrub(mergedContext).(map[string]interface{})

	return &Entry{
		Level:       level,
		Message:     scrubbedMessage,
		Timestamp:   time.Now(),
		LoggerName:  l.name,
		Context:     scrubbedContext,
		Tags:        tags,
		Source:      l.getSource(4),
		TraceID:     traceID,
		SpanID:      spanID,
		SessionID:   l.sessionID,
		Environment: l.config.Environment,
		Release:     l.config.Release,
		SDKName:     "statly-observe-go",
		SDKVersion:  "0.2.0",
	}
}

func (l *Logger) write(entry *Entry) {
	for _, dest := range l.destinations {
		dest.Write(entry)
	}
}

// Logging methods

// Trace logs a trace message.
func (l *Logger) Trace(message string, ctx map[string]interface{}) {
	if !l.shouldLog(LevelTrace) {
		return
	}
	l.write(l.createEntry(LevelTrace, message, ctx))
}

// Debug logs a debug message.
func (l *Logger) Debug(message string, ctx map[string]interface{}) {
	if !l.shouldLog(LevelDebug) {
		return
	}
	l.write(l.createEntry(LevelDebug, message, ctx))
}

// Info logs an info message.
func (l *Logger) Info(message string, ctx map[string]interface{}) {
	if !l.shouldLog(LevelInfo) {
		return
	}
	l.write(l.createEntry(LevelInfo, message, ctx))
}

// Warn logs a warning message.
func (l *Logger) Warn(message string, ctx map[string]interface{}) {
	if !l.shouldLog(LevelWarn) {
		return
	}
	l.write(l.createEntry(LevelWarn, message, ctx))
}

// Error logs an error message.
func (l *Logger) Error(message string, ctx map[string]interface{}) {
	if !l.shouldLog(LevelError) {
		return
	}
	l.write(l.createEntry(LevelError, message, ctx))
}

// ErrorErr logs an error from an error value.
func (l *Logger) ErrorErr(err error, ctx map[string]interface{}) {
	if !l.shouldLog(LevelError) {
		return
	}
	if ctx == nil {
		ctx = make(map[string]interface{})
	}
	ctx["errorType"] = fmt.Sprintf("%T", err)
	l.write(l.createEntry(LevelError, err.Error(), ctx))
}

// Fatal logs a fatal message.
func (l *Logger) Fatal(message string, ctx map[string]interface{}) {
	if !l.shouldLog(LevelFatal) {
		return
	}
	l.write(l.createEntry(LevelFatal, message, ctx))
}

// FatalErr logs a fatal error from an error value.
func (l *Logger) FatalErr(err error, ctx map[string]interface{}) {
	if !l.shouldLog(LevelFatal) {
		return
	}
	if ctx == nil {
		ctx = make(map[string]interface{})
	}
	ctx["errorType"] = fmt.Sprintf("%T", err)
	l.write(l.createEntry(LevelFatal, err.Error(), ctx))
}

// Audit logs an audit message (always logged, never sampled).
func (l *Logger) Audit(message string, ctx map[string]interface{}) {
	l.write(l.createEntry(LevelAudit, message, ctx))
}

// Log logs at a specific level.
func (l *Logger) Log(level Level, message string, ctx map[string]interface{}) {
	if !l.shouldLog(level) {
		return
	}
	l.write(l.createEntry(level, message, ctx))
}

// Context and tags

// SetContext sets persistent context.
func (l *Logger) SetContext(ctx map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, v := range ctx {
		l.context[k] = v
	}
}

// ClearContext clears all context.
func (l *Logger) ClearContext() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.context = make(map[string]interface{})
}

// SetTag sets a tag.
func (l *Logger) SetTag(key, value string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tags[key] = value
}

// SetTags sets multiple tags.
func (l *Logger) SetTags(tags map[string]string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, v := range tags {
		l.tags[k] = v
	}
}

// ClearTags clears all tags.
func (l *Logger) ClearTags() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tags = make(map[string]string)
}

// Tracing

// SetTraceID sets the trace ID for distributed tracing.
func (l *Logger) SetTraceID(traceID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.traceID = traceID
}

// SetSpanID sets the span ID.
func (l *Logger) SetSpanID(spanID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.spanID = spanID
}

// ClearTracing clears tracing context.
func (l *Logger) ClearTracing() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.traceID = ""
	l.spanID = ""
}

// Child loggers

// Child creates a child logger with additional context.
func (l *Logger) Child(name string, ctx map[string]interface{}, tags map[string]string) *Logger {
	l.mu.RLock()
	mergedContext := make(map[string]interface{})
	for k, v := range l.context {
		mergedContext[k] = v
	}
	for k, v := range ctx {
		mergedContext[k] = v
	}
	mergedTags := make(map[string]string)
	for k, v := range l.tags {
		mergedTags[k] = v
	}
	for k, v := range tags {
		mergedTags[k] = v
	}
	l.mu.RUnlock()

	if name == "" {
		name = l.name + ".child"
	}

	child := &Logger{
		name:         name,
		config:       l.config,
		minLevel:     l.minLevel,
		destinations: l.destinations, // Share destinations
		scrubber:     l.scrubber,
		context:      mergedContext,
		tags:         mergedTags,
		sessionID:    l.sessionID,
		traceID:      l.traceID,
		spanID:       l.spanID,
	}

	return child
}

// Level management

// SetLevel sets the minimum log level.
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevel = level
}

// GetLevel returns the current minimum level.
func (l *Logger) GetLevel() Level {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.minLevel
}

// IsLevelEnabled checks if a level is enabled.
func (l *Logger) IsLevelEnabled(level Level) bool {
	return l.shouldLog(level)
}

// Destination management

// AddDestination adds a custom destination.
func (l *Logger) AddDestination(dest Destination) {
	l.destinations = append(l.destinations, dest)
}

// RemoveDestination removes a destination by name.
func (l *Logger) RemoveDestination(name string) {
	filtered := make([]Destination, 0, len(l.destinations))
	for _, d := range l.destinations {
		if d.Name() != name {
			filtered = append(filtered, d)
		}
	}
	l.destinations = filtered
}

// Lifecycle

// Flush flushes all destinations.
func (l *Logger) Flush() {
	for _, dest := range l.destinations {
		dest.Flush()
	}
}

// Close closes the logger and all destinations.
func (l *Logger) Close() {
	for _, dest := range l.destinations {
		dest.Close()
	}
}

// GetName returns the logger name.
func (l *Logger) GetName() string {
	return l.name
}

// GetSessionID returns the session ID.
func (l *Logger) GetSessionID() string {
	return l.sessionID
}

// AI Features

// ErrorExplanation represents an AI-powered error explanation.
type ErrorExplanation struct {
	Summary        string   `json:"summary"`
	PossibleCauses []string `json:"possibleCauses"`
	StackAnalysis  string   `json:"stackAnalysis,omitempty"`
	RelatedDocs    []string `json:"relatedDocs,omitempty"`
}

// FixSuggestion represents an AI-powered fix suggestion.
type FixSuggestion struct {
	Summary        string                   `json:"summary"`
	SuggestedFixes []map[string]interface{} `json:"suggestedFixes"`
	PreventionTips []string                 `json:"preventionTips,omitempty"`
}

// ExplainError gets an AI explanation for an error.
func (l *Logger) ExplainError(err error, apiKey string) (*ErrorExplanation, error) {
	if l.config.DSN == "" {
		return &ErrorExplanation{
			Summary:        "AI features not available (no DSN configured)",
			PossibleCauses: []string{},
		}, nil
	}

	endpoint := l.getAIEndpoint() + "/explain"

	payload := map[string]interface{}{
		"error": map[string]interface{}{
			"message": err.Error(),
			"type":    fmt.Sprintf("%T", err),
		},
	}

	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Statly-DSN", l.config.DSN)
	if apiKey != "" {
		req.Header.Set("X-AI-API-Key", apiKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI API error: %d", resp.StatusCode)
	}

	var result ErrorExplanation
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// SuggestFix gets AI fix suggestions for an error.
func (l *Logger) SuggestFix(err error, code, file, language, apiKey string) (*FixSuggestion, error) {
	if l.config.DSN == "" {
		return &FixSuggestion{
			Summary:        "AI features not available (no DSN configured)",
			SuggestedFixes: []map[string]interface{}{},
		}, nil
	}

	endpoint := l.getAIEndpoint() + "/suggest-fix"

	payload := map[string]interface{}{
		"error": map[string]interface{}{
			"message": err.Error(),
			"type":    fmt.Sprintf("%T", err),
		},
	}

	ctx := make(map[string]interface{})
	if code != "" {
		ctx["code"] = code
	}
	if file != "" {
		ctx["file"] = file
	}
	if language != "" {
		ctx["language"] = language
	}
	if len(ctx) > 0 {
		payload["context"] = ctx
	}

	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Statly-DSN", l.config.DSN)
	if apiKey != "" {
		req.Header.Set("X-AI-API-Key", apiKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI API error: %d", resp.StatusCode)
	}

	var result FixSuggestion
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (l *Logger) getAIEndpoint() string {
	if l.config.DSN == "" {
		return "https://statly.live/api/v1/logs/ai"
	}

	u, err := url.Parse(l.config.DSN)
	if err != nil {
		return "https://statly.live/api/v1/logs/ai"
	}

	return fmt.Sprintf("%s://%s/api/v1/logs/ai", u.Scheme, u.Host)
}

// WithContext creates a context with logger attached.
func (l *Logger) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// FromContext retrieves a logger from context.
func FromContext(ctx context.Context) *Logger {
	if l, ok := ctx.Value(loggerKey{}).(*Logger); ok {
		return l
	}
	return nil
}

type loggerKey struct{}
