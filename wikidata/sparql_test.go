package wikidata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/karte-bayern/wikimedia/mediawiki"
)

func TestSPARQLQueryPostsAndDecodesBindings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		if got := r.Header.Get("Accept"); got != "application/sparql-results+json" {
			t.Errorf("Accept=%q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "sparql-test/1.0 (test@example.org)" {
			t.Errorf("User-Agent=%q", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("query"); got != "SELECT ?item WHERE { ?item wdt:P31 wd:Q515 }" {
			t.Errorf("query=%q", got)
		}
		_, _ = w.Write([]byte(`{"head":{"vars":["item"]},"results":{"bindings":[{"item":{"type":"uri","value":"http://www.wikidata.org/entity/Q64"}}]}}`))
	}))
	defer server.Close()
	client, err := NewSPARQLClient(WithSPARQLEndpoint(server.URL), WithSPARQLUserAgent("sparql-test/1.0 (test@example.org)"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Query(context.Background(), " SELECT ?item WHERE { ?item wdt:P31 wd:Q515 } ")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Variables) != 1 || result.Variables[0] != "item" || result.Bindings[0]["item"].Value != "http://www.wikidata.org/entity/Q64" {
		t.Fatalf("result=%+v", result)
	}
}

func TestSPARQLQueryPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("query") == "OFFSET 0 LIMIT 2" {
			_, _ = w.Write([]byte(`{"head":{"vars":["item"]},"results":{"bindings":[{"item":{"type":"uri","value":"Q1"}},{"item":{"type":"uri","value":"Q2"}}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"head":{"vars":["item"]},"results":{"bindings":[{"item":{"type":"uri","value":"Q3"}}]}}`))
	}))
	defer server.Close()
	client, err := NewSPARQLClient(WithSPARQLEndpoint(server.URL), WithSPARQLUserAgent("sparql-test/1.0 (test@example.org)"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	err = client.QueryPages(context.Background(), 2, 5, func(offset, limit int) string {
		return "OFFSET " + strconv.Itoa(offset) + " LIMIT " + strconv.Itoa(limit)
	}, func(page *SPARQLResult) error {
		for _, row := range page.Bindings {
			got = append(got, row["item"].Value)
		}
		return nil
	})
	if err != nil || len(got) != 3 || got[2] != "Q3" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestSPARQLQueryRejectsInvalidQuery(t *testing.T) {
	client, err := NewSPARQLClient(WithSPARQLUserAgent("sparql-test/1.0 (test@example.org)"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Query(context.Background(), " \n "); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("err=%v", err)
	}
}

func TestSPARQLQueryRetriesRateLimit(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"head":{"vars":["item"]},"results":{"bindings":[]}}`))
	}))
	defer server.Close()
	client, err := NewSPARQLClient(
		WithSPARQLEndpoint(server.URL), WithSPARQLUserAgent("sparql-test/1.0 (test@example.org)"),
		WithSPARQLRetryPolicy(mediawiki.RetryPolicy{MaxAttempts: 2, InitialBackoff: 0, MaxBackoff: 0}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Query(context.Background(), "SELECT * WHERE {}"); err != nil || attempts != 2 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
}

func TestSPARQLRetryAfterAndBackoff(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := sparqlRetryAfter("2", now); got != 2*time.Second {
		t.Fatalf("retry after=%s", got)
	}
	if got := sparqlRetryAfter(now.Add(3*time.Second).Format(http.TimeFormat), now); got != 3*time.Second {
		t.Fatalf("date retry after=%s", got)
	}
	if got := sparqlBackoff(mediawiki.RetryPolicy{InitialBackoff: time.Second, MaxBackoff: 3 * time.Second}, 3); got != 3*time.Second {
		t.Fatalf("backoff=%s", got)
	}
}
