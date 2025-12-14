# Issue #008: Implement Contact Window Data Structure

**Labels:** `scope5`, `multihop`, `connectivity`, `contact-windows`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High

## Description

Extend connectivity evaluation to maintain contact windows - time intervals when links are geometrically feasible. This is the foundation for time-aware multi-hop path computation.

## Tasks

1. **Define ContactWindow type** in `core/connectivity_service.go` or new file:
   ```go
   type ContactWindow struct {
       LinkID      string
       StartTime   time.Time
       EndTime     time.Time
       Quality     float64 // SNR or link quality metric
   }
   ```

2. **Define ContactPlan type**:
   ```go
   type ContactPlan struct {
       LinkID string
       Windows []ContactWindow
   }
   ```

3. **Extend connectivity service** to compute contact windows:
   - For each link, compute when it's "in view" over a time horizon
   - Store windows with start/end times and quality metrics
   - Update windows as simulation time advances

4. **Add contact plan queries to ScenarioState**:
   ```go
   GetContactPlan(linkID string, horizon time.Duration) []ContactWindow
   GetContactPlansForNode(nodeID string, horizon time.Duration) map[string][]ContactWindow
   ```

5. **Integration with existing connectivity evaluation**:
   - Contact windows should align with existing link up/down logic
   - Use same geometry/horizon checks
   - Cache windows to avoid recomputation

## Acceptance Criteria

- [ ] ContactWindow type is defined with all required fields
- [ ] ContactPlan type groups windows by link
- [ ] Connectivity service computes contact windows correctly
- [ ] GetContactPlan returns windows for a link over time horizon
- [ ] GetContactPlansForNode returns all links for a node
- [ ] Windows align with existing link connectivity logic
- [ ] Unit tests verify window computation
- [ ] Unit tests verify window queries
- [ ] Integration tests verify windows match actual link states

## Dependencies

- None (can build on existing connectivity evaluation)

## Related Issues

- #009: Time-Expanded Graph Construction (will use contact windows)
- #010: Multi-Hop Pathfinding Algorithm (will use contact windows)

## Notes

Contact windows are the time-varying connectivity information needed for multi-hop pathfinding. This is a foundational data structure for time-aware routing.

