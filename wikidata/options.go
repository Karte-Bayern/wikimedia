package wikidata

import (
	"time"

	"github.com/karte-bayern/wikimedia/mediawiki"
)

const defaultEndpoint = "https://www.wikidata.org/w/api.php"

type config struct {
	endpoint         string
	mediawikiClient  *mediawiki.Client
	httpClient       mediawiki.HTTPDoer
	userAgent        string
	maxLag           int
	maxResponseBytes int64
	retry            mediawiki.RetryPolicy
	cache            mediawiki.Cache
	cacheTTL         time.Duration
	languages        []string
	batchSize        int
}

func defaultConfig() config {
	return config{
		endpoint: defaultEndpoint, maxLag: 5, maxResponseBytes: 64 << 20,
		retry:    mediawiki.RetryPolicy{MaxAttempts: 3, InitialBackoff: 250 * time.Millisecond, MaxBackoff: 4 * time.Second},
		cacheTTL: 24 * time.Hour, languages: []string{"en"}, batchSize: 50,
	}
}

// Option configures a Client.
type Option func(*config)

// WithEndpoint overrides the Wikidata Action API endpoint.
func WithEndpoint(value string) Option { return func(c *config) { c.endpoint = value } }

// WithMediaWikiClient injects a preconfigured Action API client.
func WithMediaWikiClient(value *mediawiki.Client) Option {
	return func(c *config) { c.mediawikiClient = value }
}

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

// WithCache enables successful raw-response caching.
func WithCache(value mediawiki.Cache, ttl time.Duration) Option {
	return func(c *config) { c.cache, c.cacheTTL = value, ttl }
}

// WithLanguages sets labels, descriptions, and aliases languages.
func WithLanguages(values ...string) Option {
	return func(c *config) { c.languages = append([]string(nil), values...) }
}

// WithBatchSize sets the entity count per wbgetentities request, capped at 50.
func WithBatchSize(value int) Option { return func(c *config) { c.batchSize = value } }
