package wikidata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/karte-bayern/wikimedia/mediawiki"
)

const defaultSPARQLEndpoint = "https://query.wikidata.org/sparql"

var ErrInvalidQuery = errors.New("wikidata: invalid SPARQL query")

// SPARQLBinding is one RDF term in a SPARQL JSON result binding.
type SPARQLBinding struct {
	Type     string `json:"type"`
	Value    string `json:"value"`
	Datatype string `json:"datatype,omitempty"`
	Language string `json:"xml:lang,omitempty"`
}

// SPARQLResult is a SPARQL Query Results JSON document.
type SPARQLResult struct {
	Variables []string                   `json:"variables"`
	Bindings  []map[string]SPARQLBinding `json:"bindings"`
}

// SPARQLPageQuery creates a query for the supplied OFFSET and LIMIT values.
// The returned query must apply both values for QueryPages to be bounded.
type SPARQLPageQuery func(offset, limit int) string

// SPARQLPageVisitor receives one decoded page. Returning an error stops the
// iteration and returns that error to the caller.
type SPARQLPageVisitor func(*SPARQLResult) error

// SPARQLClient queries a SPARQL endpoint and decodes SPARQL Results JSON.
type SPARQLClient struct {
	endpoint         string
	httpClient       mediawiki.HTTPDoer
	userAgent        string
	maxResponseBytes int64
	retry            mediawiki.RetryPolicy
	now              func() time.Time
}

// SPARQLOption configures a SPARQLClient.
type SPARQLOption func(*sparqlConfig)

type sparqlConfig struct {
	endpoint         string
	httpClient       mediawiki.HTTPDoer
	userAgent        string
	maxResponseBytes int64
	retry            mediawiki.RetryPolicy
}

// NewSPARQLClient creates a bounded client for a SPARQL Results JSON endpoint.
func NewSPARQLClient(options ...SPARQLOption) (*SPARQLClient, error) {
	cfg := sparqlConfig{endpoint: defaultSPARQLEndpoint, httpClient: &http.Client{Timeout: 45 * time.Second}, maxResponseBytes: 64 << 20, retry: mediawiki.RetryPolicy{MaxAttempts: 3, InitialBackoff: 250 * time.Millisecond, MaxBackoff: 4 * time.Second}}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	parsed, err := url.Parse(strings.TrimSpace(cfg.endpoint))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("wikidata: invalid SPARQL endpoint %q", cfg.endpoint)
	}
	if err := mediawiki.ValidateUserAgent(cfg.userAgent); err != nil {
		return nil, err
	}
	if cfg.httpClient == nil {
		return nil, errors.New("wikidata: nil SPARQL HTTP client")
	}
	if cfg.maxResponseBytes <= 0 {
		return nil, errors.New("wikidata: SPARQL response limit must be positive")
	}
	if cfg.retry.MaxAttempts <= 0 {
		cfg.retry.MaxAttempts = 1
	}
	if cfg.retry.InitialBackoff < 0 || cfg.retry.MaxBackoff < 0 {
		return nil, errors.New("wikidata: SPARQL retry delays must not be negative")
	}
	if cfg.retry.MaxBackoff == 0 {
		cfg.retry.MaxBackoff = cfg.retry.InitialBackoff
	}
	return &SPARQLClient{endpoint: parsed.String(), httpClient: cfg.httpClient, userAgent: cfg.userAgent, maxResponseBytes: cfg.maxResponseBytes, retry: cfg.retry, now: time.Now}, nil
}

// WithSPARQLEndpoint overrides the SPARQL endpoint.
func WithSPARQLEndpoint(value string) SPARQLOption {
	return func(c *sparqlConfig) { c.endpoint = value }
}

// WithSPARQLHTTPClient injects an HTTP transport.
func WithSPARQLHTTPClient(value mediawiki.HTTPDoer) SPARQLOption {
	return func(c *sparqlConfig) {
		if value != nil {
			c.httpClient = value
		}
	}
}

// WithSPARQLUserAgent sets the descriptive application User-Agent.
func WithSPARQLUserAgent(value string) SPARQLOption {
	return func(c *sparqlConfig) { c.userAgent = value }
}

// WithSPARQLMaxResponseBytes limits the response body before decoding.
func WithSPARQLMaxResponseBytes(value int64) SPARQLOption {
	return func(c *sparqlConfig) { c.maxResponseBytes = value }
}

// WithSPARQLRetryPolicy configures bounded retries for rate limiting and
// transient endpoint failures.
func WithSPARQLRetryPolicy(value mediawiki.RetryPolicy) SPARQLOption {
	return func(c *sparqlConfig) { c.retry = value }
}

// Query submits a SPARQL query by POST and decodes the standard JSON result.
func (c *SPARQLClient) Query(ctx context.Context, query string) (*SPARQLResult, error) {
	if c == nil || c.httpClient == nil {
		return nil, errors.New("wikidata: nil SPARQL client")
	}
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 1<<20 {
		return nil, ErrInvalidQuery
	}
	var raw []byte
	var lastErr error
	for attempt := 1; attempt <= c.retry.MaxAttempts; attempt++ {
		var retryAfter time.Duration
		var retryable bool
		raw, retryAfter, retryable, lastErr = c.queryOnce(ctx, query)
		if lastErr == nil {
			break
		}
		if !retryable || attempt == c.retry.MaxAttempts {
			return nil, lastErr
		}
		delay := retryAfter
		if delay <= 0 {
			delay = sparqlBackoff(c.retry, attempt)
		}
		if err := sleepSPARQL(ctx, delay); err != nil {
			return nil, err
		}
	}
	var envelope struct {
		Head struct {
			Variables []string `json:"vars"`
		} `json:"head"`
		Results struct {
			Bindings []map[string]SPARQLBinding `json:"bindings"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("wikidata: decode SPARQL response: %w", err)
	}
	return &SPARQLResult{Variables: envelope.Head.Variables, Bindings: envelope.Results.Bindings}, nil
}

func (c *SPARQLClient) queryOnce(ctx context.Context, query string) ([]byte, time.Duration, bool, error) {
	body := url.Values{"query": {query}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewBufferString(body))
	if err != nil {
		return nil, 0, false, err
	}
	req.Header.Set("Accept", "application/sparql-results+json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, false, ctx.Err()
		}
		return nil, 0, true, fmt.Errorf("wikidata: SPARQL request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.maxResponseBytes+1))
	if err != nil {
		return nil, 0, false, fmt.Errorf("wikidata: read SPARQL response: %w", err)
	}
	if int64(len(raw)) > c.maxResponseBytes {
		return nil, 0, false, errors.New("wikidata: SPARQL response too large")
	}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return raw, 0, false, nil
	}
	message := strings.TrimSpace(string(raw))
	if len(message) > 300 {
		message = message[:300]
	}
	retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError
	return nil, sparqlRetryAfter(resp.Header.Get("Retry-After"), c.now()), retryable, fmt.Errorf("wikidata: SPARQL HTTP %d: %s", resp.StatusCode, message)
}

func sparqlRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil && when.After(now) {
		return when.Sub(now)
	}
	return 0
}

func sparqlBackoff(policy mediawiki.RetryPolicy, attempt int) time.Duration {
	delay := policy.InitialBackoff
	for step := 1; step < attempt && delay < policy.MaxBackoff; step++ {
		delay *= 2
	}
	if policy.MaxBackoff > 0 && delay > policy.MaxBackoff {
		return policy.MaxBackoff
	}
	return delay
}

func sleepSPARQL(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// QueryPages invokes build for consecutive OFFSET/LIMIT values and stops when
// the returned page has fewer bindings than pageSize or maxPages is reached.
// It intentionally does not rewrite arbitrary SPARQL strings, which could
// invalidate nested queries or existing LIMIT/OFFSET clauses.
func (c *SPARQLClient) QueryPages(ctx context.Context, pageSize, maxPages int, build SPARQLPageQuery, visit SPARQLPageVisitor) error {
	if pageSize <= 0 || pageSize > 10000 || maxPages <= 0 {
		return ErrInvalidQuery
	}
	if build == nil || visit == nil {
		return ErrInvalidQuery
	}
	for pageNumber := 0; pageNumber < maxPages; pageNumber++ {
		query := build(pageNumber*pageSize, pageSize)
		result, err := c.Query(ctx, query)
		if err != nil {
			return err
		}
		if err := visit(result); err != nil {
			return err
		}
		if len(result.Bindings) < pageSize {
			return nil
		}
	}
	return nil
}
