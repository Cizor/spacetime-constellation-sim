# Issue #006: Implement Bandwidth-Aware Path Allocation

**Labels:** `scope5`, `scheduling`, `bandwidth`, `allocation`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High

## Description

When scheduling a ServiceRequest, check each link in the path for available bandwidth. If any link lacks sufficient capacity, either preempt lower-priority flows (if SR has higher priority) or mark SR as "insufficient capacity" and skip.

## Tasks

1. **Implement path capacity checking** in `internal/sbi/controller/scheduler.go`:
   ```go
   func (s *Scheduler) CheckPathCapacity(path []string, requiredBps uint64) (bool, []string)
   ```
   - Returns: (hasCapacity, conflictingLinkIDs)
   - Check each link in path for available bandwidth
   - Return list of links with insufficient capacity

2. **Implement path capacity allocation**:
   ```go
   func (s *Scheduler) AllocatePathCapacity(path []string, srID string, bps uint64) error
   ```
   - Reserve bandwidth on each link in path
   - Associate reservation with ServiceRequest ID
   - Track reservations for later release

3. **Implement path capacity release**:
   ```go
   func (s *Scheduler) ReleasePathCapacity(path []string, srID string, bps uint64) error
   ```
   - Release bandwidth reservations on all links in path
   - Handle partial releases if path is partially torn down

4. **Integration with path computation**:
   - Before scheduling a path, check capacity
   - Only schedule if sufficient capacity available
   - Track which ServiceRequests are using which links

5. **Handle bandwidth requirements from ServiceRequest**:
   - Extract required bandwidth from FlowRequirements
   - Use maximum required bandwidth across all flows
   - Support per-flow bandwidth if needed

## Acceptance Criteria

- [ ] CheckPathCapacity correctly identifies links with insufficient capacity
- [ ] AllocatePathCapacity reserves bandwidth on all path links
- [ ] ReleasePathCapacity releases bandwidth correctly
- [ ] Bandwidth reservations are tracked per ServiceRequest
- [ ] Integration with path scheduling checks capacity before scheduling
- [ ] Unit tests verify capacity checking for various scenarios
- [ ] Unit tests verify allocation and release
- [ ] Integration tests verify bandwidth tracking across multiple SRs

## Dependencies

- #003: Add Capacity/Bandwidth Tracking to Links (must have bandwidth tracking first)

## Related Issues

- #005: Priority Queue for ServiceRequests (will use priority to decide preemption)
- #007: Preemption Logic (will use capacity checking to identify conflicts)

## Notes

This enables the scheduler to respect link capacity constraints. Combined with priority, it enables intelligent resource allocation and preemption.

