package wikidata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestSPARQLQueryRejectsInvalidQuery(t *testing.T) {
	client, err := NewSPARQLClient(WithSPARQLUserAgent("sparql-test/1.0 (test@example.org)"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Query(context.Background(), " \n "); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("err=%v", err)
	}
}
