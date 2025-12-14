# Issue #003: Add Capacity/Bandwidth Tracking to Links

**Labels:** `scope5`, `prep`, `bandwidth`, `capacity`, `conflict-resolution`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High (Foundation)

## Description

For conflict resolution and priority-based scheduling, we need to track link capacity and bandwidth reservations. This enables the scheduler to check if links have sufficient capacity before scheduling ServiceRequests and to detect capacity conflicts.

## Tasks

1. **Extend NetworkLink model** in `core/network_link.go`:
   ```go
   type NetworkLink struct {
       // existing fields...
       MaxBandwidthBps       uint64 // link's maximum capacity
       AvailableBandwidthBps uint64 // current available capacity
       ReservedBandwidthBps  uint64 // reserved by active flows
   }
   ```

2. **Add bandwidth tracking to ScenarioState** in `internal/sim/state/state.go`:
   - `ReserveBandwidth(linkID string, bps uint64) error`
   - `ReleaseBandwidth(linkID string, bps uint64) error`
   - `GetAvailableBandwidth(linkID string) uint64`

3. **Initialize bandwidth from link configuration**:
   - When links are created, set MaxBandwidthBps from configuration
   - Initialize AvailableBandwidthBps = MaxBandwidthBps
   - ReservedBandwidthBps starts at 0

4. **Thread-safe bandwidth operations**:
   - Use proper locking to prevent race conditions
   - Ensure atomic updates to bandwidth counters

5. **Add bandwidth validation**:
   - Prevent reserving more than available
   - Prevent releasing more than reserved
   - Handle edge cases (negative values, overflow)

## Acceptance Criteria

- [ ] NetworkLink includes MaxBandwidthBps, AvailableBandwidthBps, ReservedBandwidthBps
- [ ] ReserveBandwidth correctly decreases available bandwidth
- [ ] ReleaseBandwidth correctly increases available bandwidth
- [ ] GetAvailableBandwidth returns current available capacity
- [ ] All operations are thread-safe
- [ ] Validation prevents invalid operations (over-reservation, negative releases)
- [ ] Unit tests verify bandwidth reservation and release
- [ ] Integration tests verify bandwidth tracking across multiple ServiceRequests

## Dependencies

- None (foundation issue)

## Related Issues

- #004: Priority-Based ServiceRequest Scheduling (will use bandwidth checking)
- #005: Bandwidth-aware allocation (depends on this)
- #007: Conflict Detection & Resolution (will check bandwidth conflicts)

## Notes

This is a foundational capability needed for priority-based scheduling and conflict resolution. The bandwidth tracking must be accurate and thread-safe as multiple ServiceRequests may compete for the same link capacity.

