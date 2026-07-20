package mediawiki

import (
	"net/http"
	"time"
)

// HTTPDoer is implemented by http.Client.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// RetryPolicy controls retries for read-only requests.
type RetryPolicy struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

func defaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, InitialBackoff: 250 * time.Millisecond, MaxBackoff: 4 * time.Second}
}

type config struct {
	httpClient       HTTPDoer
	userAgent        string
	maxLag           int
	maxResponseBytes int64
	retry            RetryPolicy
	cache            Cache
	cacheTTL         time.Duration
}

func defaultConfig() config {
	return config{
		httpClient:       &http.Client{Timeout: 45 * time.Second},
		maxLag:           5,
		maxResponseBytes: 64 << 20,
		retry:            defaultRetryPolicy(),
		cacheTTL:         24 * time.Hour,
	}
}

// Option configures a Client.
type Option func(*config)

// WithHTTPClient injects an HTTP transport.
func WithHTTPClient(client HTTPDoer) Option {
	return func(c *config) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithUserAgent sets the descriptive application User-Agent.
func WithUserAgent(value string) Option { return func(c *config) { c.userAgent = value } }

// WithMaxLag sets the Action API maxlag parameter. A negative value disables it.
func WithMaxLag(seconds int) Option { return func(c *config) { c.maxLag = seconds } }

// WithMaxResponseBytes limits JSON response bodies.
func WithMaxResponseBytes(value int64) Option {
	return func(c *config) { c.maxResponseBytes = value }
}

// WithRetryPolicy configures bounded retries.
func WithRetryPolicy(value RetryPolicy) Option { return func(c *config) { c.retry = value } }

// WithCache enables successful-response caching. A non-positive TTL keeps
// entries until the cache implementation evicts them.
func WithCache(cache Cache, ttl time.Duration) Option {
	return func(c *config) { c.cache, c.cacheTTL = cache, ttl }
}
