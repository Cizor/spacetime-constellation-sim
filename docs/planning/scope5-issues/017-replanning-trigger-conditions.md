# Issue #017: Implement Trigger Conditions for Re-Planning

**Labels:** `scope5`, `replanning`, `triggers`, `scheduling`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High

## Description

Define and implement conditions that trigger re-planning of active ServiceRequest paths. Re-plan when paths break, better paths become available, or quality degrades.

## Tasks

1. **Implement ShouldReplan function** in `internal/sbi/controller/scheduler.go`:
   ```go
   func (s *Scheduler) ShouldReplan(srID string, now time.Time) bool
   ```

2. **Trigger conditions**:
   - **Contact window closes early**: Path broken → trigger re-plan
   - **New contact window opens**: Better path potentially available → trigger re-plan
   - **Link quality degrades**: Below threshold → trigger re-plan
   - **Higher-priority SR arrives**: May need to re-plan lower-priority SRs → trigger re-plan

3. **Condition evaluation logic**:
   - Check path health (from #016)
   - Compare current path with potential new paths
   - Evaluate if re-planning would improve situation
   - Consider re-planning cost (don't thrash)

4. **Re-planning frequency limits**:
   - Prevent excessive re-planning (thrashing)
   - Minimum time between re-plans for same SR
   - Configurable re-planning interval

5. **Integration with monitoring**:
   - ShouldReplan called periodically or on events
   - Efficient evaluation (don't check unnecessarily)
   - Log re-planning triggers for observability

## Acceptance Criteria

- [ ] ShouldReplan correctly identifies when re-planning is needed
- [ ] All trigger conditions are evaluated
- [ ] Re-planning frequency limits prevent thrashing
- [ ] Early contact window closure triggers re-plan
- [ ] New better paths trigger re-plan
- [ ] Quality degradation triggers re-plan
- [ ] Unit tests verify each trigger condition
- [ ] Unit tests verify frequency limits
- [ ] Integration tests verify re-planning triggers in realistic scenarios

## Dependencies

- #016: Path Monitoring (needs path health checking)

## Related Issues

- #018: Incremental Path Updates (will be triggered by these conditions)
- #019: Re-Planning Loop (will use ShouldReplan)

## Notes

Trigger conditions determine when re-planning is beneficial. They must balance responsiveness with stability (avoid thrashing).

