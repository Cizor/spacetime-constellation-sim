# Issue #024: Implement Active ServiceRequest Status Updates

**Labels:** `scope5`, `servicerequest`, `status-tracking`, `scheduler`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High

## Description

Actively maintain ServiceRequest status fields based on actual path availability and scheduling decisions. Update IsProvisionedNow and ProvisionedIntervals when paths are found, lost, or updated.

## Tasks

1. **Status update triggers** in scheduler:
   - Path is found and scheduled → IsProvisionedNow = true, add interval
   - Path is torn down → IsProvisionedNow = false, close current interval
   - Path is preempted → IsProvisionedNow = false
   - Path recomputed → update intervals

2. **Implement status update function**:
   ```go
   func (s *Scheduler) UpdateSRStatus(srID string, isProvisioned bool, interval *TimeInterval) error
   ```
   - Use helpers from #001
   - Update IsProvisionedNow
   - Manage ProvisionedIntervals list

3. **ProvisionedIntervals tracking**:
   - Maintain chronological list of intervals
   - When new interval starts: add to list
   - When interval ends: mark end time, keep in history
   - Handle gaps between intervals

4. **Integration points**:
   - Call UpdateSRStatus when path scheduled (#011)
   - Call UpdateSRStatus when path deleted
   - Call UpdateSRStatus when path updated (#018)
   - Call UpdateSRStatus when preempted (#007)

5. **Status consistency**:
   - Ensure status matches actual path state
   - Handle race conditions
   - Reconcile status on scheduler restart

## Acceptance Criteria

- [ ] UpdateSRStatus updates status correctly
- [ ] Status updated when path scheduled
- [ ] Status updated when path torn down
- [ ] Status updated when path preempted
- [ ] Status updated when path recomputed
- [ ] ProvisionedIntervals are tracked correctly
- [ ] Interval gaps are recorded
- [ ] Unit tests verify status updates for all scenarios
- [ ] Integration tests verify status consistency

## Dependencies

- #001: Audit ServiceRequest Status Tracking (needs status helpers)
- #011: Path Validation and Scheduling (triggers status updates)

## Related Issues

- #007: Preemption Logic (updates status on preemption)
- #018: Incremental Path Updates (updates status on path change)

## Notes

Active status tracking ensures ServiceRequest status accurately reflects current provisioning state. This is critical for observability and NBI status queries.

