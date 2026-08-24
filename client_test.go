package wikimedia

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/karte-bayern/wikimedia/mediawiki"
)

func TestFetchAggregatesDirectCategoryAndWikipedia(t *testing.T) {
	server := newFixtureServer(t, false)
	defer server.Close()
	client, err := New(
		WithUserAgent("aggregate-test/1.0 (test@example.org)"), WithLanguages("de", "en"),
		WithWikidataEndpoint(server.URL+"/wd"), WithCommonsEndpoint(server.URL+"/commons"),
		WithWikipediaEndpointTemplate(server.URL+"/wp/%s"), WithRetryPolicy(mediawiki.RetryPolicy{MaxAttempts: 1}),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Fetch(context.Background(), "Q82425", WithCommonsCategories(true), WithWikipediaSummaries(true), WithRawEntity(true), WithMediaLimit(10))
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "Q82425" || result.Label != "Brandenburger Tor" || result.Description != "Tor in Berlin" {
		t.Fatalf("identity=%+v", result)
	}
	if result.Coordinates == nil || result.Coordinates.Latitude != 52.5163 {
		t.Fatalf("coordinates=%+v", result.Coordinates)
	}
	if len(result.Commons) != 1 || result.Commons[0].Kind != "category" {
		t.Fatalf("commons=%+v", result.Commons)
	}
	if len(result.Media) != 2 {
		t.Fatalf("media=%+v", result.Media)
	}
	primary := result.PrimaryMedia()
	if primary == nil || primary.Title != "File:Gate-current.jpg" {
		t.Fatalf("primary=%+v media=%+v", primary, result.Media)
	}
	if len(primary.Sources) != 2 {
		t.Fatalf("sources=%+v", primary.Sources)
	}
	if primary.LicenseShortName != "CC BY-SA 4.0" || primary.Artist != "Alice" {
		t.Fatalf("metadata=%+v", primary)
	}
	if len(result.Articles) != 1 || !strings.Contains(result.Articles[0].Extract, "Wahrzeichen") {
		t.Fatalf("articles=%+v", result.Articles)
	}
	if len(result.RawEntity) == 0 || len(result.Warnings) != 0 {
		t.Fatalf("raw=%d warnings=%+v", len(result.RawEntity), result.Warnings)
	}
	if _, ok := result.Claims["P18"]; !ok {
		t.Fatal("P18 missing")
	}
}

func TestFetchReturnsPartialResultWhenCommonsFails(t *testing.T) {
	server := newFixtureServer(t, true)
	defer server.Close()
	client, err := New(WithUserAgent("aggregate-test/1.0 (test@example.org)"), WithLanguages("de"), WithWikidataEndpoint(server.URL+"/wd"), WithCommonsEndpoint(server.URL+"/commons"), WithWikipediaEndpointTemplate(server.URL+"/wp/%s"), WithRetryPolicy(mediawiki.RetryPolicy{MaxAttempts: 1}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Fetch(context.Background(), "Q82425")
	if err != nil {
		t.Fatal(err)
	}
	if result.Label != "Brandenburger Tor" || len(result.Media) != 0 || len(result.Warnings) != 1 || result.Warnings[0].Service != ServiceCommons {
		t.Fatalf("result=%+v", result)
	}
}

func TestFetchInvalidID(t *testing.T) {
	client, err := New(WithUserAgent("aggregate-test/1.0 (test@example.org)"))
	if err != nil {
		t.Fatal(err)
	}
	if client.SPARQL() == nil {
		t.Fatal("SPARQL client is nil")
	}
	_, err = client.Fetch(context.Background(), "Q0")
	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("err=%v", err)
	}
}

func TestFetchResolvesWikipediaAndOSMReferences(t *testing.T) {
	server := newFixtureServer(t, false)
	defer server.Close()
	client, err := New(
		WithUserAgent("aggregate-test/1.0 (test@example.org)"),
		WithWikidataEndpoint(server.URL+"/wd"), WithCommonsEndpoint(server.URL+"/commons"),
		WithRetryPolicy(mediawiki.RetryPolicy{MaxAttempts: 1}),
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, fetch := range map[string]func() (*Result, error){
		"Wikipedia": func() (*Result, error) {
			return client.FetchByWikipedia(context.Background(), "de", "Brandenburger Tor", WithMediaLimit(0))
		},
		"OSM relation": func() (*Result, error) {
			return client.FetchByOSM(context.Background(), OSMRelation, "62422", WithMediaLimit(0))
		},
		"OSM way": func() (*Result, error) {
			return client.FetchByOSM(context.Background(), OSMWay, "121590158", WithMediaLimit(0))
		},
		"OSM node": func() (*Result, error) {
			return client.FetchByOSM(context.Background(), OSMNode, "164979149", WithMediaLimit(0))
		},
	} {
		result, err := fetch()
		if err != nil || result.ID != "Q82425" {
			t.Fatalf("%s: result=%+v err=%v", name, result, err)
		}
	}
	if _, err := client.FetchByOSM(context.Background(), OSMRelation, "0"); !errors.Is(err, ErrInvalidOSMID) {
		t.Fatalf("invalid ID error=%v", err)
	}
}

func TestFetchByReferenceAcceptsCommonURLs(t *testing.T) {
	server := newFixtureServer(t, false)
	defer server.Close()
	client, err := New(
		WithUserAgent("aggregate-test/1.0 (test@example.org)"),
		WithWikidataEndpoint(server.URL+"/wd"), WithCommonsEndpoint(server.URL+"/commons"),
		WithRetryPolicy(mediawiki.RetryPolicy{MaxAttempts: 1}),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{
		"wikidata:Q82425",
		"https://www.wikidata.org/entity/Q82425",
		"https://de.wikipedia.org/wiki/Brandenburger_Tor",
		"https://de.m.wikipedia.org/wiki/Brandenburger_Tor",
		"https://www.openstreetmap.org/relation/62422",
		"way/121590158",
		"osm:node:164979149",
	} {
		result, err := client.FetchByReference(context.Background(), reference, WithMediaLimit(0))
		if err != nil || result.ID != "Q82425" {
			t.Fatalf("reference=%q result=%+v err=%v", reference, result, err)
		}
	}
	if _, err := client.FetchByReference(context.Background(), "https://example.org/item/1"); !errors.Is(err, ErrUnsupportedReference) {
		t.Fatalf("unsupported reference error=%v", err)
	}
}

func TestFetchManyBatchesQIDsAndKeepsItemErrors(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wd" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		calls++
		if got := r.URL.Query().Get("ids"); got != "Q1|Q2" {
			t.Fatalf("ids=%q", got)
		}
		_, _ = w.Write([]byte(`{"entities":{"Q1":{"id":"Q1","labels":{"en":{"language":"en","value":"One"}}},"Q2":{"id":"Q2","labels":{"en":{"language":"en","value":"Two"}}}}}`))
	}))
	defer server.Close()
	client, err := New(WithUserAgent("batch-test/1.0 (test@example.org)"), WithWikidataEndpoint(server.URL+"/wd"))
	if err != nil {
		t.Fatal(err)
	}
	items, err := client.FetchMany(context.Background(), []string{"Q1", "Q2", "Q1", "not-a-reference"}, WithMediaLimit(0))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(items) != 4 || items[0].Result.ID != "Q1" || items[1].Result.ID != "Q2" || items[2].Result.ID != "Q1" || items[3].Err == nil || items[3].Error == "" {
		t.Fatalf("calls=%d items=%+v", calls, items)
	}
}

func TestFetchByReferenceSupportsCommonsFilesAndCategories(t *testing.T) {
	server := newFixtureServer(t, false)
	defer server.Close()
	client, err := New(
		WithUserAgent("commons-reference-test/1.0 (test@example.org)"),
		WithWikidataEndpoint(server.URL+"/wd"), WithCommonsEndpoint(server.URL+"/commons"),
		WithRetryPolicy(mediawiki.RetryPolicy{MaxAttempts: 1}),
	)
	if err != nil {
		t.Fatal(err)
	}
	file, err := client.FetchByReference(context.Background(), "https://commons.wikimedia.org/wiki/File:old_gate.jpg")
	if err != nil || file.ID != "File:Gate-current.jpg" || len(file.Media) != 1 {
		t.Fatalf("file=%+v err=%v", file, err)
	}
	category, err := client.FetchByReference(context.Background(), "Category:Brandenburg Gate", WithMediaLimit(10))
	if err != nil || category.ID != "Category:Brandenburg Gate" || len(category.Media) != 2 {
		t.Fatalf("category=%+v err=%v", category, err)
	}
}

func TestRankAndLimitMedia(t *testing.T) {
	values := []Media{
		{Title: "File:Logo.svg", Kind: MediaKindLogo, MIMEType: "image/svg+xml", Width: 1000, Height: 1000, Sources: []MediaSource{{BaseScore: 350, MediaKind: MediaKindLogo}}},
		{Title: "File:Photo.jpg", Kind: MediaKindImage, MIMEType: "image/jpeg", Width: 2400, Height: 1600, LicenseURL: "https://example.test/license", Artist: "Alice", Sources: []MediaSource{{BaseScore: 1000, MediaKind: MediaKindImage}}},
	}
	values = rankAndLimitMedia(values, 1)
	if len(values) != 1 || values[0].Title != "File:Photo.jpg" || !values[0].Primary {
		t.Fatalf("values=%+v", values)
	}
}

func newFixtureServer(t *testing.T, commonsFailure bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/wd":
			if r.URL.Query().Get("list") == "search" {
				if got := r.URL.Query().Get("srsearch"); !strings.HasPrefix(got, "haswbstatement:P") {
					t.Errorf("statement search=%q", got)
				}
				_, _ = w.Write([]byte(`{"query":{"search":[{"title":"Q82425"}]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"entities":{"Q82425":{"pageid":42,"id":"Q82425","labels":{"de":{"language":"de","value":"Brandenburger Tor"},"en":{"language":"en","value":"Brandenburg Gate"}},"descriptions":{"de":{"language":"de","value":"Tor in Berlin"}},"aliases":{"de":[{"language":"de","value":"Brandenburg Gate"}]},"claims":{"P18":[{"id":"Q82425$P18","rank":"preferred","mainsnak":{"snaktype":"value","property":"P18","datatype":"commonsMedia","datavalue":{"type":"string","value":"old_gate.jpg"}}},{"id":"deprecated","rank":"deprecated","mainsnak":{"snaktype":"value","property":"P18","datatype":"commonsMedia","datavalue":{"type":"string","value":"Deprecated.jpg"}}}],"P625":[{"id":"coord","rank":"normal","mainsnak":{"snaktype":"value","property":"P625","datatype":"globe-coordinate","datavalue":{"type":"globecoordinate","value":{"latitude":52.5163,"longitude":13.3777,"precision":0.0001,"globe":"http://www.wikidata.org/entity/Q2"}}}}],"P373":[{"id":"category","rank":"normal","mainsnak":{"snaktype":"value","property":"P373","datatype":"string","datavalue":{"type":"string","value":"Brandenburg Gate"}}}],"P856":[{"id":"site","rank":"normal","mainsnak":{"snaktype":"value","property":"P856","datatype":"url","datavalue":{"type":"string","value":"https://example.test/gate"}}}]},"sitelinks":{"dewiki":{"site":"dewiki","title":"Brandenburger Tor","badges":[]},"commonswiki":{"site":"commonswiki","title":"Category:Brandenburg Gate","badges":[]}}}}}`))
		case r.URL.Path == "/commons":
			if commonsFailure {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("temporary"))
				return
			}
			if r.URL.Query().Get("generator") == "categorymembers" {
				_, _ = w.Write([]byte(`{"query":{"pages":[{"pageid":100,"ns":6,"title":"File:Gate-current.jpg","imageinfo":[{"size":1000000,"width":3000,"height":2000,"sha1":"gatehash","url":"https://upload.wikimedia.org/gate.jpg","descriptionurl":"https://commons.wikimedia.org/wiki/File:Gate-current.jpg","thumburl":"https://upload.wikimedia.org/1200px-gate.jpg","thumbwidth":1200,"thumbheight":800,"mime":"image/jpeg","mediatype":"BITMAP","extmetadata":{"Artist":{"value":"Alice"},"LicenseShortName":{"value":"CC BY-SA 4.0"},"LicenseUrl":{"value":"https://creativecommons.org/licenses/by-sa/4.0/"}}}]},{"pageid":101,"ns":6,"title":"File:Gate-side.jpg","imageinfo":[{"size":500000,"width":1800,"height":1200,"sha1":"sidehash","url":"https://upload.wikimedia.org/side.jpg","descriptionurl":"https://commons.wikimedia.org/wiki/File:Gate-side.jpg","thumburl":"https://upload.wikimedia.org/1200px-side.jpg","thumbwidth":1200,"thumbheight":800,"mime":"image/jpeg","mediatype":"BITMAP","extmetadata":{"Artist":{"value":"Bob"},"LicenseShortName":{"value":"CC0"}}}]}]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"query":{"normalized":[{"from":"File:old gate.jpg","to":"File:Old gate.jpg"}],"redirects":[{"from":"File:Old gate.jpg","to":"File:Gate-current.jpg"}],"pages":[{"pageid":100,"ns":6,"title":"File:Gate-current.jpg","imageinfo":[{"timestamp":"2025-01-01T00:00:00Z","user":"Alice","size":1000000,"width":3000,"height":2000,"sha1":"gatehash","url":"https://upload.wikimedia.org/gate.jpg","descriptionurl":"https://commons.wikimedia.org/wiki/File:Gate-current.jpg","thumburl":"https://upload.wikimedia.org/1200px-gate.jpg","thumbwidth":1200,"thumbheight":800,"mime":"image/jpeg","mediatype":"BITMAP","extmetadata":{"ImageDescription":{"value":"<b>Brandenburger Tor</b>"},"Artist":{"value":"Alice"},"LicenseShortName":{"value":"CC BY-SA 4.0"},"LicenseUrl":{"value":"https://creativecommons.org/licenses/by-sa/4.0/"}}}]}]}}`))
		case strings.HasPrefix(r.URL.Path, "/wp/de"):
			_, _ = w.Write([]byte(`{"query":{"pages":[{"pageid":7,"title":"Brandenburger Tor","fullurl":"https://de.wikipedia.org/wiki/Brandenburger_Tor","description":"Tor in Berlin","extract":"Das Brandenburger Tor ist ein Wahrzeichen.","thumbnail":{"source":"https://upload.wikimedia.org/wiki/thumb.jpg","width":1200,"height":800}}]}}`))
		default:
			t.Fatalf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
	}))
}
