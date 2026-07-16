package mediawiki

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testCache struct {
	mu     sync.Mutex
	values map[string][]byte
}

func (c *testCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.values[key]
	return append([]byte(nil), v...), ok, nil
}
func (c *testCache) Set(_ context.Context, key string, value []byte, _ time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.values == nil {
		c.values = map[string][]byte{}
	}
	c.values[key] = append([]byte(nil), value...)
	return nil
}
func (c *testCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	delete(c.values, key)
	c.mu.Unlock()
	return nil
}

func TestQueryAddsProtocolParametersAndCaches(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if got := r.Header.Get("User-Agent"); got != "test-client/1.0 (test@example.org)" {
			t.Errorf("User-Agent=%q", got)
		}
		if got := r.URL.Query().Get("maxlag"); got != "7" {
			t.Errorf("maxlag=%q", got)
		}
		if got := r.URL.Query().Get("formatversion"); got != "2" {
			t.Errorf("formatversion=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":"ok"}`))
	}))
	defer server.Close()
	storage := &testCache{values: map[string][]byte{}}
	client, err := NewClient(server.URL, WithUserAgent("test-client/1.0 (test@example.org)"), WithMaxLag(7), WithCache(storage, time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		var result struct {
			Value string `json:"value"`
		}
		raw, err := client.Query(context.Background(), url.Values{"action": {"query"}}, &result)
		if err != nil {
			t.Fatal(err)
		}
		if result.Value != "ok" || len(raw) == 0 {
			t.Fatalf("result=%+v raw=%q", result, raw)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls=%d", got)
	}
}

func TestQueryRetriesMaxLagAndRateLimit(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1:
			_, _ = w.Write([]byte(`{"error":{"code":"maxlag","info":"lagged"}}`))
		case 2:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("slow down"))
		default:
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, WithUserAgent("retry-test/1.0 (test@example.org)"), WithRetryPolicy(RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond}))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		OK bool `json:"ok"`
	}
	if _, err := client.Query(context.Background(), url.Values{"action": {"query"}}, &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || calls.Load() != 3 {
		t.Fatalf("result=%+v calls=%d", result, calls.Load())
	}
}

func TestQueryResponseLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"long":"0123456789"}`)) }))
	defer server.Close()
	client, err := NewClient(server.URL, WithUserAgent("limit-test/1.0 (test@example.org)"), WithMaxResponseBytes(5), WithRetryPolicy(RetryPolicy{MaxAttempts: 1}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Query(context.Background(), url.Values{"action": {"query"}}, &struct{}{})
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateUserAgent(t *testing.T) {
	for _, value := range []string{"", "curl/8", "Mozilla/5.0"} {
		if err := ValidateUserAgent(value); !errors.Is(err, ErrInvalidUserAgent) {
			t.Errorf("%q err=%v", value, err)
		}
	}
	if err := ValidateUserAgent("map-importer/1.0 (https://example.org/contact)"); err != nil {
		t.Fatal(err)
	}
}
