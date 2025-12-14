# Issue #007: Implement Preemption Logic for High-Priority ServiceRequests

**Labels:** `scope5`, `scheduling`, `preemption`, `priority`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High

## Description

When a high-priority ServiceRequest needs capacity occupied by lower-priority ServiceRequests, implement preemption logic to tear down lower-priority paths and allocate resources to the higher-priority request.

## Tasks

1. **Identify conflicting ServiceRequests**:
   - When high-priority SR needs a link with insufficient capacity
   - Find all lower-priority SRs using that link
   - Sort conflicting SRs by priority (lowest first for preemption)

2. **Preemption workflow**:
   - For each conflicting lower-priority SR:
     - Mark as "preempted"
     - Update ServiceRequest status: `IsProvisionedNow = false` (via #001 helpers)
     - Schedule `DeleteBeam`/`DeleteRoute` actions to tear down existing paths
     - Release bandwidth reservations (via #006)
   - Then schedule the new high-priority SR

3. **Preemption state tracking**:
   - Track which SRs were preempted
   - Record preemption reason and timestamp
   - Support preemption notifications (for future observability)

4. **Handle preemption edge cases**:
   - What if preempted SR has same priority? (use earliest-deadline-first or FIFO)
   - What if multiple high-priority SRs conflict? (use secondary criteria)
   - What if preemption fails? (rollback or error handling)

5. **Integration with scheduler**:
   - Preemption happens before scheduling new SR
   - Ensure atomicity: either all preemptions succeed or none
   - Update status for all affected SRs

## Acceptance Criteria

- [ ] Conflicting lower-priority SRs are correctly identified
- [ ] Preemption marks SRs as preempted and updates status
- [ ] Preemption schedules DeleteBeam/DeleteRoute actions
- [ ] Preemption releases bandwidth reservations
- [ ] High-priority SR is scheduled after preemption
- [ ] Preemption state is tracked and queryable
- [ ] Unit tests verify preemption workflow
- [ ] Unit tests verify edge cases (equal priority, multiple conflicts)
- [ ] Integration tests verify preemption in realistic scenarios

## Dependencies

- #001: Audit ServiceRequest Status Tracking (needs status update helpers)
- #003: Add Bandwidth Tracking to Links (needs bandwidth tracking)
- #005: Priority Queue for ServiceRequests (needs priority ordering)
- #006: Bandwidth-Aware Allocation (needs capacity checking)

## Related Issues

- #015: ServiceRequest Status Tracking & Updates (will update status during preemption)

## Notes

Preemption is a critical feature for priority-based scheduling. It ensures high-priority requests can always get resources, even if it means interrupting lower-priority traffic.

