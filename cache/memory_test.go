package cache

import (
	"context"
	"testing"
	"time"
)

func TestMemoryCopiesAndExpires(t *testing.T) {
	cache := NewMemory()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }
	input := []byte("value")
	if err := cache.Set(context.Background(), "key", input, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	input[0] = 'X'
	value, found, err := cache.Get(context.Background(), "key")
	if err != nil || !found || string(value) != "value" {
		t.Fatalf("value=%q found=%v err=%v", value, found, err)
	}
	value[0] = 'Y'
	second, _, _ := cache.Get(context.Background(), "key")
	if string(second) != "value" {
		t.Fatalf("second=%q", second)
	}
	now = now.Add(2 * time.Hour)
	if _, found, err := cache.Get(context.Background(), "key"); err != nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
}

func TestMemoryZeroValueIsUsable(t *testing.T) {
	var storage Memory
	if err := storage.Set(context.Background(), "key", []byte("value"), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	value, found, err := storage.Get(context.Background(), "key")
	if err != nil || !found || string(value) != "value" {
		t.Fatalf("value=%q found=%v err=%v", value, found, err)
	}
}
