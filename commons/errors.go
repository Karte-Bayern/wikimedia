package commons

import "errors"

var (
	// ErrNotFound indicates a missing Commons file or category.
	ErrNotFound = errors.New("commons: not found")
	// ErrInvalidTitle indicates an empty or malformed Commons title.
	ErrInvalidTitle = errors.New("commons: invalid title")
)
