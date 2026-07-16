// Package cache provides optional MediaWiki response cache implementations.
package cache

import (
	"context"
	"sync"
	"time"
)

type memoryEntry struct {
	value     []byte
	expiresAt time.Time
}

// Memory is a concurrency-safe in-memory cache.
type Memory struct {
	mu      sync.RWMutex
	entries map[string]memoryEntry
	now     func() time.Time
}

// NewMemory creates an empty memory cache.
func NewMemory() *Memory { return &Memory{entries: make(map[string]memoryEntry), now: time.Now} }

// Get implements mediawiki.Cache.
func (c *Memory) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if c == nil {
		return nil, false, nil
	}
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	if !entry.expiresAt.IsZero() && !c.currentTime().Before(entry.expiresAt) {
		_ = c.Delete(ctx, key)
		return nil, false, nil
	}
	return append([]byte(nil), entry.value...), true, nil
}

// Set implements mediawiki.Cache.
func (c *Memory) Set(ctx context.Context, key string, value []byte, expiresAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]memoryEntry)
	}
	c.entries[key] = memoryEntry{value: append([]byte(nil), value...), expiresAt: expiresAt}
	return nil
}

// Delete implements mediawiki.Cache.
func (c *Memory) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil {
		return nil
	}
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
	return nil
}

func (c *Memory) currentTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// Len returns the number of entries, including entries not yet lazily expired.
func (c *Memory) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
