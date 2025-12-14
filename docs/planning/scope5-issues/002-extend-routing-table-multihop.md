# Issue #002: Extend Routing Table Model for Multi-Hop Paths

**Labels:** `scope5`, `prep`, `routing`, `multihop`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High (Foundation)

## Description

Scope 4 introduced basic `RouteEntry` for single-hop routing. For Scope 5 multi-hop path computation, we need to extend the routing table model to support full path information, path costs, and expiration times.

## Tasks

1. **Extend RouteEntry struct** in `model/networknode.go` or appropriate location:
   ```go
   type RouteEntry struct {
       DestinationCIDR string
       NextHopNodeID   string
       OutInterfaceID  string
       // New for Scope 5:
       Path []string // full path: [srcNode, hop1, hop2, ..., dstNode]
       Cost int      // path cost metric (hops, latency, etc.)
       ValidUntil time.Time // when this route expires
   }
   ```

2. **Add multi-hop route methods to ScenarioState** in `internal/sim/state/state.go`:
   - `InstallMultiHopRoute(nodeID string, route RouteEntry) error`
   - `GetRoutePath(srcNodeID, dstNodeID string) ([]string, error)`
   - `InvalidateExpiredRoutes(now time.Time) error`

3. **Update existing InstallRoute** to support both single-hop and multi-hop:
   - If `route.Path` is provided, store full path
   - If `route.Path` is empty, treat as single-hop (backward compatible)

4. **Add route expiration logic**:
   - Periodic cleanup of expired routes
   - Integration with scheduler's time controller

## Acceptance Criteria

- [ ] RouteEntry struct includes Path, Cost, and ValidUntil fields
- [ ] InstallMultiHopRoute correctly stores full path information
- [ ] GetRoutePath returns complete path from source to destination
- [ ] InvalidateExpiredRoutes removes expired routes
- [ ] Backward compatibility maintained for single-hop routes
- [ ] Unit tests verify multi-hop route installation and retrieval
- [ ] Route expiration is tested and working

## Dependencies

- None (foundation issue)

## Related Issues

- #012: Time-Aware Multi-Hop Path Computation (will use these route structures)
- #013: Path validation and scheduling (will install multi-hop routes)

## Notes

This extends the existing routing infrastructure to support multi-hop paths. The full path information is needed for path validation, conflict detection, and re-planning.

