# Issue #038: Implement Contact Window Caching for Performance

**Labels:** `scope5`, `performance`, `caching`, `optimization`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** Medium

## Description

Pre-compute and cache contact windows to avoid repeated computation. This improves scheduler performance for large-scale scenarios.

## Tasks

1. **Define ContactWindowCache type** in `internal/sbi/controller/cache.go`:
   ```go
   type ContactWindowCache struct {
       mu sync.RWMutex
       windows map[string][]ContactWindow // linkID -> windows
       lastUpdate map[string]time.Time
       ttl time.Duration
   }
   ```

2. **Implement cache methods**:
   - `GetWindows(linkID string, horizon time.Duration) []ContactWindow`
   - `Invalidate(linkID string)`
   - `InvalidateAll()`
   - `UpdateWindows(linkID string, windows []ContactWindow)`

3. **Cache invalidation logic**:
   - Invalidate when platform motion changes significantly
   - Invalidate when link parameters change
   - Periodic refresh (every N simulation seconds)
   - TTL-based expiration

4. **Integration with connectivity service**:
   - Use cache in contact window queries
   - Populate cache on first query
   - Update cache on invalidation

5. **Performance monitoring**:
   - Track cache hit/miss rates
   - Monitor cache size
   - Log cache performance metrics

## Acceptance Criteria

- [ ] ContactWindowCache is implemented correctly
- [ ] GetWindows returns cached windows when available
- [ ] Cache invalidation works correctly
- [ ] Cache improves query performance
- [ ] Cache hit/miss rates are tracked
- [ ] Unit tests verify cache operations
- [ ] Unit tests verify invalidation
- [ ] Performance tests verify cache benefits

## Dependencies

- #008: Contact Window Data Structure (needs ContactWindow type)

## Related Issues

- #039: Incremental Path Updates (may use cache)
- #040: Parallel Path Computation (may use cache)

## Notes

Contact window caching is critical for performance at scale. Windows are expensive to compute but change infrequently, making them ideal for caching.

## Implementation Plan

- **Cache API & placement**: implement `ContactWindowCache` in `internal/sbi/controller/cache.go`. The cache exposes `GetWindows(linkID, horizon) ([]ContactWindow, bool)` which returns cached windows and a hit flag, `UpdateWindows(linkID, windows)` to refresh entries, and invalidation helpers (per-link + global + TTL-based eviction).
- **TTL & recompute policy**: track `lastUpdate[linkID]` timestamps, expire entries when `time.Since(lastUpdate)` exceeds `ttl` (configurable via scheduler options) or when upstream metadata (link/contact horizon) changes. Use synthetic `contactWindowVersion` indexes to skip stale writes.
- **Invalidation triggers**: hook into scheduler events that modify link state (new contact, horizon change, platform/node motion). Expose `Invalidate(linkID)` on the cache so the scheduler and telemetry service can mark entries dirty when they detect relevant state changes.
- **Integration with scheduler/pathfinding**: the scheduler’s `ensureContactWindows` (or equivalent) consults the cache before recomputing windows. On cache miss/stale entry, rebuild windows and call `UpdateWindows`. Continue to store windows on the scheduler state so subsequent queries within the TTL reuse the cache.
- **Metrics & visibility**: add counter gauges for hits, misses, updates, invalidations. Log TTL/size stats periodically (e.g., every N scheduler iterations) so ops can adjust TTL for their scenarios.
- **Unit tests**: cover `GetWindows` returning hits vs misses, `UpdateWindows` replacing entries, invalidation hooks, and TTL expiry. Simulate scheduler pathfinding calling the cache and verify it avoids recomputation when cache is valid.
