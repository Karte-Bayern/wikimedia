package wikimedia

import (
	"errors"

	"github.com/karte-bayern/wikimedia/wikidata"
)

var (
	// ErrInvalidID indicates an invalid Wikidata Q item ID.
	ErrInvalidID = wikidata.ErrInvalidID
	// ErrNotFound indicates a missing Wikidata item.
	ErrNotFound = wikidata.ErrNotFound
	// ErrInvalidOSMID indicates an invalid OpenStreetMap object ID or type.
	ErrInvalidOSMID = errors.New("wikimedia: invalid OpenStreetMap ID")
)
