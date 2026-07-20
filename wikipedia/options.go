package wikipedia

import (
	"time"

	"github.com/karte-bayern/wikimedia/mediawiki"
)

type config struct {
	httpClient       mediawiki.HTTPDoer
	userAgent        string
	maxLag           int
	maxResponseBytes int64
	retry            mediawiki.RetryPolicy
	cache            mediawiki.Cache
	cacheTTL         time.Duration
	thumbnailWidth   int
	endpointTemplate string
}

func defaultConfig() config {
	return config{
		maxLag: 5, maxResponseBytes: 32 << 20,
		retry:    mediawiki.RetryPolicy{MaxAttempts: 3, InitialBackoff: 250 * time.Millisecond, MaxBackoff: 4 * time.Second},
		cacheTTL: 24 * time.Hour, thumbnailWidth: 1200,
		endpointTemplate: "https://%s.wikipedia.org/w/api.php",
	}
}

// Option configures a Client.
type Option func(*config)

// WithHTTPClient injects an HTTP transport.
func WithHTTPClient(value mediawiki.HTTPDoer) Option { return func(c *config) { c.httpClient = value } }

// WithUserAgent sets the descriptive application User-Agent.
func WithUserAgent(value string) Option { return func(c *config) { c.userAgent = value } }

// WithMaxLag sets maxlag; a negative value disables it.
func WithMaxLag(value int) Option { return func(c *config) { c.maxLag = value } }

// WithMaxResponseBytes limits JSON response bodies.
func WithMaxResponseBytes(value int64) Option { return func(c *config) { c.maxResponseBytes = value } }

// WithRetryPolicy configures bounded read retries.
func WithRetryPolicy(value mediawiki.RetryPolicy) Option { return func(c *config) { c.retry = value } }

// WithCache enables successful raw-response caching. A non-positive TTL keeps
// entries until the cache implementation evicts them.
func WithCache(value mediawiki.Cache, ttl time.Duration) Option {
	return func(c *config) { c.cache, c.cacheTTL = value, ttl }
}

// WithThumbnailWidth sets the requested page-image width.
func WithThumbnailWidth(value int) Option { return func(c *config) { c.thumbnailWidth = value } }

// WithEndpointTemplate replaces the default https://%s.wikipedia.org/w/api.php template.
// It is primarily useful for compatible installations and tests.
func WithEndpointTemplate(value string) Option { return func(c *config) { c.endpointTemplate = value } }
