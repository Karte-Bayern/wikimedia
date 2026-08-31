package wikidata

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Text is a localized Wikibase string.
type Text struct {
	Language string `json:"language"`
	Value    string `json:"value"`
}

// Entity is a Wikidata item returned by wbgetentities.
type Entity struct {
	PageID         int64               `json:"pageid,omitempty"`
	Namespace      int                 `json:"ns,omitempty"`
	Title          string              `json:"title,omitempty"`
	LastRevision   int64               `json:"lastrevid,omitempty"`
	Modified       string              `json:"modified,omitempty"`
	Type           string              `json:"type,omitempty"`
	ID             string              `json:"id"`
	RedirectedFrom string              `json:"redirected,omitempty"`
	Missing        bool                `json:"missing,omitempty"`
	Labels         map[string]Text     `json:"labels,omitempty"`
	Descriptions   map[string]Text     `json:"descriptions,omitempty"`
	Aliases        map[string][]Text   `json:"aliases,omitempty"`
	Claims         map[string][]Claim  `json:"claims,omitempty"`
	Sitelinks      map[string]Sitelink `json:"sitelinks,omitempty"`
	Raw            json.RawMessage     `json:"-"`
}

// Sitelink links an item to a Wikimedia wiki page.
type Sitelink struct {
	Site   string   `json:"site"`
	Title  string   `json:"title"`
	Badges []string `json:"badges,omitempty"`
	URL    string   `json:"url,omitempty"`
}

// Claim is one Wikibase statement.
type Claim struct {
	ID              string            `json:"id,omitempty"`
	Type            string            `json:"type,omitempty"`
	MainSnak        Snak              `json:"mainsnak"`
	Rank            string            `json:"rank,omitempty"`
	Qualifiers      map[string][]Snak `json:"qualifiers,omitempty"`
	QualifiersOrder []string          `json:"qualifiers-order,omitempty"`
	References      []Reference       `json:"references,omitempty"`
}

// Reference is one claim reference block.
type Reference struct {
	Hash       string            `json:"hash,omitempty"`
	Snaks      map[string][]Snak `json:"snaks,omitempty"`
	SnaksOrder []string          `json:"snaks-order,omitempty"`
}

// Snak contains one typed Wikibase value.
type Snak struct {
	SnakType  string    `json:"snaktype,omitempty"`
	Property  string    `json:"property,omitempty"`
	Hash      string    `json:"hash,omitempty"`
	DataValue DataValue `json:"datavalue,omitempty"`
	DataType  string    `json:"datatype,omitempty"`
}

// DataValue preserves a Wikibase data value without limiting uncommon types.
type DataValue struct {
	Value json.RawMessage `json:"value,omitempty"`
	Type  string          `json:"type,omitempty"`
}

// EntityValue identifies another Wikibase entity.
type EntityValue struct {
	EntityType string `json:"entity-type,omitempty"`
	NumericID  int64  `json:"numeric-id,omitempty"`
	ID         string `json:"id,omitempty"`
}

// CoordinateValue is a globe coordinate.
type CoordinateValue struct {
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	Altitude  *float64 `json:"altitude,omitempty"`
	Precision *float64 `json:"precision,omitempty"`
	Globe     string   `json:"globe,omitempty"`
}

// TimeValue is a Wikibase time including precision and calendar model.
type TimeValue struct {
	Time          string `json:"time"`
	Timezone      int    `json:"timezone,omitempty"`
	Before        int    `json:"before,omitempty"`
	After         int    `json:"after,omitempty"`
	Precision     int    `json:"precision,omitempty"`
	CalendarModel string `json:"calendarmodel,omitempty"`
}

// QuantityValue is a Wikibase quantity.
type QuantityValue struct {
	Amount     string  `json:"amount"`
	Unit       string  `json:"unit,omitempty"`
	UpperBound *string `json:"upperBound,omitempty"`
	LowerBound *string `json:"lowerBound,omitempty"`
}

// MonolingualTextValue is text tagged with one language.
type MonolingualTextValue struct {
	Text     string `json:"text"`
	Language string `json:"language"`
}

// ClaimsByRank returns a property's claims in Wikidata rank order: preferred,
// normal, then deprecated. The source order inside each rank is retained.
// Deprecated claims are omitted unless includeDeprecated is true.
func (e Entity) ClaimsByRank(property string, includeDeprecated bool) []Claim {
	claims := e.Claims[strings.ToUpper(strings.TrimSpace(property))]
	if len(claims) == 0 {
		return nil
	}
	values := make([]Claim, 0, len(claims))
	for _, rank := range []string{"preferred", "normal", "deprecated"} {
		if rank == "deprecated" && !includeDeprecated {
			continue
		}
		for _, claim := range claims {
			if normalizedClaimRank(claim.Rank) == rank {
				values = append(values, claim)
			}
		}
	}
	return values
}

// PreferredClaim returns the first claim at the best available rank. Set
// includeDeprecated only when deprecated values are desired as a fallback.
func (e Entity) PreferredClaim(property string, includeDeprecated bool) (Claim, bool) {
	claims := e.ClaimsByRank(property, includeDeprecated)
	if len(claims) == 0 {
		return Claim{}, false
	}
	return claims[0], true
}

// QualifierSnaks returns all qualifier snaks for property in source order.
func (c Claim) QualifierSnaks(property string) []Snak {
	return cloneSnaks(c.Qualifiers[strings.ToUpper(strings.TrimSpace(property))])
}

// ReferenceSnaks returns all reference snaks for property in source order.
// Reference blocks are processed in their source order.
func (c Claim) ReferenceSnaks(property string) []Snak {
	property = strings.ToUpper(strings.TrimSpace(property))
	var values []Snak
	for _, reference := range c.References {
		values = append(values, reference.Snaks[property]...)
	}
	return cloneSnaks(values)
}

func normalizedClaimRank(rank string) string {
	switch strings.ToLower(strings.TrimSpace(rank)) {
	case "preferred", "deprecated":
		return strings.ToLower(strings.TrimSpace(rank))
	default:
		return "normal"
	}
}

func cloneSnaks(values []Snak) []Snak {
	if len(values) == 0 {
		return nil
	}
	return append([]Snak(nil), values...)
}

// StringValue decodes string, URL, external-ID, and Commons-media values.
func (s Snak) StringValue() (string, bool) {
	if s.SnakType != "value" || len(s.DataValue.Value) == 0 {
		return "", false
	}
	var value string
	if err := json.Unmarshal(s.DataValue.Value, &value); err != nil {
		return "", false
	}
	return value, true
}

// EntityValue decodes a wikibase-item value.
func (s Snak) EntityValue() (EntityValue, bool) {
	var value EntityValue
	if s.SnakType != "value" || json.Unmarshal(s.DataValue.Value, &value) != nil {
		return EntityValue{}, false
	}
	if value.ID == "" && value.NumericID > 0 {
		value.ID = "Q" + strconv.FormatInt(value.NumericID, 10)
	}
	return value, value.ID != ""
}

// CoordinateValue decodes a globe-coordinate value.
func (s Snak) CoordinateValue() (CoordinateValue, bool) {
	var value CoordinateValue
	if s.SnakType != "value" || json.Unmarshal(s.DataValue.Value, &value) != nil {
		return CoordinateValue{}, false
	}
	return value, true
}

// TimeValue decodes a time value.
func (s Snak) TimeValue() (TimeValue, bool) {
	var value TimeValue
	if s.SnakType != "value" || json.Unmarshal(s.DataValue.Value, &value) != nil {
		return TimeValue{}, false
	}
	return value, value.Time != ""
}

// QuantityValue decodes a quantity value.
func (s Snak) QuantityValue() (QuantityValue, bool) {
	var value QuantityValue
	if s.SnakType != "value" || json.Unmarshal(s.DataValue.Value, &value) != nil {
		return QuantityValue{}, false
	}
	return value, value.Amount != ""
}

// MonolingualTextValue decodes a monolingual-text value.
func (s Snak) MonolingualTextValue() (MonolingualTextValue, bool) {
	var value MonolingualTextValue
	if s.SnakType != "value" || json.Unmarshal(s.DataValue.Value, &value) != nil {
		return MonolingualTextValue{}, false
	}
	return value, value.Text != ""
}

// ValueString returns a stable human-readable representation for common values.
func (s Snak) ValueString() string {
	if value, ok := s.StringValue(); ok {
		return value
	}
	if value, ok := s.EntityValue(); ok {
		return value.ID
	}
	if value, ok := s.CoordinateValue(); ok {
		return fmt.Sprintf("%g,%g", value.Latitude, value.Longitude)
	}
	if value, ok := s.TimeValue(); ok {
		return value.Time
	}
	if value, ok := s.QuantityValue(); ok {
		return strings.TrimPrefix(value.Amount, "+") + " " + value.Unit
	}
	if value, ok := s.MonolingualTextValue(); ok {
		return value.Text
	}
	return ""
}
