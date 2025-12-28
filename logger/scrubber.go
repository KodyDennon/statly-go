package logger

import (
	"regexp"
	"strings"
)

// REDACTED is the placeholder for scrubbed values.
const REDACTED = "[REDACTED]"

// SensitiveKeys contains key names that indicate sensitive data.
var SensitiveKeys = map[string]bool{
	"password":       true,
	"passwd":         true,
	"pwd":            true,
	"secret":         true,
	"api_key":        true,
	"apikey":         true,
	"api-key":        true,
	"token":          true,
	"access_token":   true,
	"accesstoken":    true,
	"refresh_token":  true,
	"auth":           true,
	"authorization":  true,
	"bearer":         true,
	"credential":     true,
	"credentials":    true,
	"private_key":    true,
	"privatekey":     true,
	"private-key":    true,
	"secret_key":     true,
	"secretkey":      true,
	"secret-key":     true,
	"session_id":     true,
	"sessionid":      true,
	"session-id":     true,
	"session":        true,
	"cookie":         true,
	"x-api-key":      true,
	"x-auth-token":   true,
	"x-access-token": true,
}

// Built-in scrub patterns.
var scrubPatterns = map[string]*regexp.Regexp{
	"apiKey":     regexp.MustCompile(`(?i)(?:api[_-]?key|apikey)\s*[=:]\s*["']?([a-zA-Z0-9_\-]{20,})["']?`),
	"password":   regexp.MustCompile(`(?i)(?:password|passwd|pwd|secret)\s*[=:]\s*["']?([^"'\s]{3,})["']?`),
	"token":      regexp.MustCompile(`(?i)(?:bearer\s+|token\s*[=:]\s*["']?)([a-zA-Z0-9_\-\.]{20,})["']?`),
	"creditCard": regexp.MustCompile(`\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12}|(?:2131|1800|35\d{3})\d{11})\b`),
	"ssn":        regexp.MustCompile(`\b\d{3}[-\s]?\d{2}[-\s]?\d{4}\b`),
	"email":      regexp.MustCompile(`\b[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}\b`),
	"ipAddress":  regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`),
	"awsKey":     regexp.MustCompile(`(?:AKIA|ABIA|ACCA)[A-Z0-9]{16}`),
	"privateKey": regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA )?PRIVATE KEY-----[\s\S]*?-----END (?:RSA |EC |DSA )?PRIVATE KEY-----`),
	"jwt":        regexp.MustCompile(`eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*`),
}

var defaultPatterns = []string{"apiKey", "password", "token", "creditCard", "ssn", "awsKey", "privateKey", "jwt"}

// Scrubber handles secret scrubbing.
type Scrubber struct {
	enabled       bool
	patterns      []*regexp.Regexp
	allowlist     map[string]bool
}

// NewScrubber creates a new scrubber with the given configuration.
func NewScrubber(config *ScrubbingConfig) *Scrubber {
	if config == nil {
		config = &ScrubbingConfig{Enabled: true}
	}

	s := &Scrubber{
		enabled:   config.Enabled,
		patterns:  make([]*regexp.Regexp, 0),
		allowlist: make(map[string]bool),
	}

	// Load patterns
	patternNames := config.Patterns
	if len(patternNames) == 0 {
		patternNames = defaultPatterns
	}

	for _, name := range patternNames {
		if p, ok := scrubPatterns[name]; ok {
			s.patterns = append(s.patterns, p)
		}
	}

	// Add custom regexps
	for _, p := range config.CustomRegexps {
		s.patterns = append(s.patterns, p)
	}

	// Build allowlist
	for _, key := range config.Allowlist {
		s.allowlist[strings.ToLower(key)] = true
	}

	return s
}

// Scrub scrubs sensitive data from a value.
func (s *Scrubber) Scrub(value interface{}) interface{} {
	if !s.enabled {
		return value
	}
	return s.scrubValue(value, "")
}

// ScrubString scrubs sensitive patterns from a string.
func (s *Scrubber) ScrubString(value string) string {
	if !s.enabled {
		return value
	}

	result := value
	for _, p := range s.patterns {
		result = p.ReplaceAllString(result, REDACTED)
	}
	return result
}

func (s *Scrubber) scrubValue(value interface{}, key string) interface{} {
	// Check allowlist
	if key != "" && s.allowlist[strings.ToLower(key)] {
		return value
	}

	// Check if key indicates sensitive data
	if key != "" && s.isSensitiveKey(key) {
		return REDACTED
	}

	// Handle different types
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return s.scrubString(v)
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = s.scrubValue(item, "")
		}
		return result
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, val := range v {
			result[k] = s.scrubValue(val, k)
		}
		return result
	default:
		return value
	}
}

func (s *Scrubber) scrubString(value string) string {
	result := value
	for _, p := range s.patterns {
		result = p.ReplaceAllString(result, REDACTED)
	}
	return result
}

func (s *Scrubber) isSensitiveKey(key string) bool {
	return SensitiveKeys[strings.ToLower(key)]
}

// AddPattern adds a custom regex pattern.
func (s *Scrubber) AddPattern(pattern *regexp.Regexp) {
	s.patterns = append(s.patterns, pattern)
}

// AddToAllowlist adds a key to the allowlist.
func (s *Scrubber) AddToAllowlist(key string) {
	s.allowlist[strings.ToLower(key)] = true
}

// SetEnabled enables or disables scrubbing.
func (s *Scrubber) SetEnabled(enabled bool) {
	s.enabled = enabled
}
