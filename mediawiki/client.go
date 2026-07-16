package mediawiki

import (
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
)

// Client is a read-only MediaWiki Action API client.
type Client struct {
	endpoint         string
	httpClient       HTTPDoer
	userAgent        string
	maxLag           int
	maxResponseBytes int64
	retry            RetryPolicy
	cache            Cache
	cacheTTL         time.Duration
	now              func() time.Time
}

// NewClient creates a client for one Action API endpoint.
func NewClient(endpoint string, options ...Option) (*Client, error) {
	cfg := defaultConfig()
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("mediawiki: invalid endpoint %q", endpoint)
	}
	if err := ValidateUserAgent(cfg.userAgent); err != nil {
		return nil, err
	}
	if cfg.httpClient == nil {
		return nil, errors.New("mediawiki: nil HTTP client")
	}
	if cfg.maxResponseBytes <= 0 {
		return nil, errors.New("mediawiki: response limit must be positive")
	}
	if cfg.retry.MaxAttempts <= 0 {
		cfg.retry.MaxAttempts = 1
	}
	if cfg.retry.InitialBackoff < 0 || cfg.retry.MaxBackoff < 0 {
		return nil, errors.New("mediawiki: retry delays must not be negative")
	}
	if cfg.retry.MaxBackoff == 0 {
		cfg.retry.MaxBackoff = cfg.retry.InitialBackoff
	}
	return &Client{
		endpoint: parsed.String(), httpClient: cfg.httpClient, userAgent: cfg.userAgent,
		maxLag: cfg.maxLag, maxResponseBytes: cfg.maxResponseBytes, retry: cfg.retry,
		cache: cfg.cache, cacheTTL: cfg.cacheTTL, now: time.Now,
	}, nil
}

// Endpoint returns the configured Action API endpoint.
func (c *Client) Endpoint() string { return c.endpoint }

// Query performs a GET request, decodes JSON into target, and returns the raw body.
func (c *Client) Query(ctx context.Context, parameters url.Values, target any) ([]byte, error) {
	if c == nil {
		return nil, errors.New("mediawiki: nil client")
	}
	values := cloneValues(parameters)
	values.Set("format", "json")
	values.Set("formatversion", "2")
	if c.maxLag >= 0 && values.Get("maxlag") == "" {
		values.Set("maxlag", strconv.Itoa(c.maxLag))
	}
	requestURL, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	requestURL.RawQuery = values.Encode()
	cacheKey := requestURL.String()
	if c.cache != nil {
		body, found, cacheErr := c.cache.Get(ctx, cacheKey)
		if cacheErr != nil {
			return nil, fmt.Errorf("mediawiki: cache get: %w", cacheErr)
		}
		if found {
			if err := decodeResponse(body, target); err != nil {
				_ = c.cache.Delete(ctx, cacheKey)
				return nil, err
			}
			return append([]byte(nil), body...), nil
		}
	}

	var lastErr error
	for attempt := 1; attempt <= c.retry.MaxAttempts; attempt++ {
		body, retryAfter, retryable, err := c.queryOnce(ctx, requestURL, target)
		if err == nil {
			if c.cache != nil {
				expires := c.now().Add(c.cacheTTL)
				if cacheErr := c.cache.Set(ctx, cacheKey, body, expires); cacheErr != nil {
					return nil, fmt.Errorf("mediawiki: cache set: %w", cacheErr)
				}
			}
			return body, nil
		}
		lastErr = err
		if !retryable || attempt == c.retry.MaxAttempts {
			break
		}
		delay := retryAfter
		if delay <= 0 {
			delay = c.backoff(attempt)
		}
		if err := sleepContext(ctx, delay); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) queryOnce(ctx context.Context, requestURL *url.URL, target any) ([]byte, time.Duration, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return nil, 0, false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, 0, false, ctxErr
		}
		return nil, 0, true, fmt.Errorf("mediawiki: request: %w", err)
	}
	defer resp.Body.Close()
	retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"), c.now())
	body, err := readLimited(resp.Body, c.maxResponseBytes)
	if err != nil {
		return nil, retryAfter, false, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		cause := error(nil)
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		if resp.StatusCode == http.StatusTooManyRequests {
			cause = ErrRateLimited
		}
		message := strings.TrimSpace(string(body))
		if len(message) > 300 {
			message = message[:300]
		}
		apiErr := &APIError{Endpoint: c.endpoint, StatusCode: resp.StatusCode, Message: message, RetryAfter: retryAfter, cause: cause}
		return nil, retryAfter, retryable, apiErr
	}
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
			Info string `json:"info"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, 0, false, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}
	if envelope.Error != nil {
		cause := error(nil)
		retryable := false
		if strings.EqualFold(envelope.Error.Code, "maxlag") {
			cause, retryable = ErrMaxLag, true
		}
		apiErr := &APIError{Endpoint: c.endpoint, Code: envelope.Error.Code, Message: envelope.Error.Info, RetryAfter: retryAfter, cause: cause}
		return nil, retryAfter, retryable, apiErr
	}
	if err := decodeResponse(body, target); err != nil {
		return nil, 0, false, err
	}
	return body, 0, false, nil
}

func decodeResponse(body []byte, target any) error {
	if target == nil {
		return nil
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}
	return nil
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("mediawiki: read response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, ErrResponseTooLarge
	}
	return body, nil
}

func cloneValues(source url.Values) url.Values {
	result := make(url.Values, len(source))
	for key, values := range source {
		result[key] = append([]string(nil), values...)
	}
	return result
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := when.Sub(now); delay > 0 {
			return delay
		}
	}
	return 0
}

func (c *Client) backoff(attempt int) time.Duration {
	delay := c.retry.InitialBackoff
	for i := 1; i < attempt && delay < c.retry.MaxBackoff; i++ {
		delay *= 2
		if delay > c.retry.MaxBackoff {
			delay = c.retry.MaxBackoff
		}
	}
	return delay
}

func sleepContext(ctx context.Context, delay time.Duration) error {
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
