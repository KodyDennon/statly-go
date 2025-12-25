package statly

import (
	"sync"
	"time"
)

// Scope holds contextual information to be attached to events.
type Scope struct {
	mu             sync.RWMutex
	user           *User
	tags           map[string]string
	extra          map[string]interface{}
	contexts       map[string]interface{}
	breadcrumbs    []Breadcrumb
	maxBreadcrumbs int
	transaction    string
	fingerprint    []string
}

// NewScope creates a new scope.
func NewScope() *Scope {
	return &Scope{
		tags:           make(map[string]string),
		extra:          make(map[string]interface{}),
		contexts:       make(map[string]interface{}),
		breadcrumbs:    make([]Breadcrumb, 0),
		maxBreadcrumbs: 100,
	}
}

// SetUser sets the current user.
func (s *Scope) SetUser(user User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.user = &user
}

// ClearUser clears the current user.
func (s *Scope) ClearUser() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.user = nil
}

// SetTag sets a tag.
func (s *Scope) SetTag(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tags[key] = value
}

// SetTags sets multiple tags.
func (s *Scope) SetTags(tags map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range tags {
		s.tags[k] = v
	}
}

// RemoveTag removes a tag.
func (s *Scope) RemoveTag(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tags, key)
}

// SetExtra sets extra data.
func (s *Scope) SetExtra(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.extra[key] = value
}

// SetContext sets a context.
func (s *Scope) SetContext(key string, value map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.contexts[key] = value
}

// AddBreadcrumb adds a breadcrumb.
func (s *Scope) AddBreadcrumb(crumb Breadcrumb) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if crumb.Timestamp.IsZero() {
		crumb.Timestamp = time.Now().UTC()
	}
	if crumb.Level == "" {
		crumb.Level = LevelInfo
	}
	if crumb.Type == "" {
		crumb.Type = "default"
	}

	s.breadcrumbs = append(s.breadcrumbs, crumb)

	// Trim breadcrumbs if we exceed the limit
	if len(s.breadcrumbs) > s.maxBreadcrumbs {
		s.breadcrumbs = s.breadcrumbs[len(s.breadcrumbs)-s.maxBreadcrumbs:]
	}
}

// ClearBreadcrumbs clears all breadcrumbs.
func (s *Scope) ClearBreadcrumbs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.breadcrumbs = make([]Breadcrumb, 0)
}

// SetTransaction sets the transaction name.
func (s *Scope) SetTransaction(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transaction = name
}

// SetFingerprint sets the fingerprint for grouping.
func (s *Scope) SetFingerprint(fingerprint []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fingerprint = fingerprint
}

// Clear clears all scope data.
func (s *Scope) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.user = nil
	s.tags = make(map[string]string)
	s.extra = make(map[string]interface{})
	s.contexts = make(map[string]interface{})
	s.breadcrumbs = make([]Breadcrumb, 0)
	s.transaction = ""
	s.fingerprint = nil
}

// Clone creates a deep copy of this scope.
func (s *Scope) Clone() *Scope {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clone := NewScope()
	clone.maxBreadcrumbs = s.maxBreadcrumbs

	if s.user != nil {
		user := *s.user
		clone.user = &user
	}

	for k, v := range s.tags {
		clone.tags[k] = v
	}

	for k, v := range s.extra {
		clone.extra[k] = v
	}

	for k, v := range s.contexts {
		clone.contexts[k] = v
	}

	clone.breadcrumbs = make([]Breadcrumb, len(s.breadcrumbs))
	copy(clone.breadcrumbs, s.breadcrumbs)

	clone.transaction = s.transaction

	if s.fingerprint != nil {
		clone.fingerprint = make([]string, len(s.fingerprint))
		copy(clone.fingerprint, s.fingerprint)
	}

	return clone
}

// ApplyToEvent applies this scope's data to an event.
func (s *Scope) ApplyToEvent(event *Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Apply user
	if s.user != nil {
		event.User = &EventUser{
			ID:       s.user.ID,
			Email:    s.user.Email,
			Username: s.user.Username,
			IPAddr:   s.user.IPAddr,
			Data:     s.user.Data,
		}
	}

	// Apply tags
	for k, v := range s.tags {
		event.Tags[k] = v
	}

	// Apply extra
	for k, v := range s.extra {
		event.Extra[k] = v
	}

	// Apply contexts
	for k, v := range s.contexts {
		event.Contexts[k] = v
	}

	// Apply breadcrumbs
	for _, crumb := range s.breadcrumbs {
		event.Breadcrumbs = append(event.Breadcrumbs, BreadcrumbValue{
			Message:   crumb.Message,
			Category:  crumb.Category,
			Level:     crumb.Level,
			Type:      crumb.Type,
			Data:      crumb.Data,
			Timestamp: crumb.Timestamp.Format(time.RFC3339),
		})
	}
}
