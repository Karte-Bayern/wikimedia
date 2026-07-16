package wikimedia

import (
	"encoding/json"
	"testing"

	"github.com/karte-bayern/wikimedia/wikidata"
)

func TestNormalizeEntityFiltersDeprecatedReferencesAndLinks(t *testing.T) {
	entity := &wikidata.Entity{
		ID: "Q1",
		Claims: map[string][]wikidata.Claim{
			"P373": {
				stringClaim("P373", "normal", "Current category"),
				stringClaim("P373", "deprecated", "Old category"),
			},
			"P935": {
				stringClaim("P935", "deprecated", "Old gallery"),
			},
			"P856": {
				stringClaim("P856", "normal", "https://example.test/current"),
				stringClaim("P856", "deprecated", "https://example.test/old"),
			},
		},
	}

	result := normalizeEntity(entity, []string{"en"}, fetchConfig{})
	if len(result.Commons) != 1 || result.Commons[0].Title != "Category:Current category" {
		t.Fatalf("commons=%+v", result.Commons)
	}
	if len(result.Links) != 2 || result.Links[1].URL != "https://example.test/current" {
		t.Fatalf("links=%+v", result.Links)
	}

	withDeprecated := normalizeEntity(entity, []string{"en"}, fetchConfig{deprecated: true})
	if len(withDeprecated.Commons) != 3 {
		t.Fatalf("commons with deprecated=%+v", withDeprecated.Commons)
	}
	if len(withDeprecated.Links) != 3 {
		t.Fatalf("links with deprecated=%+v", withDeprecated.Links)
	}
}

func stringClaim(property, rank, value string) wikidata.Claim {
	raw, _ := json.Marshal(value)
	return wikidata.Claim{
		Rank: rank,
		MainSnak: wikidata.Snak{
			SnakType: "value",
			Property: property,
			DataValue: wikidata.DataValue{
				Type:  "string",
				Value: raw,
			},
		},
	}
}
