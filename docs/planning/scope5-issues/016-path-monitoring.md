# Issue #016: Implement Path Monitoring for Active ServiceRequests

**Labels:** `scope5`, `replanning`, `monitoring`, `paths`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High

## Description

Track active ServiceRequest paths and monitor their health. This enables reactive re-planning when paths break or better paths become available.

## Tasks

1. **Define ActivePath type** in `internal/sbi/controller/scheduler.go`:
   ```go
   type ActivePath struct {
       ServiceRequestID string
       Path             *Path
       ScheduledActions []string // entry IDs
       LastUpdated      time.Time
       Health           PathHealth
   }
   ```

2. **Define PathHealth type**:
   ```go
   type PathHealth string
   const (
       HealthHealthy PathHealth = "healthy"
       HealthDegraded PathHealth = "degraded"
       HealthBroken PathHealth = "broken"
   )
   ```

3. **Add active path tracking to Scheduler**:
   - `activePaths map[string]*ActivePath` (keyed by ServiceRequestID)
   - Track all active paths for all provisioned SRs

4. **Path monitoring logic**:
   - Monitor contact window changes (links go down earlier/later than expected)
   - Detect new better paths become available (shorter, lower latency)
   - Detect link quality degradation
   - Update path health status

5. **Health check function**:
   ```go
   func (s *Scheduler) CheckPathHealth(path *Path, now time.Time) PathHealth
   ```
   - Check if all hops are still valid
   - Check if contact windows are still active
   - Check link quality metrics
   - Return health status

## Acceptance Criteria

- [ ] ActivePath type is defined correctly
- [ ] PathHealth type represents path states
- [ ] Scheduler tracks all active paths
- [ ] CheckPathHealth correctly evaluates path health
- [ ] Path monitoring detects window changes
- [ ] Path monitoring detects quality degradation
- [ ] Unit tests verify path tracking
- [ ] Unit tests verify health checking
- [ ] Integration tests verify monitoring in realistic scenarios

## Dependencies

- #010: Multi-Hop Pathfinding Algorithm (needs Path type)
- #011: Path Validation and Scheduling (paths are created here)

## Related Issues

- #017: Trigger Conditions for Re-Planning (will use path health)
- #018: Incremental Path Updates (will update monitored paths)

## Notes

Path monitoring is the foundation for reactive re-planning. It continuously evaluates whether active paths are still valid and healthy.

