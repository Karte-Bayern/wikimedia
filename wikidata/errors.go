package wikidata

import "errors"

var (
	// ErrInvalidID indicates an invalid non-zero Wikidata item ID.
	ErrInvalidID = errors.New("wikidata: invalid item ID")
	// ErrNotFound indicates a missing Wikidata entity.
	ErrNotFound = errors.New("wikidata: entity not found")
	// ErrInvalidProperty indicates an invalid Wikidata property ID.
	ErrInvalidProperty = errors.New("wikidata: invalid property ID")
	// ErrInvalidSearch indicates an empty or otherwise invalid item search.
	ErrInvalidSearch = errors.New("wikidata: invalid item search")
)
