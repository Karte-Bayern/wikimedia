package wikimedia

import "github.com/karte-bayern/wikimedia/wikidata"

var (
	// ErrInvalidID indicates an invalid Wikidata Q item ID.
	ErrInvalidID = wikidata.ErrInvalidID
	// ErrNotFound indicates a missing Wikidata item.
	ErrNotFound = wikidata.ErrNotFound
)
