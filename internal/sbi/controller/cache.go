package controller

import (
	"sync"
	"time"
)

const (
	defaultContactWindowCacheTTL = 30 * time.Second
)

type contactWindowEntry struct {
	windows []ContactWindow
	updated time.Time
}

// ContactWindowCache caches contact windows per link to avoid redundant sampling.
type ContactWindowCache struct {
	mu       sync.RWMutex
	windows  map[string]contactWindowEntry
	ttl      time.Duration
	hits     int64
	misses   int64
	invalids int64
}

// NewContactWindowCache creates a cache with the provided TTL; zero uses a default.
func NewContactWindowCache(ttl time.Duration) *ContactWindowCache {
	if ttl <= 0 {
		ttl = defaultContactWindowCacheTTL
	}
	return &ContactWindowCache{
		windows: make(map[string]contactWindowEntry),
		ttl:     ttl,
	}
}

func (c *ContactWindowCache) TTL() time.Duration {
	if c == nil {
		return 0
	}
	return c.ttl
}

func (c *ContactWindowCache) Get(linkID string) ([]ContactWindow, bool) {
	if c == nil || linkID == "" {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.windows[linkID]
	c.mu.RUnlock()
	if !ok {
		c.recordMiss()
		return nil, false
	}
	if time.Since(entry.updated) > c.ttl {
		c.recordMiss()
		return nil, false
	}
	c.recordHit()
	return cloneContactWindows(entry.windows), true
}

func (c *ContactWindowCache) UpdateWindows(linkID string, windows []ContactWindow) {
	if c == nil || linkID == "" {
		return
	}
	c.mu.Lock()
	c.windows[linkID] = contactWindowEntry{windows: cloneContactWindows(windows), updated: time.Now()}
	c.mu.Unlock()
}

func (c *ContactWindowCache) Invalidate(linkID string) {
	if c == nil || linkID == "" {
		return
	}
	c.mu.Lock()
	if _, ok := c.windows[linkID]; ok {
		c.invalids++
		delete(c.windows, linkID)
	}
	c.mu.Unlock()
}

func (c *ContactWindowCache) InvalidateAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.windows = make(map[string]contactWindowEntry)
	c.invalids++
	c.mu.Unlock()
}

func (c *ContactWindowCache) Stats() (hits, misses, invalids int64) {
	if c == nil {
		return 0, 0, 0
	}
	c.mu.RLock()
	hits, misses, invalids = c.hits, c.misses, c.invalids
	c.mu.RUnlock()
	return
}

func (c *ContactWindowCache) recordHit() {
	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
}

func (c *ContactWindowCache) recordMiss() {
	c.mu.Lock()
	c.misses++
	c.mu.Unlock()
}

func cloneContactWindows(src []ContactWindow) []ContactWindow {
	if src == nil {
		return nil
	}
	clone := make([]ContactWindow, len(src))
	copy(clone, src)
	return clone
}
