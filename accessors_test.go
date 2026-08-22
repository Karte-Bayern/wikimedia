package wikimedia

import (
	"encoding/json"
	"testing"

	"github.com/karte-bayern/wikimedia/wikidata"
)

func TestResultTypedAccessors(t *testing.T) {
	stringClaim := func(property, value string) wikidata.Claim {
		raw, _ := json.Marshal(value)
		return wikidata.Claim{Rank: "normal", MainSnak: wikidata.Snak{SnakType: "value", Property: property, DataValue: wikidata.DataValue{Value: raw}}}
	}
	entityClaim := func(property, id string) wikidata.Claim {
		raw, _ := json.Marshal(wikidata.EntityValue{ID: id})
		return wikidata.Claim{Rank: "normal", MainSnak: wikidata.Snak{SnakType: "value", Property: property, DataValue: wikidata.DataValue{Value: raw}}}
	}
	timeRaw := json.RawMessage(`{"time":"+1791-01-01T00:00:00Z","precision":9}`)
	result := &Result{Claims: map[string][]wikidata.Claim{
		"P856":  {stringClaim("P856", "https://example.org")},
		"P8629": {stringClaim("P8629", "Mo-Fr 09:00-17:00")},
		"P6375": {stringClaim("P6375", "Museumstraße")},
		"P670":  {stringClaim("P670", "1")},
		"P281":  {stringClaim("P281", "10115")},
		"P17":   {entityClaim("P17", "Q183")},
		"P131":  {entityClaim("P131", "Q64")},
		"P1435": {entityClaim("P1435", "Q916333")},
		"P571":  {{Rank: "preferred", MainSnak: wikidata.Snak{SnakType: "value", Property: "P571", DataValue: wikidata.DataValue{Value: timeRaw}}}},
	}}
	if inception, ok := result.Inception(); !ok || inception.Time != "+1791-01-01T00:00:00Z" {
		t.Fatalf("inception=%+v ok=%v", inception, ok)
	}
	if got := result.OpeningHours(); got != "Mo-Fr 09:00-17:00" {
		t.Fatalf("opening hours=%q", got)
	}
	if areas := result.AdministrativeAreas(); len(areas) != 1 || areas[0].ID != "Q64" {
		t.Fatalf("areas=%+v", areas)
	}
	if designations := result.HeritageDesignations(); len(designations) != 1 || designations[0].ID != "Q916333" {
		t.Fatalf("designations=%+v", designations)
	}
	address := result.Address()
	if address.StreetAddress != "Museumstraße" || address.HouseNumber != "1" || address.PostalCode != "10115" || address.Country == nil || address.Country.ID != "Q183" {
		t.Fatalf("address=%+v", address)
	}
}
