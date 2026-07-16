package download

import (
	"errors"
	"net/url"
	"time"

	"github.com/karte-bayern/wikimedia/mediawiki"
)

// URLValidator can apply application-specific URL restrictions after built-in validation.
type URLValidator func(*url.URL) error

type config struct {
	httpClient     mediawiki.HTTPDoer
	userAgent      string
	maximumBytes   int64
	allowedHosts   map[string]struct{}
	allowedSchemes map[string]struct{}
	overwrite      bool
	strictMIME     bool
	timeout        time.Duration
	validator      URLValidator
}

func defaultConfig() config {
	return config{maximumBytes: 50 << 20, allowedHosts: map[string]struct{}{"upload.wikimedia.org": {}}, allowedSchemes: map[string]struct{}{"https": {}}, timeout: 2 * time.Minute}
}

// Option configures a Downloader and may reject invalid values.
type Option func(*config) error

// WithHTTPClient injects an HTTP transport.
func WithHTTPClient(value mediawiki.HTTPDoer) Option {
	return func(c *config) error {
		if value != nil {
			c.httpClient = value
		}
		return nil
	}
}

// WithUserAgent sets the descriptive application User-Agent.
func WithUserAgent(value string) Option {
	return func(c *config) error { c.userAgent = value; return nil }
}

// WithMaximumBytes sets the maximum advertised and observed size per file.
func WithMaximumBytes(value int64) Option {
	return func(c *config) error {
		if value <= 0 {
			return errors.New("download: maximum bytes must be positive")
		}
		c.maximumBytes = value
		return nil
	}
}

// WithAllowedHosts replaces the exact hostname allow-list.
func WithAllowedHosts(values ...string) Option {
	return func(c *config) error {
		c.allowedHosts = stringSet(values)
		if len(c.allowedHosts) == 0 {
			return errors.New("download: host allow-list must not be empty")
		}
		return nil
	}
}

// WithAllowedSchemes replaces the URL scheme allow-list.
func WithAllowedSchemes(values ...string) Option {
	return func(c *config) error {
		c.allowedSchemes = stringSet(values)
		if len(c.allowedSchemes) == 0 {
			return errors.New("download: scheme allow-list must not be empty")
		}
		return nil
	}
}

// WithOverwrite permits replacement of an existing regular destination.
func WithOverwrite(value bool) Option {
	return func(c *config) error { c.overwrite = value; return nil }
}

// WithStrictMIME requires a supplied MIME type to match the response MIME type.
func WithStrictMIME(value bool) Option {
	return func(c *config) error { c.strictMIME = value; return nil }
}

// WithTimeout sets the built-in HTTP client timeout.
func WithTimeout(value time.Duration) Option {
	return func(c *config) error {
		if value <= 0 {
			return errors.New("download: timeout must be positive")
		}
		c.timeout = value
		return nil
	}
}

// WithURLValidator adds application-specific validation after built-in checks.
func WithURLValidator(value URLValidator) Option {
	return func(c *config) error { c.validator = value; return nil }
}
