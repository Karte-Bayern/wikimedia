package download

import "errors"

// Stable downloader error categories.
var (
	ErrInvalidURL        = errors.New("download: invalid URL")
	ErrHostNotAllowed    = errors.New("download: host not allowed")
	ErrTooLarge          = errors.New("download: file too large")
	ErrAlreadyExists     = errors.New("download: destination already exists")
	ErrUnsafeDestination = errors.New("download: unsafe destination")
	ErrSizeMismatch      = errors.New("download: size mismatch")
	ErrChecksumMismatch  = errors.New("download: checksum mismatch")
	ErrMIMEMismatch      = errors.New("download: MIME type mismatch")
	ErrContentEncoding   = errors.New("download: unsupported content encoding")
)
