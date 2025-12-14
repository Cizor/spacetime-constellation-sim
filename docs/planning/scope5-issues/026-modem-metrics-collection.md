# Issue #026: Implement Modem Metrics Collection in Agents

**Labels:** `scope5`, `telemetry`, `modem-metrics`, `agent`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** Medium

## Description

Extend agent telemetry generation to collect modem-level metrics (SNR, modulation, coding rate, BER, throughput) for active interfaces.

## Tasks

1. **Extend agent** in `internal/sbi/agent/agent.go`:
   - Add `CollectModemMetrics(interfaceID string) *ModemMetrics` method

2. **Metrics collection logic**:
   - When link is active, collect:
     - SNR from connectivity evaluation
     - Modulation/coding from transceiver model
     - Throughput from active flows (bandwidth utilization)
     - BER (bit error rate) - can be derived or modeled

3. **Periodic collection**:
   - Every telemetry interval (configurable)
   - Collect metrics for all active interfaces
   - Include in ExportMetrics request

4. **Metrics derivation**:
   - SNR: from connectivity service link quality
   - Modulation/Coding: from transceiver model configuration
   - Throughput: from active ServiceRequest bandwidth usage
   - BER: model based on SNR and modulation

5. **Integration with telemetry**:
   - Use ModemMetrics type from #004
   - Update TelemetryState via UpdateModemMetrics
   - Include in telemetry export

## Acceptance Criteria

- [ ] CollectModemMetrics collects all required metrics
- [ ] SNR is collected from connectivity evaluation
- [ ] Modulation/coding is collected from transceiver model
- [ ] Throughput is calculated from active flows
- [ ] BER is modeled or derived
- [ ] Metrics are collected periodically
- [ ] Metrics are included in telemetry export
- [ ] Unit tests verify metrics collection
- [ ] Integration tests verify metrics in realistic scenarios

## Dependencies

- #004: Extend Telemetry Model for Modem Metrics (needs ModemMetrics type)

## Related Issues

- #027: Intent/ServiceRequest Telemetry (related telemetry enhancement)

## Notes

Modem metrics provide detailed link quality information beyond basic interface metrics. This is useful for debugging and link quality monitoring.

