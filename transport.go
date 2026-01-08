package statly

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Transport defines the interface for sending events.
type Transport interface {
	Send(event *Event) bool
	Flush(timeout time.Duration)
	Close(timeout time.Duration)
}

// TransportOptions configures the HTTP transport.
type TransportOptions struct {
	DSN         string
	Timeout     time.Duration
	MaxRetries  int
	RetryDelay  time.Duration
	BatchSize   int
	FlushPeriod time.Duration
	Debug       bool
}

// HTTPTransport sends events over HTTP with batching and retry support.
type HTTPTransport struct {
	options  TransportOptions
	dsn      string
	endpoint string
	client   *http.Client
	queue    chan *Event
	wg       sync.WaitGroup
	done     chan struct{}
	mu       sync.Mutex
}

// NewHTTPTransport creates a new HTTP transport.
func NewHTTPTransport(options TransportOptions) *HTTPTransport {
	// Set defaults
	if options.Timeout == 0 {
		options.Timeout = 30 * time.Second
	}
	if options.MaxRetries == 0 {
		options.MaxRetries = 3
	}
	if options.RetryDelay == 0 {
		options.RetryDelay = time.Second
	}
	if options.BatchSize == 0 {
		options.BatchSize = 10
	}
	if options.FlushPeriod == 0 {
		options.FlushPeriod = 5 * time.Second
	}

	t := &HTTPTransport{
		options:  options,
		dsn:      options.DSN,
		endpoint: parseDSN(options.DSN),
		client: &http.Client{
			Timeout: options.Timeout,
		},
		queue: make(chan *Event, 100),
		done:  make(chan struct{}),
	}

	// Start background worker
	t.wg.Add(1)
	go t.worker()

	return t
}

// parseDSN parses the DSN and returns the API endpoint.
// DSN format: https://<api-key>@statly.live/<org-slug>
func parseDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		// Fallback - assume it's just the org slug or misformatted
		return "https://statly.live/api/v1/observe/ingest"
	}

	return fmt.Sprintf("%s://%s/api/v1/observe/ingest", u.Scheme, u.Host)
}

// Send queues an event for sending.
func (t *HTTPTransport) Send(event *Event) bool {
	select {
	case t.queue <- event:
		if t.options.Debug {
			log.Printf("[statly] Event queued: %s", event.EventID)
		}
		return true
	case <-t.done:
		return false
	default:
		if t.options.Debug {
			log.Printf("[statly] Queue full, event dropped: %s", event.EventID)
		}
		return false
	}
}

// Flush flushes pending events.
func (t *HTTPTransport) Flush(timeout time.Duration) {
	// Wait for queue to drain
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-time.After(100 * time.Millisecond):
			if len(t.queue) == 0 || time.Now().After(deadline) {
				return
			}
		}
	}
}

// Close closes the transport.
func (t *HTTPTransport) Close(timeout time.Duration) {
	close(t.done)
	t.wg.Wait()
}

// worker processes events in the background.
func (t *HTTPTransport) worker() {
	defer t.wg.Done()

	var batch []*Event
	timer := time.NewTimer(t.options.FlushPeriod)
	defer timer.Stop()

	for {
		select {
		case event := <-t.queue:
			batch = append(batch, event)

			// Send if batch is full
			if len(batch) >= t.options.BatchSize {
				t.sendBatch(batch)
				batch = nil
				timer.Reset(t.options.FlushPeriod)
			}

		case <-timer.C:
			// Send pending batch
			if len(batch) > 0 {
				t.sendBatch(batch)
				batch = nil
			}
			timer.Reset(t.options.FlushPeriod)

		case <-t.done:
			// Send remaining events
			if len(batch) > 0 {
				t.sendBatch(batch)
			}

			// Drain queue
			for {
				select {
				case event := <-t.queue:
					batch = append(batch, event)
					if len(batch) >= t.options.BatchSize {
						t.sendBatch(batch)
						batch = nil
					}
				default:
					if len(batch) > 0 {
						t.sendBatch(batch)
					}
					return
				}
			}
		}
	}
}

// sendBatch sends a batch of events.
func (t *HTTPTransport) sendBatch(batch []*Event) {
	if len(batch) == 0 {
		return
	}

	// Build request body
	type requestBody struct {
		Events []*Event `json:"events"`
	}

	body := requestBody{Events: batch}
	data, err := json.Marshal(body)
	if err != nil {
		if t.options.Debug {
			log.Printf("[statly] Failed to marshal events: %v", err)
		}
		return
	}

	// Retry loop
	for attempt := 0; attempt < t.options.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(t.options.RetryDelay * time.Duration(1<<attempt))
		}

		req, err := http.NewRequest("POST", t.endpoint, bytes.NewReader(data))
		if err != nil {
			if t.options.Debug {
				log.Printf("[statly] Failed to create request: %v", err)
			}
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", fmt.Sprintf("statly-observe-go/%s", Version))
		req.Header.Set("X-Statly-DSN", t.dsn)

		resp, err := t.client.Do(req)
		if err != nil {
			if t.options.Debug {
				log.Printf("[statly] Request failed: %v (attempt %d/%d)", err, attempt+1, t.options.MaxRetries)
			}
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == 200 || resp.StatusCode == 202 {
			if t.options.Debug {
				log.Printf("[statly] Sent %d events successfully", len(batch))
			}
			return
		}

		// Don't retry on 4xx errors
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			if t.options.Debug {
				log.Printf("[statly] Client error %d, not retrying", resp.StatusCode)
			}
			return
		}

		if t.options.Debug {
			log.Printf("[statly] Server error %d (attempt %d/%d)", resp.StatusCode, attempt+1, t.options.MaxRetries)
		}
	}

	if t.options.Debug {
		log.Printf("[statly] Failed to send %d events after %d retries", len(batch), t.options.MaxRetries)
	}
}

// SyncTransport sends events synchronously (useful for testing).
type SyncTransport struct {
	options  TransportOptions
	dsn      string
	endpoint string
	client   *http.Client
}

// NewSyncTransport creates a new synchronous transport.
func NewSyncTransport(options TransportOptions) *SyncTransport {
	if options.Timeout == 0 {
		options.Timeout = 30 * time.Second
	}
	if options.MaxRetries == 0 {
		options.MaxRetries = 3
	}

	return &SyncTransport{
		options:  options,
		dsn:      options.DSN,
		endpoint: parseDSN(options.DSN),
		client: &http.Client{
			Timeout: options.Timeout,
		},
	}
}

// Send sends an event synchronously.
func (t *SyncTransport) Send(event *Event) bool {
	data, err := json.Marshal(event)
	if err != nil {
		return false
	}

	for attempt := 0; attempt < t.options.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(t.options.RetryDelay * time.Duration(1<<attempt))
		}

		req, err := http.NewRequest("POST", t.endpoint, bytes.NewReader(data))
		if err != nil {
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", fmt.Sprintf("statly-observe-go/%s", Version))
		req.Header.Set("X-Statly-DSN", t.dsn)

		resp, err := t.client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == 200 || resp.StatusCode == 202 {
			return true
		}

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return false
		}
	}

	return false
}

// Flush is a no-op for sync transport.
func (t *SyncTransport) Flush(timeout time.Duration) {}

// Close is a no-op for sync transport.
func (t *SyncTransport) Close(timeout time.Duration) {}
