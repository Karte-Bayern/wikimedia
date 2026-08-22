package wikimedia

import (
	"net/url"
	"strings"

	"github.com/karte-bayern/wikimedia/wikidata"
)

// EntityReference identifies a related Wikidata item.
type EntityReference struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// Address contains common address claims when available. Fields are source
// data and may be incomplete or use locally appropriate formats.
type Address struct {
	StreetAddress string           `json:"street_address,omitempty"`
	HouseNumber   string           `json:"house_number,omitempty"`
	PostalCode    string           `json:"postal_code,omitempty"`
	Country       *EntityReference `json:"country,omitempty"`
}

// StringValues returns non-empty string values for a Wikidata property.
func (r *Result) StringValues(property string) []string {
	if r == nil {
		return nil
	}
	values := make([]string, 0, len(r.Claims[property]))
	seen := make(map[string]struct{})
	for _, claim := range r.Claims[property] {
		value, ok := claim.MainSnak.StringValue()
		value = strings.TrimSpace(value)
		if !ok || value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

// EntityReferences returns linked-item values for a Wikidata property.
func (r *Result) EntityReferences(property string) []EntityReference {
	if r == nil {
		return nil
	}
	values := make([]EntityReference, 0, len(r.Claims[property]))
	seen := make(map[string]struct{})
	for _, claim := range r.Claims[property] {
		value, ok := claim.MainSnak.EntityValue()
		if !ok || value.ID == "" {
			continue
		}
		if _, exists := seen[value.ID]; exists {
			continue
		}
		seen[value.ID] = struct{}{}
		values = append(values, EntityReference{ID: value.ID, URL: "https://www.wikidata.org/wiki/" + value.ID})
	}
	return values
}

// Inception returns the preferred inception date (P571), if present.
func (r *Result) Inception() (wikidata.TimeValue, bool) {
	return r.firstTimeValue("P571")
}

// OfficialWebsites returns validated official website claims (P856).
func (r *Result) OfficialWebsites() []string {
	values := r.StringValues("P856")
	websites := values[:0]
	for _, value := range values {
		parsed, err := url.Parse(value)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" {
			websites = append(websites, value)
		}
	}
	return websites
}

// AdministrativeAreas returns administrative-territory links (P131).
func (r *Result) AdministrativeAreas() []EntityReference { return r.EntityReferences("P131") }

// HeritageDesignations returns heritage designation links (P1435).
func (r *Result) HeritageDesignations() []EntityReference { return r.EntityReferences("P1435") }

// OpeningHours returns the first opening-hours statement (P8629), if present.
func (r *Result) OpeningHours() string {
	values := r.StringValues("P8629")
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// Address returns common address claims: street address (P6375), house number
// (P670), postal code (P281), and country (P17).
func (r *Result) Address() Address {
	address := Address{StreetAddress: firstStringValue(r, "P6375"), HouseNumber: firstStringValue(r, "P670"), PostalCode: firstStringValue(r, "P281")}
	if countries := r.EntityReferences("P17"); len(countries) > 0 {
		address.Country = &countries[0]
	}
	return address
}

func (r *Result) firstTimeValue(property string) (wikidata.TimeValue, bool) {
	if r == nil {
		return wikidata.TimeValue{}, false
	}
	for _, rank := range []string{"preferred", "normal", "deprecated"} {
		for _, claim := range r.Claims[property] {
			if claim.Rank != rank {
				continue
			}
			if value, ok := claim.MainSnak.TimeValue(); ok {
				return value, true
			}
		}
	}
	for _, claim := range r.Claims[property] {
		if value, ok := claim.MainSnak.TimeValue(); ok {
			return value, true
		}
	}
	return wikidata.TimeValue{}, false
}

func firstStringValue(result *Result, property string) string {
	values := result.StringValues(property)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
