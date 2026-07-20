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

// SPARQLClient queries a SPARQL endpoint and decodes SPARQL Results JSON.
type SPARQLClient struct {
	endpoint         string
	httpClient       mediawiki.HTTPDoer
	userAgent        string
	maxResponseBytes int64
}

// SPARQLOption configures a SPARQLClient.
type SPARQLOption func(*sparqlConfig)

type sparqlConfig struct {
	endpoint         string
	httpClient       mediawiki.HTTPDoer
	userAgent        string
	maxResponseBytes int64
}

// NewSPARQLClient creates a bounded client for a SPARQL Results JSON endpoint.
func NewSPARQLClient(options ...SPARQLOption) (*SPARQLClient, error) {
	cfg := sparqlConfig{endpoint: defaultSPARQLEndpoint, httpClient: &http.Client{Timeout: 45 * time.Second}, maxResponseBytes: 64 << 20}
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
	return &SPARQLClient{endpoint: parsed.String(), httpClient: cfg.httpClient, userAgent: cfg.userAgent, maxResponseBytes: cfg.maxResponseBytes}, nil
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

// Query submits a SPARQL query by POST and decodes the standard JSON result.
func (c *SPARQLClient) Query(ctx context.Context, query string) (*SPARQLResult, error) {
	if c == nil || c.httpClient == nil {
		return nil, errors.New("wikidata: nil SPARQL client")
	}
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 1<<20 {
		return nil, ErrInvalidQuery
	}
	body := url.Values{"query": {query}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/sparql-results+json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("wikidata: SPARQL request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("wikidata: read SPARQL response: %w", err)
	}
	if int64(len(raw)) > c.maxResponseBytes {
		return nil, errors.New("wikidata: SPARQL response too large")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(raw))
		if len(message) > 300 {
			message = message[:300]
		}
		return nil, fmt.Errorf("wikidata: SPARQL HTTP %d: %s", resp.StatusCode, message)
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
