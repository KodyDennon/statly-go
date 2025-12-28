package statly

import (
	"context"
	"time"
)

// SpanStatus represents the status of a span.
type SpanStatus string

const (
	SpanStatusOK    SpanStatus = "ok"
	SpanStatusError SpanStatus = "error"
)

// SpanContext contains tracing identification data.
type SpanContext struct {
	TraceID  string `json:"trace_id"`
	SpanID   string `json:"span_id"`
	ParentID string `json:"parent_id,omitempty"`
}

// SpanData is the serializable representation of a span.
type SpanData struct {
	Name       string                 `json:"name"`
	TraceID    string                 `json:"trace_id"`
	SpanID     string                 `json:"span_id"`
	ParentID   string                 `json:"parent_id,omitempty"`
	StartTime  int64                  `json:"start_time"`
	EndTime    int64                  `json:"end_time,omitempty"`
	DurationMs float64                `json:"duration_ms"`
	Status     SpanStatus             `json:"status"`
	Tags       map[string]string      `json:"tags,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// Span represents a single operation in a trace.
type Span struct {
	Name      string
	Context   SpanContext
	StartTime time.Time
	EndTime   time.Time
	Status    SpanStatus
	Tags      map[string]string
	Metadata  map[string]interface{}
	client    *Client
	finished  bool
}

// Finish completes the span and sends it to Statly.
func (s *Span) Finish() {
	if s.finished {
		return
	}
	s.EndTime = time.Now()
	s.finished = true
	if s.client != nil {
		s.client.CaptureSpan(s)
	}
}

// SetTag sets a tag on the span.
func (s *Span) SetTag(key, value string) {
	if s.Tags == nil {
		s.Tags = make(map[string]string)
	}
	s.Tags[key] = value
}

// SetStatus sets the status of the span.
func (s *Span) SetStatus(status SpanStatus) {
	s.Status = status
}

// ToData converts the Span to its serializable format.
func (s *Span) ToData() SpanData {
	duration := s.EndTime.Sub(s.StartTime)
	if !s.finished {
		duration = time.Since(s.StartTime)
	}

	return SpanData{
		Name:       s.Name,
		TraceID:    s.Context.TraceID,
		SpanID:     s.Context.SpanID,
		ParentID:   s.Context.ParentID,
		StartTime:  s.StartTime.UnixNano() / 1e6,
		EndTime:    s.EndTime.UnixNano() / 1e6,
		DurationMs: float64(duration.Milliseconds()),
		Status:     s.Status,
		Tags:       s.Tags,
		Metadata:   s.Metadata,
	}
}

type ctxKey struct{}

var activeSpanKey = ctxKey{}

// ContextWithSpan returns a new context with the given span.
func ContextWithSpan(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, activeSpanKey, span)
}

// SpanFromContext returns the span stored in the context, or nil if none.
func SpanFromContext(ctx context.Context) *Span {
	if span, ok := ctx.Value(activeSpanKey).(*Span); ok {
		return span
	}
	return nil
}
