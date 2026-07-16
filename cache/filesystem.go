package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type diskEntry struct {
	Key       string    `json:"key"`
	ExpiresAt time.Time `json:"expires_at"`
	Value     []byte    `json:"value"`
}

// Filesystem stores entries as hashed JSON files and replaces them atomically.
type Filesystem struct {
	directory string
	now       func() time.Time
}

// NewFilesystem creates or opens a cache directory.
func NewFilesystem(directory string) (*Filesystem, error) {
	if directory == "" {
		return nil, errors.New("cache: empty directory")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("cache: resolve directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return nil, fmt.Errorf("cache: create directory: %w", err)
	}
	return &Filesystem{directory: absolute, now: time.Now}, nil
}

// Get implements mediawiki.Cache.
func (c *Filesystem) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if c == nil {
		return nil, false, nil
	}
	path := c.path(key)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("cache: read: %w", err)
	}
	var entry diskEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		_ = os.Remove(path)
		return nil, false, nil
	}
	if entry.Key != key {
		return nil, false, errors.New("cache: key hash collision")
	}
	if !entry.ExpiresAt.IsZero() && !c.now().Before(entry.ExpiresAt) {
		_ = os.Remove(path)
		return nil, false, nil
	}
	return append([]byte(nil), entry.Value...), true, nil
}

// Set implements mediawiki.Cache.
func (c *Filesystem) Set(ctx context.Context, key string, value []byte, expiresAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil {
		return nil
	}
	raw, err := json.Marshal(diskEntry{Key: key, ExpiresAt: expiresAt, Value: value})
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(c.directory, ".cache-*.tmp")
	if err != nil {
		return fmt.Errorf("cache: create temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		return fmt.Errorf("cache: write: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("cache: sync: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, c.path(key)); err != nil {
		return fmt.Errorf("cache: commit: %w", err)
	}
	committed = true
	return nil
}

// Delete implements mediawiki.Cache.
func (c *Filesystem) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil {
		return nil
	}
	err := os.Remove(c.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (c *Filesystem) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(c.directory, hex.EncodeToString(sum[:])+".json")
}
