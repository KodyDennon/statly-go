package statly

import "errors"

// Common errors returned by the SDK.
var (
	ErrMissingDSN       = errors.New("statly: DSN is required")
	ErrNotInitialized   = errors.New("statly: SDK not initialized, call Init() first")
	ErrAlreadyInitialized = errors.New("statly: SDK already initialized, call Close() first")
)
