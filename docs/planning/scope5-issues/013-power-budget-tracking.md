# Issue #013: Implement Power Budget Tracking

**Labels:** `scope5`, `conflict-detection`, `power`, `transceiver`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High

## Description

Extend TransceiverModel to include power limits and track per-interface power allocation. This enables conflict detection based on power budget constraints.

## Tasks

1. **Extend TransceiverModel** in `core/transceiver_model.go`:
   ```go
   type TransceiverModel struct {
       // existing fields...
       MaxPowerWatts float64
   }
   ```

2. **Define InterfacePowerState** in `internal/sim/state/state.go` or new file:
   ```go
   type InterfacePowerState struct {
       InterfaceID string
       AllocatedPower float64
       Beams []BeamSpec // active beams
   }
   ```

3. **Add power tracking to ScenarioState**:
   - `AllocatePower(interfaceID string, powerWatts float64) error`
   - `ReleasePower(interfaceID string, powerWatts float64) error`
   - `GetAvailablePower(interfaceID string) float64`

4. **Power allocation logic**:
   - Track total allocated power per interface
   - Check against MaxPowerWatts from TransceiverModel
   - Prevent over-allocation
   - Associate power with specific beams

5. **Integration with beam scheduling**:
   - Allocate power when beam is activated
   - Release power when beam is deactivated
   - Check power availability before scheduling

## Acceptance Criteria

- [ ] TransceiverModel includes MaxPowerWatts field
- [ ] InterfacePowerState tracks allocated power and active beams
- [ ] AllocatePower correctly allocates and tracks power
- [ ] ReleasePower correctly releases power
- [ ] GetAvailablePower returns correct available power
- [ ] Power allocation prevents over-allocation
- [ ] Unit tests verify power allocation and release
- [ ] Unit tests verify power limit enforcement
- [ ] Integration tests verify power tracking across beam lifecycle

## Dependencies

- None (foundation issue)

## Related Issues

- #012: Beam Conflict Detection (will use power tracking)
- #015: Conflict Resolution Strategies (may adjust power allocation)

## Notes

Power budget tracking is needed for conflict detection. It ensures beams don't exceed interface power limits.

