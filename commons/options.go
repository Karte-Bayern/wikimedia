package commons

import (
	"time"

	"github.com/karte-bayern/wikimedia/mediawiki"
)

const defaultEndpoint = "https://commons.wikimedia.org/w/api.php"

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
	language         string
	thumbnailWidth   int
	batchSize        int
}

func defaultConfig() config {
	return config{
		endpoint: defaultEndpoint, maxLag: 5, maxResponseBytes: 64 << 20,
		retry:    mediawiki.RetryPolicy{MaxAttempts: 3, InitialBackoff: 250 * time.Millisecond, MaxBackoff: 4 * time.Second},
		cacheTTL: 7 * 24 * time.Hour, language: "en", thumbnailWidth: 1200, batchSize: 10,
	}
}

// Option configures a Commons Client.
type Option func(*config)

// WithEndpoint overrides the Commons Action API endpoint.
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

// WithLanguage sets the preferred Commons metadata language.
func WithLanguage(value string) Option { return func(c *config) { c.language = value } }

// WithThumbnailWidth sets the default requested thumbnail width.
func WithThumbnailWidth(value int) Option { return func(c *config) { c.thumbnailWidth = value } }

// WithBatchSize sets the conservative number of file titles per request.
func WithBatchSize(value int) Option { return func(c *config) { c.batchSize = value } }

// FileOption configures file metadata requests.
type FileOption func(*fileConfig)
type fileConfig struct {
	language       string
	thumbnailWidth int
	commonMetadata bool
}

// FileLanguage overrides the metadata language for one file request.
func FileLanguage(value string) FileOption { return func(c *fileConfig) { c.language = value } }

// FileThumbnailWidth overrides thumbnail width for one file request.
func FileThumbnailWidth(value int) FileOption {
	return func(c *fileConfig) { c.thumbnailWidth = value }
}

// FileCommonMetadata enables the larger imageinfo commonmetadata block.
func FileCommonMetadata(value bool) FileOption {
	return func(c *fileConfig) { c.commonMetadata = value }
}

// CategoryOption configures one category request.
type CategoryOption func(*categoryConfig)
type categoryConfig struct {
	language       string
	thumbnailWidth int
	limit          int
	continueToken  string
	continueValue  string
	commonMetadata bool
}

// CategoryLanguage overrides metadata language for one category page.
func CategoryLanguage(value string) CategoryOption {
	return func(c *categoryConfig) { c.language = value }
}

// CategoryThumbnailWidth overrides thumbnail width for category members.
func CategoryThumbnailWidth(value int) CategoryOption {
	return func(c *categoryConfig) { c.thumbnailWidth = value }
}

// CategoryLimit limits direct files on one category page.
func CategoryLimit(value int) CategoryOption { return func(c *categoryConfig) { c.limit = value } }

// CategoryContinue resumes explicit category pagination.
func CategoryContinue(value string) CategoryOption {
	return func(c *categoryConfig) {
		c.continueToken = value
		if value != "" {
			c.continueValue = "-||"
		}
	}
}

// CategoryContinueWith resumes pagination with both values returned by MediaWiki.
func CategoryContinueWith(token, generic string) CategoryOption {
	return func(c *categoryConfig) {
		c.continueToken = token
		c.continueValue = generic
	}
}

// CategoryCommonMetadata enables commonmetadata for category members.
func CategoryCommonMetadata(value bool) CategoryOption {
	return func(c *categoryConfig) { c.commonMetadata = value }
}
