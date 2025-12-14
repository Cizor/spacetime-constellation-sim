# Issue #019: Integrate Re-Planning Loop into Scheduler

**Labels:** `scope5`, `replanning`, `integration`, `scheduler`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High

## Description

Integrate reactive re-planning into the scheduler's periodic run loop. Periodically check active paths and recompute/update them when needed.

## Tasks

1. **Add re-planning to scheduler run loop** in `internal/sbi/controller/scheduler.go`:
   - Every N simulation seconds (configurable)
   - Or on connectivity change events
   - For each active ServiceRequest:
     - Check if re-planning is needed (via #017)
     - If yes, compute new path (via #010)
     - If new path is better, update (via #018)

2. **Re-planning frequency control**:
   - Configurable re-planning interval
   - Minimum time between re-plans for same SR
   - Respect re-planning frequency limits (from #017)

3. **Event-driven re-planning**:
   - Trigger re-planning on connectivity changes
   - Trigger on new ServiceRequest arrival
   - Trigger on path health degradation

4. **Re-planning coordination**:
   - Avoid re-planning all SRs simultaneously
   - Prioritize critical SRs
   - Batch re-planning when possible

5. **Integration with existing scheduler**:
   - Re-planning runs alongside initial path computation
   - Re-planning doesn't block new SR scheduling
   - Proper locking to prevent race conditions

## Acceptance Criteria

- [ ] Re-planning loop is integrated into scheduler
- [ ] Re-planning runs periodically or on events
- [ ] Frequency limits are respected
- [ ] Re-planning doesn't block other operations
- [ ] Event-driven triggers work correctly
- [ ] Unit tests verify re-planning loop
- [ ] Unit tests verify frequency control
- [ ] Integration tests verify re-planning in realistic scenarios

## Dependencies

- #016: Path Monitoring (needs active paths)
- #017: Trigger Conditions for Re-Planning (needs trigger logic)
- #018: Incremental Path Updates (needs update logic)
- #010: Multi-Hop Pathfinding Algorithm (needs path computation)

## Related Issues

- All previous re-planning issues (#016, #017, #018)

## Notes

This integrates all re-planning components into a cohesive system. It's the final piece that makes reactive re-planning work end-to-end.

