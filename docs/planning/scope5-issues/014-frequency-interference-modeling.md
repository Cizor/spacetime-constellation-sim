# Issue #014: Implement Frequency Interference Modeling

**Labels:** `scope5`, `conflict-detection`, `frequency`, `interference`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** Medium

## Description

Model frequency interference for wireless links. Detect when beams on the same or overlapping frequencies cause interference that exceeds acceptable thresholds.

## Tasks

1. **Define FrequencyInterference type** in `internal/sbi/controller/conflicts.go`:
   ```go
   type FrequencyInterference struct {
       FrequencyGHz float64
       InterferingBeams []BeamSpec
       InterferenceLeveldB float64
   }
   ```

2. **Implement interference computation**:
   ```go
   func ComputeInterference(beam BeamSpec, allBeams []BeamSpec) float64
   ```

3. **Interference calculation logic**:
   - Identify beams on same or overlapping frequencies
   - Compute interference level (dB)
   - Consider beam power, frequency separation, time overlap
   - Return interference level

4. **Interference threshold**:
   - Define acceptable interference threshold (configurable)
   - Mark as conflict if interference exceeds threshold
   - Consider link quality degradation

5. **Integration with conflict detection**:
   - Use interference computation in conflict detection (#012)
   - Mark conflicts when interference is too high
   - Support interference-aware resolution

## Acceptance Criteria

- [ ] FrequencyInterference type is defined correctly
- [ ] ComputeInterference identifies interfering beams
- [ ] Interference level is computed correctly
- [ ] Interference threshold is configurable
- [ ] Conflicts are detected when interference exceeds threshold
- [ ] Unit tests verify interference computation
- [ ] Unit tests verify threshold detection
- [ ] Integration tests verify interference in realistic scenarios

## Dependencies

- None (can be implemented independently)

## Related Issues

- #012: Beam Conflict Detection (will use interference modeling)
- #015: Conflict Resolution Strategies (may use frequency separation)

## Notes

Frequency interference modeling enables detection of conflicts where beams interfere with each other even if they don't violate MaxBeams or power limits.

