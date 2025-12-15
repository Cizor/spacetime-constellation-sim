package controller

import (
	"testing"
	"time"
)

func TestContactWindowCache_GetUpdateInvalidate(t *testing.T) {
	cache := NewContactWindowCache(50 * time.Millisecond)
	linkID := "link-A"
	windows := []ContactWindow{{LinkID: linkID, StartTime: time.Now(), EndTime: time.Now().Add(1 * time.Second)}}
	if _, ok := cache.Get(linkID); ok {
		t.Fatalf("expected cache miss before update")
	}
	cache.UpdateWindows(linkID, windows)
	if got, ok := cache.Get(linkID); !ok {
		t.Fatalf("expected cache hit after update")
	} else if len(got) != len(windows) {
		t.Fatalf("expected %d windows, got %d", len(windows), len(got))
	}
	cache.Invalidate(linkID)
	if _, ok := cache.Get(linkID); ok {
		t.Fatalf("expected cache miss after invalidation")
	}
	if hits, misses, invalids := cache.Stats(); hits != 1 || misses != 2 || invalids != 1 {
		t.Fatalf("unexpected stats hits=%d misses=%d invalids=%d", hits, misses, invalids)
	}
}

func TestContactWindowCache_TTLExpiry(t *testing.T) {
	cache := NewContactWindowCache(10 * time.Millisecond)
	linkID := "link-B"
	windows := []ContactWindow{{LinkID: linkID, StartTime: time.Now(), EndTime: time.Now().Add(1 * time.Second)}}
	cache.UpdateWindows(linkID, windows)
	if _, ok := cache.Get(linkID); !ok {
		t.Fatalf("expected hit before TTL")
	}
	time.Sleep(25 * time.Millisecond)
	if _, ok := cache.Get(linkID); ok {
		t.Fatalf("expected miss after TTL")
	}
}
