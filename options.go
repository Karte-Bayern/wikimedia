package wikimedia

import (
	"net/http"
	"strings"
	"time"

	"github.com/karte-bayern/wikimedia/mediawiki"
)

// CacheTTLs controls per-service raw response expiration. Zero fields use defaults.
type CacheTTLs struct {
	Wikidata  time.Duration
	Commons   time.Duration
	Wikipedia time.Duration
}

type config struct {
	userAgent                 string
	languages                 []string
	httpClient                mediawiki.HTTPDoer
	wikidataEndpoint          string
	commonsEndpoint           string
	wikipediaEndpointTemplate string
	maxLag                    int
	maxResponseBytes          int64
	retry                     mediawiki.RetryPolicy
	cache                     mediawiki.Cache
	cacheTTLs                 CacheTTLs
}

func defaultConfig() config {
	return config{
		languages: []string{"en"}, wikidataEndpoint: "https://www.wikidata.org/w/api.php",
		commonsEndpoint:           "https://commons.wikimedia.org/w/api.php",
		wikipediaEndpointTemplate: "https://%s.wikipedia.org/w/api.php",
		maxLag:                    5, maxResponseBytes: 64 << 20,
		retry:     mediawiki.RetryPolicy{MaxAttempts: 3, InitialBackoff: 250 * time.Millisecond, MaxBackoff: 4 * time.Second},
		cacheTTLs: CacheTTLs{Wikidata: 24 * time.Hour, Commons: 7 * 24 * time.Hour, Wikipedia: 24 * time.Hour},
	}
}

// Option configures the aggregate Client.
type Option func(*config)

// WithUserAgent sets the descriptive application User-Agent.
func WithUserAgent(value string) Option { return func(c *config) { c.userAgent = value } }

// WithLanguages sets the preferred language fallback order.
func WithLanguages(values ...string) Option {
	return func(c *config) { c.languages = append([]string(nil), values...) }
}

// WithHTTPClient injects a standard HTTP client.
func WithHTTPClient(value *http.Client) Option {
	return func(c *config) {
		if value != nil {
			c.httpClient = value
		}
	}
}

// WithHTTPDoer injects a custom transport, including test doubles and traced clients.
func WithHTTPDoer(value mediawiki.HTTPDoer) Option {
	return func(c *config) {
		if value != nil {
			c.httpClient = value
		}
	}
}

// WithWikidataEndpoint overrides the Wikidata Action API endpoint.
func WithWikidataEndpoint(value string) Option { return func(c *config) { c.wikidataEndpoint = value } }

// WithCommonsEndpoint overrides the Commons Action API endpoint.
func WithCommonsEndpoint(value string) Option { return func(c *config) { c.commonsEndpoint = value } }

// WithWikipediaEndpointTemplate overrides the language-specific Wikipedia endpoint template.
func WithWikipediaEndpointTemplate(value string) Option {
	return func(c *config) { c.wikipediaEndpointTemplate = value }
}

// WithMaxLag sets the Action API maxlag value; a negative value disables it.
func WithMaxLag(value int) Option { return func(c *config) { c.maxLag = value } }

// WithMaxResponseBytes limits each decoded API response body.
func WithMaxResponseBytes(value int64) Option { return func(c *config) { c.maxResponseBytes = value } }

// WithRetryPolicy configures retries for rate limits, maxlag, and temporary failures.
func WithRetryPolicy(value mediawiki.RetryPolicy) Option { return func(c *config) { c.retry = value } }

// WithCache enables raw response caching with optional per-service TTL overrides.
func WithCache(value mediawiki.Cache, ttls CacheTTLs) Option {
	return func(c *config) {
		c.cache = value
		if ttls.Wikidata > 0 {
			c.cacheTTLs.Wikidata = ttls.Wikidata
		}
		if ttls.Commons > 0 {
			c.cacheTTLs.Commons = ttls.Commons
		}
		if ttls.Wikipedia > 0 {
			c.cacheTTLs.Wikipedia = ttls.Wikipedia
		}
	}
}

// FetchOption configures one aggregate request.
type FetchOption func(*fetchConfig)
type fetchConfig struct {
	directMedia       bool
	categories        bool
	wikipedia         bool
	claims            bool
	rawEntity         bool
	deprecated        bool
	mediaLimit        int
	thumbnailWidth    int
	articleLimit      int
	categoryPageLimit int
	mediaProperties   []MediaProperty
}

func defaultFetchConfig() fetchConfig {
	return fetchConfig{directMedia: true, claims: true, mediaLimit: 20, thumbnailWidth: 1200, articleLimit: 1, categoryPageLimit: 3, mediaProperties: DefaultMediaProperties()}
}

// WithDirectMedia controls direct Wikidata media resolution.
func WithDirectMedia(value bool) FetchOption { return func(c *fetchConfig) { c.directMedia = value } }

// WithCommonsCategories enables bounded direct-file enrichment from linked Commons categories.
func WithCommonsCategories(value bool) FetchOption {
	return func(c *fetchConfig) { c.categories = value }
}

// WithWikipediaSummaries enables introductory extracts from preferred Wikipedias.
func WithWikipediaSummaries(value bool) FetchOption {
	return func(c *fetchConfig) { c.wikipedia = value }
}

// WithClaims controls whether generic Wikidata claims are included in Result.
func WithClaims(value bool) FetchOption { return func(c *fetchConfig) { c.claims = value } }

// WithRawEntity includes the raw Wikidata entity JSON in Result.
func WithRawEntity(value bool) FetchOption { return func(c *fetchConfig) { c.rawEntity = value } }

// WithDeprecatedStatements includes deprecated statements in output and media discovery.
func WithDeprecatedStatements(value bool) FetchOption {
	return func(c *fetchConfig) { c.deprecated = value }
}

// WithMediaLimit limits ranked media output; zero disables media enrichment.
func WithMediaLimit(value int) FetchOption { return func(c *fetchConfig) { c.mediaLimit = value } }

// WithThumbnailWidth sets the requested Commons thumbnail width.
func WithThumbnailWidth(value int) FetchOption {
	return func(c *fetchConfig) { c.thumbnailWidth = value }
}

// WithArticleLimit limits optional Wikipedia extracts.
func WithArticleLimit(value int) FetchOption { return func(c *fetchConfig) { c.articleLimit = value } }

// WithCategoryPageLimit limits pages fetched per linked Commons category.
func WithCategoryPageLimit(value int) FetchOption {
	return func(c *fetchConfig) { c.categoryPageLimit = value }
}

// WithMediaProperties replaces the built-in Wikidata media-property registry.
func WithMediaProperties(values ...MediaProperty) FetchOption {
	return func(c *fetchConfig) { c.mediaProperties = append([]MediaProperty(nil), values...) }
}

// WithAdditionalMediaProperties appends entries to the media-property registry.
func WithAdditionalMediaProperties(values ...MediaProperty) FetchOption {
	return func(c *fetchConfig) { c.mediaProperties = append(c.mediaProperties, values...) }
}

func normalizeLanguages(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return []string{"en"}
	}
	return result
}
