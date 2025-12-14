# Issue #025: Expose ServiceRequest Status via NBI

**Labels:** `scope5`, `nbi`, `servicerequest`, `api`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** Medium

## Description

Extend ServiceRequestService NBI to expose status information. Allow clients to query current provisioning state and history.

## Tasks

1. **Extend ServiceRequestService** in `internal/nbi/servicerequest_service.go`:
   - Add `GetServiceRequestStatus` RPC:
     ```go
     GetServiceRequestStatus(GetServiceRequestStatusRequest) returns (ServiceRequestStatus)
     ```

2. **Define ServiceRequestStatus proto message** (or use existing):
   - IsProvisionedNow
   - CurrentProvisionedInterval
   - AllProvisionedIntervals (history)
   - LastProvisionedAt
   - LastUnprovisionedAt

3. **Implement status query**:
   - Use GetServiceRequestStatus from ScenarioState (#001)
   - Convert internal status to proto
   - Return status to client

4. **Status response includes**:
   - Current provisioning state
   - Current interval (if provisioned)
   - Full interval history
   - Timestamps for last provision/unprovision

5. **Error handling**:
   - Return NotFound if SR doesn't exist
   - Handle missing status gracefully

## Acceptance Criteria

- [ ] GetServiceRequestStatus RPC is implemented
- [ ] Status proto message includes all required fields
- [ ] Status query returns current state
- [ ] Status query returns interval history
- [ ] Error handling is correct
- [ ] Unit tests verify NBI status query
- [ ] Integration tests verify status exposure
- [ ] Example client can query status

## Dependencies

- #001: Audit ServiceRequest Status Tracking (needs status helpers)
- #024: Active ServiceRequest Status Updates (needs active status tracking)

## Related Issues

- #024: Active ServiceRequest Status Updates (provides status data)

## Notes

NBI status exposure enables external clients to monitor ServiceRequest provisioning state. This is important for observability and integration with external systems.

