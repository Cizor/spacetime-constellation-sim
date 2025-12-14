# Issue #018: Implement Incremental Path Updates

**Labels:** `scope5`, `replanning`, `path-updates`, `optimization`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High

## Description

When re-planning, update paths incrementally when possible. If new path shares hops with old path, keep shared segments and only update changed parts. This reduces scheduling overhead.

## Tasks

1. **Define PathDiff type** in `internal/sbi/controller/scheduler.go`:
   ```go
   type PathDiff struct {
       SharedHops    []PathHop
       RemovedHops   []PathHop
       AddedHops     []PathHop
   }
   ```

2. **Implement path diff computation**:
   ```go
   func (s *Scheduler) ComputePathDiff(oldPath, newPath *Path) PathDiff
   ```
   - Compare old and new paths
   - Identify shared hops
   - Identify removed hops
   - Identify added hops

3. **Implement incremental update**:
   ```go
   func (s *Scheduler) UpdatePath(srID string, newPath *Path) error
   ```

4. **Update logic**:
   - If paths share hops: Keep shared hops (don't reschedule)
   - Only schedule new hops
   - Only cancel removed hops
   - If completely new path: Tear down old path, schedule new path

5. **Route table updates**:
   - Update routes only for changed segments
   - Keep routes for shared segments
   - Use multi-hop route helpers (from #002)

6. **Status updates**:
   - Update ServiceRequest status (from #001)
   - Close old interval, start new interval if path changed significantly
   - Keep same interval if only minor changes

## Acceptance Criteria

- [ ] PathDiff type correctly represents path differences
- [ ] ComputePathDiff identifies shared, removed, and added hops
- [ ] UpdatePath updates paths incrementally when possible
- [ ] Shared hops are not rescheduled
- [ ] Only changed segments are updated
- [ ] Route tables are updated correctly
- [ ] Status is updated appropriately
- [ ] Unit tests verify path diff computation
- [ ] Unit tests verify incremental updates
- [ ] Integration tests verify update efficiency

## Dependencies

- #001: Audit ServiceRequest Status Tracking (needs status updates)
- #002: Extend Routing Table for Multi-Hop (needs route updates)
- #010: Multi-Hop Pathfinding Algorithm (needs Path type)
- #016: Path Monitoring (needs active paths)

## Related Issues

- #017: Trigger Conditions for Re-Planning (triggers updates)
- #019: Re-Planning Loop (calls UpdatePath)

## Notes

Incremental updates improve efficiency by avoiding unnecessary rescheduling. This is important for performance, especially with frequent re-planning.

