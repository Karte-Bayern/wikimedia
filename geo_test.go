package wikimedia

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFindNearbyAndBoundingBox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		query := r.Form.Get("query")
		if !strings.Contains(query, "wikibase:around") && !strings.Contains(query, "wikibase:box") {
			t.Fatalf("query=%s", query)
		}
		_, _ = w.Write([]byte(`{"head":{"vars":["item","itemLabel","location","distance"]},"results":{"bindings":[{"item":{"type":"uri","value":"http://www.wikidata.org/entity/Q64"},"itemLabel":{"type":"literal","value":"Berlin"},"location":{"type":"literal","value":"Point(13.4 52.5)"},"distance":{"type":"literal","value":"1.2"}}]}}`))
	}))
	defer server.Close()
	client, err := New(WithUserAgent("geo-test/1.0 (test@example.org)"), WithSPARQLEndpoint(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	nearby, err := client.FindNearby(context.Background(), 52.5, 13.4, 5, 10)
	if err != nil || len(nearby) != 1 || nearby[0].ID != "Q64" || nearby[0].Coordinate == nil || nearby[0].DistanceKM == nil {
		t.Fatalf("nearby=%+v err=%v", nearby, err)
	}
	boxed, err := client.FindInBoundingBox(context.Background(), 52.4, 13.3, 52.6, 13.5, 10)
	if err != nil || len(boxed) != 1 || boxed[0].ID != "Q64" {
		t.Fatalf("boxed=%+v err=%v", boxed, err)
	}
	if _, err := client.FindNearby(context.Background(), 91, 13.4, 5, 10); err == nil {
		t.Fatal("invalid point was accepted")
	}
}
