package wikimedia

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/karte-bayern/wikimedia/wikidata"
)

// GeoItem is one Wikidata item returned by a geographic query.
type GeoItem struct {
	ID          string                    `json:"id"`
	URL         string                    `json:"url"`
	Label       string                    `json:"label,omitempty"`
	Description string                    `json:"description,omitempty"`
	Coordinate  *wikidata.CoordinateValue `json:"coordinate,omitempty"`
	DistanceKM  *float64                  `json:"distance_km,omitempty"`
}

// FindNearby returns up to limit Wikidata items within radiusKM of a WGS84
// point. It uses the WDQS geographic around service and is therefore intended
// for selective, interactive queries rather than bulk extraction.
func (c *Client) FindNearby(ctx context.Context, latitude, longitude, radiusKM float64, limit int) ([]GeoItem, error) {
	if err := validatePoint(latitude, longitude); err != nil {
		return nil, err
	}
	if math.IsNaN(radiusKM) || math.IsInf(radiusKM, 0) || radiusKM <= 0 || radiusKM > 1000 {
		return nil, fmt.Errorf("%w: radius must be between 0 and 1000 km", ErrInvalidGeoQuery)
	}
	limit = normalizeGeoLimit(limit)
	query := sparqlPrefixes + fmt.Sprintf(`
SELECT ?item ?itemLabel ?itemDescription ?location ?distance WHERE {
  SERVICE wikibase:around {
    ?item wdt:P625 ?location .
    bd:serviceParam wikibase:center %q^^geo:wktLiteral .
    bd:serviceParam wikibase:radius %q .
    bd:serviceParam wikibase:distance ?distance .
  }
  SERVICE wikibase:label { bd:serviceParam wikibase:language %q . }
}
ORDER BY ASC(?distance)
LIMIT %d`, wktPoint(latitude, longitude), decimal(radiusKM), sparqlLanguageList(c.languages), limit)
	result, err := c.SPARQL().Query(ctx, query)
	if err != nil {
		return nil, err
	}
	return geoItemsFromBindings(result.Bindings), nil
}

// FindInBoundingBox returns up to limit Wikidata items inside the WGS84 box
// specified by its southern, western, northern, and eastern edges.
func (c *Client) FindInBoundingBox(ctx context.Context, south, west, north, east float64, limit int) ([]GeoItem, error) {
	if err := validatePoint(south, west); err != nil {
		return nil, err
	}
	if err := validatePoint(north, east); err != nil {
		return nil, err
	}
	if south >= north || west >= east {
		return nil, fmt.Errorf("%w: bounds must have south < north and west < east", ErrInvalidGeoQuery)
	}
	limit = normalizeGeoLimit(limit)
	query := sparqlPrefixes + fmt.Sprintf(`
SELECT ?item ?itemLabel ?itemDescription ?location WHERE {
  SERVICE wikibase:box {
    ?item wdt:P625 ?location .
    bd:serviceParam wikibase:cornerWest %q^^geo:wktLiteral .
    bd:serviceParam wikibase:cornerEast %q^^geo:wktLiteral .
  }
  SERVICE wikibase:label { bd:serviceParam wikibase:language %q . }
}
LIMIT %d`, wktPoint(south, west), wktPoint(north, east), sparqlLanguageList(c.languages), limit)
	result, err := c.SPARQL().Query(ctx, query)
	if err != nil {
		return nil, err
	}
	return geoItemsFromBindings(result.Bindings), nil
}

const sparqlPrefixes = `PREFIX bd: <http://www.bigdata.com/rdf#>
PREFIX geo: <http://www.opengis.net/ont/geosparql#>
PREFIX wikibase: <http://wikiba.se/ontology#>
PREFIX wdt: <http://www.wikidata.org/prop/direct/>
`

func validatePoint(latitude, longitude float64) error {
	if math.IsNaN(latitude) || math.IsNaN(longitude) || math.IsInf(latitude, 0) || math.IsInf(longitude, 0) || latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return fmt.Errorf("%w: latitude/longitude outside WGS84 bounds", ErrInvalidGeoQuery)
	}
	return nil
}

func normalizeGeoLimit(value int) int {
	if value <= 0 {
		return 50
	}
	if value > 500 {
		return 500
	}
	return value
}

func decimal(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }

func wktPoint(latitude, longitude float64) string {
	return "Point(" + decimal(longitude) + " " + decimal(latitude) + ")"
}

func sparqlLanguageList(values []string) string {
	valid := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if wikipediaLanguageIDPattern.MatchString(value) {
			valid = append(valid, value)
		}
	}
	if len(valid) == 0 {
		return "en"
	}
	return strings.Join(valid, ",")
}

func geoItemsFromBindings(bindings []map[string]wikidata.SPARQLBinding) []GeoItem {
	items := make([]GeoItem, 0, len(bindings))
	for _, row := range bindings {
		itemValue := row["item"].Value
		id := strings.TrimPrefix(strings.TrimPrefix(itemValue, "http://www.wikidata.org/entity/"), "https://www.wikidata.org/entity/")
		if !wikidata.ValidItemID(id) {
			continue
		}
		item := GeoItem{ID: id, URL: "https://www.wikidata.org/wiki/" + id, Label: row["itemLabel"].Value, Description: row["itemDescription"].Value}
		if coordinate, ok := coordinateFromWKT(row["location"].Value); ok {
			item.Coordinate = &coordinate
		}
		if distance, err := strconv.ParseFloat(row["distance"].Value, 64); err == nil && distance >= 0 {
			item.DistanceKM = &distance
		}
		items = append(items, item)
	}
	return items
}

func coordinateFromWKT(value string) (wikidata.CoordinateValue, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "Point(") || !strings.HasSuffix(value, ")") {
		return wikidata.CoordinateValue{}, false
	}
	parts := strings.Fields(strings.TrimSuffix(strings.TrimPrefix(value, "Point("), ")"))
	if len(parts) != 2 {
		return wikidata.CoordinateValue{}, false
	}
	longitude, longitudeErr := strconv.ParseFloat(parts[0], 64)
	latitude, latitudeErr := strconv.ParseFloat(parts[1], 64)
	if longitudeErr != nil || latitudeErr != nil || validatePoint(latitude, longitude) != nil {
		return wikidata.CoordinateValue{}, false
	}
	return wikidata.CoordinateValue{Latitude: latitude, Longitude: longitude, Globe: "http://www.wikidata.org/entity/Q2"}, true
}
