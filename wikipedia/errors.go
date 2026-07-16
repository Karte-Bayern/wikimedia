package wikipedia

import "errors"

var (
	// ErrInvalidLanguage indicates a language code unsafe for a Wikipedia hostname.
	ErrInvalidLanguage = errors.New("wikipedia: invalid language")
	// ErrNotFound indicates a missing article.
	ErrNotFound = errors.New("wikipedia: article not found")
)
