package statly

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// MockTransport is a transport that stores events for testing.
type MockTransport struct {
	mu      sync.Mutex
	events  []*Event
	flushed bool
	closed  bool
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		events: make([]*Event, 0),
	}
}

func (t *MockTransport) Send(event *Event) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, event)
	return true
}

func (t *MockTransport) Flush(timeout time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.flushed = true
}

func (t *MockTransport) Close(timeout time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
}

func (t *MockTransport) Events() []*Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.events
}

func TestClientInit(t *testing.T) {
	transport := NewMockTransport()

	client, err := NewClient(Options{
		DSN:         "https://sk_test_xxx@statly.live/test",
		Environment: "test",
		Release:     "1.0.0",
		Transport:   transport,
	})

	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client.options.DSN != "https://sk_test_xxx@statly.live/test" {
		t.Errorf("Expected DSN to be set")
	}

	if client.options.Environment != "test" {
		t.Errorf("Expected environment to be 'test'")
	}
}

func TestClientInitMissingDSN(t *testing.T) {
	_, err := NewClient(Options{})

	if err != ErrMissingDSN {
		t.Errorf("Expected ErrMissingDSN, got %v", err)
	}
}

func TestCaptureException(t *testing.T) {
	transport := NewMockTransport()

	client, _ := NewClient(Options{
		DSN:       "https://sk_test_xxx@statly.live/test",
		Transport: transport,
	})

	testErr := errors.New("test error")
	eventID := client.CaptureException(testErr)

	if eventID == "" {
		t.Errorf("Expected event ID")
	}

	events := transport.Events()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].Level != LevelError {
		t.Errorf("Expected level to be error")
	}

	if len(events[0].Exception) != 1 {
		t.Errorf("Expected 1 exception")
	}

	if events[0].Exception[0].Value != "test error" {
		t.Errorf("Expected exception value to be 'test error'")
	}
}

func TestCaptureMessage(t *testing.T) {
	transport := NewMockTransport()

	client, _ := NewClient(Options{
		DSN:       "https://sk_test_xxx@statly.live/test",
		Transport: transport,
	})

	eventID := client.CaptureMessage("test message", LevelWarning)

	if eventID == "" {
		t.Errorf("Expected event ID")
	}

	events := transport.Events()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].Message != "test message" {
		t.Errorf("Expected message to be 'test message'")
	}

	if events[0].Level != LevelWarning {
		t.Errorf("Expected level to be warning")
	}
}

func TestSetUser(t *testing.T) {
	transport := NewMockTransport()

	client, _ := NewClient(Options{
		DSN:       "https://sk_test_xxx@statly.live/test",
		Transport: transport,
	})

	client.SetUser(User{
		ID:    "user-123",
		Email: "test@example.com",
	})

	client.CaptureMessage("test", LevelInfo)

	events := transport.Events()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event")
	}

	if events[0].User == nil {
		t.Fatalf("Expected user to be set")
	}

	if events[0].User.ID != "user-123" {
		t.Errorf("Expected user ID to be 'user-123'")
	}

	if events[0].User.Email != "test@example.com" {
		t.Errorf("Expected user email to be 'test@example.com'")
	}
}

func TestSetTags(t *testing.T) {
	transport := NewMockTransport()

	client, _ := NewClient(Options{
		DSN:       "https://sk_test_xxx@statly.live/test",
		Transport: transport,
	})

	client.SetTag("key", "value")
	client.SetTags(map[string]string{"foo": "bar", "baz": "qux"})

	client.CaptureMessage("test", LevelInfo)

	events := transport.Events()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event")
	}

	if events[0].Tags["key"] != "value" {
		t.Errorf("Expected tag 'key' to be 'value'")
	}

	if events[0].Tags["foo"] != "bar" {
		t.Errorf("Expected tag 'foo' to be 'bar'")
	}
}

func TestAddBreadcrumb(t *testing.T) {
	transport := NewMockTransport()

	client, _ := NewClient(Options{
		DSN:       "https://sk_test_xxx@statly.live/test",
		Transport: transport,
	})

	client.AddBreadcrumb(Breadcrumb{
		Message:  "test breadcrumb",
		Category: "test",
		Level:    LevelInfo,
		Data: map[string]interface{}{
			"key": "value",
		},
	})

	client.CaptureMessage("test", LevelInfo)

	events := transport.Events()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event")
	}

	if len(events[0].Breadcrumbs) != 1 {
		t.Fatalf("Expected 1 breadcrumb")
	}

	if events[0].Breadcrumbs[0].Message != "test breadcrumb" {
		t.Errorf("Expected breadcrumb message to be 'test breadcrumb'")
	}

	if events[0].Breadcrumbs[0].Category != "test" {
		t.Errorf("Expected breadcrumb category to be 'test'")
	}
}

func TestSampleRate(t *testing.T) {
	transport := NewMockTransport()

	client, _ := NewClient(Options{
		DSN:        "https://sk_test_xxx@statly.live/test",
		SampleRate: 0.0, // Drop all events
		Transport:  transport,
	})

	client.CaptureMessage("test", LevelInfo)

	events := transport.Events()
	if len(events) != 0 {
		t.Errorf("Expected 0 events due to sample rate")
	}
}

func TestBeforeSend(t *testing.T) {
	transport := NewMockTransport()

	client, _ := NewClient(Options{
		DSN:       "https://sk_test_xxx@statly.live/test",
		Transport: transport,
		BeforeSend: func(event *Event) *Event {
			event.Tags["custom"] = "added"
			return event
		},
	})

	client.CaptureMessage("test", LevelInfo)

	events := transport.Events()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event")
	}

	if events[0].Tags["custom"] != "added" {
		t.Errorf("Expected custom tag to be added by before_send")
	}
}

func TestBeforeSendDropEvent(t *testing.T) {
	transport := NewMockTransport()

	client, _ := NewClient(Options{
		DSN:       "https://sk_test_xxx@statly.live/test",
		Transport: transport,
		BeforeSend: func(event *Event) *Event {
			return nil // Drop all events
		},
	})

	client.CaptureMessage("test", LevelInfo)

	events := transport.Events()
	if len(events) != 0 {
		t.Errorf("Expected 0 events due to before_send returning nil")
	}
}

func TestFlush(t *testing.T) {
	transport := NewMockTransport()

	client, _ := NewClient(Options{
		DSN:       "https://sk_test_xxx@statly.live/test",
		Transport: transport,
	})

	client.Flush()

	if !transport.flushed {
		t.Errorf("Expected transport to be flushed")
	}
}

func TestClose(t *testing.T) {
	transport := NewMockTransport()

	client, _ := NewClient(Options{
		DSN:       "https://sk_test_xxx@statly.live/test",
		Transport: transport,
	})

	client.Close()

	if !transport.closed {
		t.Errorf("Expected transport to be closed")
	}
}

func TestScopeClone(t *testing.T) {
	scope := NewScope()
	scope.SetUser(User{ID: "123"})
	scope.SetTag("key", "value")
	scope.AddBreadcrumb(Breadcrumb{Message: "test"})

	cloned := scope.Clone()

	if cloned.user.ID != "123" {
		t.Errorf("Expected cloned user ID to be '123'")
	}

	if cloned.tags["key"] != "value" {
		t.Errorf("Expected cloned tag 'key' to be 'value'")
	}

	if len(cloned.breadcrumbs) != 1 {
		t.Errorf("Expected cloned breadcrumbs length to be 1")
	}

	// Modify original, cloned should be unaffected
	scope.SetTag("key", "modified")

	if cloned.tags["key"] != "value" {
		t.Errorf("Cloned scope should not be affected by original changes")
	}
}

func TestScopeClear(t *testing.T) {
	scope := NewScope()
	scope.SetUser(User{ID: "123"})
	scope.SetTag("key", "value")
	scope.AddBreadcrumb(Breadcrumb{Message: "test"})

	scope.Clear()

	if scope.user != nil {
		t.Errorf("Expected user to be nil after clear")
	}

	if len(scope.tags) != 0 {
		t.Errorf("Expected tags to be empty after clear")
	}

	if len(scope.breadcrumbs) != 0 {
		t.Errorf("Expected breadcrumbs to be empty after clear")
	}
}

func TestMaxBreadcrumbs(t *testing.T) {
	scope := NewScope()
	scope.maxBreadcrumbs = 5

	for i := 0; i < 10; i++ {
		scope.AddBreadcrumb(Breadcrumb{Message: "breadcrumb"})
	}

	if len(scope.breadcrumbs) != 5 {
		t.Errorf("Expected 5 breadcrumbs, got %d", len(scope.breadcrumbs))
	}
}
