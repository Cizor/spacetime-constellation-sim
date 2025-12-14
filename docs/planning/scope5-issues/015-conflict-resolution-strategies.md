# Issue #015: Implement Conflict Resolution Strategies

**Labels:** `scope5`, `conflict-resolution`, `scheduling`, `strategy`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High

## Description

When conflicts are detected, apply resolution strategies to resolve them. Support priority-based, earliest-deadline-first, and fairness strategies.

## Tasks

1. **Define resolution strategies** in `internal/sbi/controller/conflicts.go`:
   ```go
   type ResolutionStrategy string
   const (
       StrategyPriority ResolutionStrategy = "priority"
       StrategyEarliestDeadline ResolutionStrategy = "earliest_deadline"
       StrategyFairness ResolutionStrategy = "fairness"
   )
   ```

2. **Implement resolution function**:
   ```go
   func ResolveConflicts(conflicts []BeamConflict, strategy ResolutionStrategy) []BeamAction
   ```

3. **Priority-based resolution**:
   - Higher-priority ServiceRequest's beams take precedence
   - Cancel conflicting lower-priority beams
   - Use priority queue to order conflicts

4. **Earliest-deadline-first resolution**:
   - Beams with earlier deadlines are kept
   - Cancel beams with later deadlines
   - Useful for time-sensitive traffic

5. **Fairness resolution**:
   - Round-robin or proportional allocation
   - Distribute resources fairly among competing SRs
   - May adjust beam parameters instead of canceling

6. **Resolution actions**:
   - Cancel conflicting lower-priority beams
   - Adjust beam parameters (power, frequency) if possible
   - Delay beams if conflicts are temporary

7. **Integration with scheduler**:
   - Apply resolution before scheduling new beams
   - Update affected ServiceRequest statuses
   - Release resources from canceled beams

## Acceptance Criteria

- [ ] Resolution strategies are defined and implemented
- [ ] Priority-based resolution works correctly
- [ ] Earliest-deadline-first resolution works correctly
- [ ] Fairness resolution works correctly
- [ ] Resolution generates appropriate actions (cancel, adjust, delay)
- [ ] Unit tests verify each resolution strategy
- [ ] Unit tests verify action generation
- [ ] Integration tests verify conflict resolution in realistic scenarios

## Dependencies

- #012: Beam Conflict Detection (needs conflicts to resolve)
- #013: Power Budget Tracking (may adjust power allocation)
- #014: Frequency Interference Modeling (may use frequency separation)

## Related Issues

- #007: Preemption Logic (related but different - preemption is for capacity, resolution is for conflicts)

## Notes

Conflict resolution ensures the scheduler can handle resource contention gracefully. Different strategies suit different use cases.

