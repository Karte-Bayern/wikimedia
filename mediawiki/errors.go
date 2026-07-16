package mediawiki

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidUserAgent indicates a missing or generic application identity.
	ErrInvalidUserAgent = errors.New("mediawiki: invalid user agent")
	// ErrRateLimited indicates an HTTP 429 response.
	ErrRateLimited = errors.New("mediawiki: rate limited")
	// ErrMaxLag indicates that the wiki rejected a request because of replication lag.
	ErrMaxLag = errors.New("mediawiki: maxlag exceeded")
	// ErrResponseTooLarge indicates a response beyond the configured byte limit.
	ErrResponseTooLarge = errors.New("mediawiki: response too large")
	// ErrInvalidResponse indicates malformed or unexpected JSON.
	ErrInvalidResponse = errors.New("mediawiki: invalid response")
)

// APIError represents an Action API or HTTP error.
type APIError struct {
	Endpoint   string
	StatusCode int
	Code       string
	Message    string
	RetryAfter time.Duration
	cause      error
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e == nil {
		return "mediawiki: API error"
	}
	if e.StatusCode != 0 {
		return fmt.Sprintf("mediawiki: %s: HTTP %d: %s", e.Endpoint, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("mediawiki: %s: %s: %s", e.Endpoint, e.Code, e.Message)
}

// Unwrap exposes a stable sentinel error where one is known.
func (e *APIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}
