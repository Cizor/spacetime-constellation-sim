# Issue #011: Implement Path Validation and Scheduling

**Labels:** `scope5`, `scheduling`, `path-validation`, `beam-scheduling`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High

## Description

When a multi-hop path is found, validate it, schedule UpdateBeam and SetRoute actions for each hop, and update ServiceRequest status to reflect provisioning.

## Tasks

1. **Path validation**:
   - Validate each hop's contact window is still valid
   - Verify all nodes in path exist
   - Verify all links in path exist
   - Check bandwidth availability (via #006)

2. **Schedule UpdateBeam actions**:
   - For each link in path, schedule UpdateBeam at correct time
   - Use contact window start time minus lead time
   - Associate beam with ServiceRequest ID

3. **Schedule SetRoute actions**:
   - For each intermediate node, schedule SetRoute
   - Route destination via next hop in path
   - Use multi-hop route installation (from #002)

4. **Update ServiceRequest status**:
   - Call `UpdateServiceRequestStatus` (from #001)
   - Set `IsProvisionedNow = true`
   - Add current interval to `ProvisionedIntervals`
   - Record path information

5. **Error handling**:
   - If validation fails, return error before scheduling
   - If scheduling fails partway, rollback previous actions
   - Handle race conditions (path becomes invalid during scheduling)

## Acceptance Criteria

- [ ] Path validation checks all required conditions
- [ ] UpdateBeam actions are scheduled for each link at correct times
- [ ] SetRoute actions are scheduled for each intermediate node
- [ ] ServiceRequest status is updated correctly
- [ ] Error handling prevents partial scheduling
- [ ] Unit tests verify validation logic
- [ ] Unit tests verify action scheduling
- [ ] Unit tests verify status updates
- [ ] Integration tests verify end-to-end path scheduling

## Dependencies

- #001: Audit ServiceRequest Status Tracking (needs status update helpers)
- #002: Extend Routing Table for Multi-Hop (needs multi-hop route installation)
- #006: Bandwidth-Aware Allocation (needs capacity checking)
- #010: Multi-Hop Pathfinding Algorithm (needs computed paths)

## Related Issues

- #015: ServiceRequest Status Tracking & Updates (will use status updates)

## Notes

This ties together pathfinding, validation, and scheduling. It's the integration point where computed paths become scheduled actions.

