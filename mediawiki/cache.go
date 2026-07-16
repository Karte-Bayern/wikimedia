package mediawiki

import (
	"context"
	"time"
)

// Cache stores successful raw Action API responses. Implementations must be
// safe for concurrent use.
type Cache interface {
	Get(ctx context.Context, key string) (value []byte, found bool, err error)
	Set(ctx context.Context, key string, value []byte, expiresAt time.Time) error
	Delete(ctx context.Context, key string) error
}
