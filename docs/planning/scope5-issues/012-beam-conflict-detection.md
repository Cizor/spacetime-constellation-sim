# Issue #012: Implement Beam Conflict Detection

**Labels:** `scope5`, `conflict-detection`, `beams`, `scheduling`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High

## Description

Detect when multiple beams are scheduled on the same interface, violating MaxBeams constraints, power limits, or causing frequency interference.

## Tasks

1. **Define BeamConflict type** in `internal/sbi/controller/conflicts.go`:
   ```go
   type BeamConflict struct {
       InterfaceID string
       ConflictingBeams []BeamSpec
       ConflictType string // "concurrent_beams", "power_limit", "frequency"
   }
   ```

2. **Implement conflict detection function**:
   ```go
   func DetectBeamConflicts(interfaceID string, scheduledBeams []BeamSpec) []BeamConflict
   ```

3. **Check MaxBeams constraint**:
   - If interface has MaxBeams=2, only 2 concurrent beams allowed
   - Detect when more beams are scheduled than allowed
   - Return conflict with type "concurrent_beams"

4. **Check power budget**:
   - Sum power of all scheduled beams
   - Compare to interface power limit (from TransceiverModel)
   - Return conflict with type "power_limit" if exceeded

5. **Check frequency interference**:
   - Identify beams on same or overlapping frequencies
   - Compute interference level
   - Return conflict with type "frequency" if interference exceeds threshold

6. **Integration with scheduler**:
   - Check conflicts before scheduling new beams
   - Check conflicts when re-planning
   - Maintain conflict state for active beams

## Acceptance Criteria

- [ ] BeamConflict type is defined correctly
- [ ] DetectBeamConflicts identifies MaxBeams violations
- [ ] DetectBeamConflicts identifies power limit violations
- [ ] DetectBeamConflicts identifies frequency interference
- [ ] Conflicts are correctly classified by type
- [ ] Unit tests verify each conflict type detection
- [ ] Unit tests verify multiple conflict types on same interface
- [ ] Integration tests verify conflict detection in realistic scenarios

## Dependencies

- #013: Power Budget Tracking (needs power tracking for power limit checks)

## Related Issues

- #013: Power Budget Tracking (dependency)
- #014: Frequency Interference Modeling (dependency)
- #015: Conflict Resolution Strategies (will use detected conflicts)

## Notes

Conflict detection is the first step in conflict resolution. It must accurately identify all types of conflicts before resolution can occur.

