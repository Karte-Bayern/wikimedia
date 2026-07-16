package wikidata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetEntityAndValueHelpers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("action"); got != "wbgetentities" {
			t.Errorf("action=%q", got)
		}
		if got := r.URL.Query().Get("languages"); got != "de|en" {
			t.Errorf("languages=%q", got)
		}
		_, _ = w.Write([]byte(`{"entities":{"Q82425":{"pageid":42,"id":"Q82425","labels":{"de":{"language":"de","value":"Brandenburger Tor"}},"descriptions":{"de":{"language":"de","value":"Tor in Berlin"}},"aliases":{"de":[{"language":"de","value":"Brandenburg Gate"}]},"claims":{"P18":[{"id":"Q82425$P18","type":"statement","rank":"preferred","mainsnak":{"snaktype":"value","property":"P18","datatype":"commonsMedia","datavalue":{"type":"string","value":"Gate.jpg"}}}],"P625":[{"id":"coord","rank":"normal","mainsnak":{"snaktype":"value","property":"P625","datatype":"globe-coordinate","datavalue":{"type":"globecoordinate","value":{"latitude":52.5,"longitude":13.3,"precision":0.1,"globe":"http://www.wikidata.org/entity/Q2"}}}}]},"sitelinks":{"dewiki":{"site":"dewiki","title":"Brandenburger Tor","badges":[]}}}}}`))
	}))
	defer server.Close()
	client, err := NewClient(WithEndpoint(server.URL), WithUserAgent("wd-test/1.0 (test@example.org)"), WithLanguages("de", "en"))
	if err != nil {
		t.Fatal(err)
	}
	entity, err := client.GetEntity(context.Background(), "Q82425")
	if err != nil {
		t.Fatal(err)
	}
	if entity.Labels["de"].Value != "Brandenburger Tor" || len(entity.Raw) == 0 {
		t.Fatalf("entity=%+v raw=%q", entity, entity.Raw)
	}
	image, ok := entity.Claims["P18"][0].MainSnak.StringValue()
	if !ok || image != "Gate.jpg" {
		t.Fatalf("image=%q ok=%v", image, ok)
	}
	coordinate, ok := entity.Claims["P625"][0].MainSnak.CoordinateValue()
	if !ok || coordinate.Latitude != 52.5 {
		t.Fatalf("coordinate=%+v ok=%v", coordinate, ok)
	}
	var raw map[string]any
	if err := json.Unmarshal(entity.Raw, &raw); err != nil {
		t.Fatal(err)
	}
}

func TestGetEntitiesValidationAndMissing(t *testing.T) {
	client, err := NewClient(WithEndpoint("https://example.invalid/api.php"), WithUserAgent("wd-test/1.0 (test@example.org)"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetEntity(context.Background(), "Q0"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("err=%v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"entities":{"Q1":{"id":"Q1","missing":true}}}`))
	}))
	defer server.Close()
	client, err = NewClient(WithEndpoint(server.URL), WithUserAgent("wd-test/1.0 (test@example.org)"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetEntity(context.Background(), "Q1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestSnakTypedValues(t *testing.T) {
	cases := []struct {
		raw   string
		check func(Snak) bool
	}{
		{`{"snaktype":"value","datavalue":{"value":{"entity-type":"item","numeric-id":64},"type":"wikibase-entityid"}}`, func(s Snak) bool { v, ok := s.EntityValue(); return ok && v.ID == "Q64" }},
		{`{"snaktype":"value","datavalue":{"value":{"time":"+1791-01-01T00:00:00Z","precision":9},"type":"time"}}`, func(s Snak) bool { v, ok := s.TimeValue(); return ok && v.Precision == 9 }},
		{`{"snaktype":"value","datavalue":{"value":{"amount":"+42","unit":"1"},"type":"quantity"}}`, func(s Snak) bool { v, ok := s.QuantityValue(); return ok && v.Amount == "+42" }},
		{`{"snaktype":"value","datavalue":{"value":{"text":"Tor","language":"de"},"type":"monolingualtext"}}`, func(s Snak) bool { v, ok := s.MonolingualTextValue(); return ok && v.Language == "de" }},
	}
	for _, test := range cases {
		var snak Snak
		if err := json.Unmarshal([]byte(test.raw), &snak); err != nil {
			t.Fatal(err)
		}
		if !test.check(snak) {
			t.Fatalf("failed for %s", test.raw)
		}
	}
}

func TestGetEntityTrimsIDAndPreservesRedirectInformation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("ids"); got != "Q4" {
			t.Errorf("ids=%q", got)
		}
		_, _ = w.Write([]byte(`{"entities":{"Q4":{"id":"Q1","redirected":"Q4","labels":{"en":{"language":"en","value":"target"}}}}}`))
	}))
	defer server.Close()
	client, err := NewClient(WithEndpoint(server.URL), WithUserAgent("wd-test/1.0 (test@example.org)"))
	if err != nil {
		t.Fatal(err)
	}
	entity, err := client.GetEntity(context.Background(), " Q4 ")
	if err != nil {
		t.Fatal(err)
	}
	if entity.ID != "Q1" || entity.RedirectedFrom != "Q4" {
		t.Fatalf("entity=%+v", entity)
	}
}
