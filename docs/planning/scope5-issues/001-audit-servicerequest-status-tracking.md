# Issue #001: Audit and Extend ServiceRequest Status Tracking

**Labels:** `scope5`, `prep`, `servicerequest`, `status-tracking`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High (Foundation)

## Description

Scope 3 introduced ServiceRequest with status fields (`IsProvisionedNow`, `ProvisionedIntervals`) that were not actively updated in Scope 4. For Scope 5, we need to verify and extend the data model to support active status tracking.

## Tasks

1. **Audit existing data model** in `model/servicerequest.go`:
   - Verify `IsProvisionedNow bool` field exists
   - Verify `ProvisionedIntervals []TimeInterval` field exists
   - Add missing fields if needed:
     - `LastProvisionedAt time.Time`
     - `LastUnprovisionedAt time.Time`

2. **Define TimeInterval type** if not already defined:
   ```go
   type TimeInterval struct {
       StartTime time.Time
       EndTime   time.Time
       Path      *Path // which path was used (optional)
   }
   ```

3. **Extend ScenarioState** in `internal/sim/state/state.go`:
   - Add `UpdateServiceRequestStatus(srID string, isProvisioned bool, interval *TimeInterval) error`
   - Add `GetServiceRequestStatus(srID string) (*ServiceRequestStatus, error)`
   - Ensure thread-safe access with proper locking

4. **Add ServiceRequestStatus type**:
   ```go
   type ServiceRequestStatus struct {
       IsProvisionedNow    bool
       CurrentInterval     *TimeInterval
       AllIntervals        []TimeInterval
       LastProvisionedAt   time.Time
       LastUnprovisionedAt time.Time
   }
   ```

## Acceptance Criteria

- [ ] ServiceRequest model includes all required status fields
- [ ] TimeInterval type is defined and documented
- [ ] ScenarioState has `UpdateServiceRequestStatus` method that safely updates status
- [ ] ScenarioState has `GetServiceRequestStatus` method that returns current status
- [ ] All methods are thread-safe and use proper locking
- [ ] Unit tests verify status update and retrieval
- [ ] Status updates persist correctly across scheduler operations

## Dependencies

- None (foundation issue)

## Related Issues

- #002: Extend routing table model for multi-hop
- #006: ServiceRequest Status Tracking & Updates (will use these helpers)

## Notes

These helpers will be called by the scheduler when paths are found/lost. This is a foundational issue that must be completed before implementing active status tracking in the scheduler.

