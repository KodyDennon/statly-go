package statly

import (
	"math/rand"
	"sync"
	"time"
)

// Client is the main client for capturing and sending events to Statly.
type Client struct {
	options   Options
	transport Transport
	scope     *Scope
	mu        sync.RWMutex
}

// NewClient creates a new Statly client.
func NewClient(options Options) (*Client, error) {
	if options.DSN == "" {
		return nil, ErrMissingDSN
	}

	// Set defaults
	if options.SampleRate == 0 {
		options.SampleRate = 1.0
	}
	if options.MaxBreadcrumbs == 0 {
		options.MaxBreadcrumbs = 100
	}
	if options.FlushTimeout == 0 {
		options.FlushTimeout = 5 * time.Second
	}
	if options.ServerName == "" {
		options.ServerName = getHostname()
	}

	// Create transport
	var transport Transport
	if options.Transport != nil {
		transport = options.Transport
	} else {
		transport = NewHTTPTransport(TransportOptions{
			DSN:     options.DSN,
			Timeout: 30 * time.Second,
			Debug:   options.Debug,
		})
	}

	client := &Client{
		options:   options,
		transport: transport,
		scope:     NewScope(),
	}

	client.scope.maxBreadcrumbs = options.MaxBreadcrumbs

	return client, nil
}

// CaptureException captures an error and sends it to Statly.
func (c *Client) CaptureException(err error) string {
	return c.CaptureExceptionWithContext(err, nil)
}

// CaptureExceptionWithContext captures an error with additional context.
func (c *Client) CaptureExceptionWithContext(err error, ctx map[string]interface{}) string {
	if err == nil {
		return ""
	}

	// Sample rate check
	if rand.Float64() > c.options.SampleRate {
		return ""
	}

	// Build event
	event := NewExceptionEvent(err)
	event.Environment = c.options.Environment
	event.Release = c.options.Release
	event.ServerName = c.options.ServerName
	event.Contexts["runtime"] = getRuntimeInfo()

	// Add extra context
	if ctx != nil {
		for k, v := range ctx {
			event.Extra[k] = v
		}
	}

	// Apply scope
	c.mu.RLock()
	c.scope.ApplyToEvent(event)
	c.mu.RUnlock()

	return c.sendEvent(event)
}

// CaptureMessage captures a message and sends it to Statly.
func (c *Client) CaptureMessage(message string, level Level) string {
	return c.CaptureMessageWithContext(message, level, nil)
}

// CaptureMessageWithContext captures a message with additional context.
func (c *Client) CaptureMessageWithContext(message string, level Level, ctx map[string]interface{}) string {
	// Sample rate check
	if rand.Float64() > c.options.SampleRate {
		return ""
	}

	// Build event
	event := NewMessageEvent(message, level)
	event.Environment = c.options.Environment
	event.Release = c.options.Release
	event.ServerName = c.options.ServerName
	event.Contexts["runtime"] = getRuntimeInfo()

	// Add extra context
	if ctx != nil {
		for k, v := range ctx {
			event.Extra[k] = v
		}
	}

	// Apply scope
	c.mu.RLock()
	c.scope.ApplyToEvent(event)
	c.mu.RUnlock()

	return c.sendEvent(event)
}

// StartSpan starts a new tracing span.
func (c *Client) StartSpan(ctx context.Context, name string) (*Span, context.Context) {
	parent := SpanFromContext(ctx)
	
	var traceID, parentID string
	if parent != nil {
		traceID = parent.Context.TraceID
		parentID = parent.Context.SpanID
	} else {
		traceID = generateEventID()
	}

	span := &Span{
		Name:      name,
		StartTime: time.Now(),
		Status:    SpanStatusOK,
		Context: SpanContext{
			TraceID:  traceID,
			SpanID:   generateEventID(),
			ParentID: parentID,
		},
		client: c,
	}

	return span, ContextWithSpan(ctx, span)
}

// CaptureSpan sends a completed span to Statly.
func (c *Client) CaptureSpan(span *Span) string {
	event := NewEvent()
	event.Level = LevelSpan
	event.Message = fmt.Sprintf("Span: %s", span.Name)
	event.Environment = c.options.Environment
	event.Release = c.options.Release
	event.ServerName = c.options.ServerName
	
	data := span.ToData()
	event.Span = &data

	// Apply scope
	c.mu.RLock()
	c.scope.ApplyToEvent(event)
	c.mu.RUnlock()

	return c.sendEvent(event)
}

// sendEvent sends an event to Statly.
func (c *Client) sendEvent(event *Event) string {
	// Apply before_send callback
	if c.options.BeforeSend != nil {
		event = c.options.BeforeSend(event)
		if event == nil {
			return ""
		}
	}

	// Send via transport
	if c.transport.Send(event) {
		return event.EventID
	}

	return ""
}

// SetUser sets the current user context.
func (c *Client) SetUser(user User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scope.SetUser(user)
}

// SetTag sets a tag on the current scope.
func (c *Client) SetTag(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scope.SetTag(key, value)
}

// SetTags sets multiple tags on the current scope.
func (c *Client) SetTags(tags map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scope.SetTags(tags)
}

// SetExtra sets extra data on the current scope.
func (c *Client) SetExtra(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scope.SetExtra(key, value)
}

// AddBreadcrumb adds a breadcrumb to the current scope.
func (c *Client) AddBreadcrumb(crumb Breadcrumb) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scope.AddBreadcrumb(crumb)
}

// Flush flushes pending events.
func (c *Client) Flush() {
	c.transport.Flush(c.options.FlushTimeout)
}

// Close closes the client and flushes pending events.
func (c *Client) Close() {
	c.transport.Close(c.options.FlushTimeout)
}
