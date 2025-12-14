# Issue #004: Extend Telemetry Model for Modem Metrics

**Labels:** `scope5`, `prep`, `telemetry`, `modem-metrics`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** Medium (Foundation)

## Description

Scope 4 stubbed ModemMetrics in telemetry. For Scope 5 enhanced telemetry, we need to define the complete ModemMetrics data structure and extend TelemetryState to store modem metrics alongside interface metrics.

## Tasks

1. **Define ModemMetrics type** in `internal/sbi/telemetry_config.go` or new file:
   ```go
   type ModemMetrics struct {
       InterfaceID string
       SNRdB       float64
       Modulation  string
       CodingRate  string
       BER         float64 // bit error rate
       ThroughputBps uint64
       Timestamp   time.Time
   }
   ```

2. **Extend TelemetryState** in `internal/sim/state/telemetry.go`:
   - Add `modemMetrics map[string]*ModemMetrics` (keyed by interfaceID)
   - Add `UpdateModemMetrics(metrics *ModemMetrics) error`
   - Add `GetModemMetrics(interfaceID string) (*ModemMetrics, error)`
   - Thread-safe access with proper locking

3. **Integrate with existing telemetry storage**:
   - Modem metrics should be associated with interface metrics
   - Support querying both interface and modem metrics together

4. **Add validation**:
   - Ensure InterfaceID exists
   - Validate metric ranges (SNR, BER, etc.)

## Acceptance Criteria

- [ ] ModemMetrics type is defined with all required fields
- [ ] TelemetryState can store modem metrics per interface
- [ ] UpdateModemMetrics correctly stores/updates metrics
- [ ] GetModemMetrics retrieves metrics for an interface
- [ ] All operations are thread-safe
- [ ] Modem metrics are associated with interface metrics
- [ ] Unit tests verify modem metrics storage and retrieval
- [ ] Integration tests verify metrics are updated correctly

## Dependencies

- None (foundation issue)

## Related Issues

- #020: Enhanced Telemetry: Modem Metrics Collection (will populate these metrics)
- #021: Intent/ServiceRequest Telemetry (may reference modem metrics)

## Notes

This prepares the data model for enhanced telemetry. The actual collection of modem metrics will be implemented in later issues. This is a foundation that must be in place first.

