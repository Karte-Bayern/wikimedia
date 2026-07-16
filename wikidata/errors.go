package wikidata

import "errors"

var (
	// ErrInvalidID indicates an invalid non-zero Wikidata item ID.
	ErrInvalidID = errors.New("wikidata: invalid item ID")
	// ErrNotFound indicates a missing Wikidata entity.
	ErrNotFound = errors.New("wikidata: entity not found")
)
