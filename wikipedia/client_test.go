package wikipedia

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/de") {
			t.Errorf("path=%q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("explaintext") != "1" || q.Get("redirects") != "1" {
			t.Errorf("query=%v", q)
		}
		_, _ = w.Write([]byte(`{"query":{"pages":[{"pageid":42,"title":"Brandenburger Tor","fullurl":"https://de.wikipedia.org/wiki/Brandenburger_Tor","description":"Tor in Berlin","extract":"Das Brandenburger Tor ist ein Bauwerk.","thumbnail":{"source":"https://upload.wikimedia.org/thumb.jpg","width":1200,"height":800}}]}}`))
	}))
	defer server.Close()
	client, err := NewClient(WithUserAgent("wp-test/1.0 (test@example.org)"), WithEndpointTemplate(server.URL+"/%s"))
	if err != nil {
		t.Fatal(err)
	}
	article, err := client.GetSummary(context.Background(), "de", "Brandenburger Tor")
	if err != nil {
		t.Fatal(err)
	}
	if article.PageID != 42 || article.Language != "de" || !strings.Contains(article.Extract, "Bauwerk") || article.Thumbnail.Width != 1200 {
		t.Fatalf("article=%+v", article)
	}
}

func TestGetSummaryValidationAndMissing(t *testing.T) {
	client, err := NewClient(WithUserAgent("wp-test/1.0 (test@example.org)"), WithEndpointTemplate("https://%s.example.invalid/api"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetSummary(context.Background(), "de/evil", "Title"); !errors.Is(err, ErrInvalidLanguage) {
		t.Fatalf("err=%v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"query":{"pages":[{"title":"Missing","missing":true}]}}`))
	}))
	defer server.Close()
	client, err = NewClient(WithUserAgent("wp-test/1.0 (test@example.org)"), WithEndpointTemplate(server.URL+"/%s"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetSummary(context.Background(), "en", "Missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}
