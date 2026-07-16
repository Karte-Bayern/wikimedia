package cache

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestFilesystemRoundTripAndExpiry(t *testing.T) {
	cache, err := NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }
	if err := cache.Set(context.Background(), "https://example.test/?a=1", []byte("payload"), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	value, found, err := cache.Get(context.Background(), "https://example.test/?a=1")
	if err != nil || !found || string(value) != "payload" {
		t.Fatalf("value=%q found=%v err=%v", value, found, err)
	}
	entries, err := os.ReadDir(cache.directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%d err=%v", len(entries), err)
	}
	now = now.Add(2 * time.Hour)
	if _, found, err := cache.Get(context.Background(), "https://example.test/?a=1"); err != nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
}
