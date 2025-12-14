# Issue #027: Implement Intent/ServiceRequest Telemetry Tracking

**Labels:** `scope5`, `telemetry`, `intent`, `servicerequest`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** Medium

## Description

Track ServiceRequest fulfillment metrics (provisioned duration, fulfillment rate, latency, bytes transferred) and expose via telemetry.

## Tasks

1. **Define IntentMetrics type** in `internal/sim/state/telemetry.go`:
   ```go
   type IntentMetrics struct {
       ServiceRequestID string
       IsProvisioned    bool
       ProvisionedDuration time.Duration
       TotalDuration    time.Duration
       FulfillmentRate  float64 // provisioned / total
       AverageLatency    time.Duration
       BytesTransferred uint64
   }
   ```

2. **Add to TelemetryState**:
   - `intentMetrics map[string]*IntentMetrics` (keyed by SR ID)
   - `UpdateIntentMetrics(metrics *IntentMetrics) error`
   - `GetIntentMetrics(srID string) *IntentMetrics`

3. **Metrics calculation**:
   - ProvisionedDuration: sum of all provisioned intervals
   - TotalDuration: time since SR creation
   - FulfillmentRate: ProvisionedDuration / TotalDuration
   - AverageLatency: average path latency when provisioned
   - BytesTransferred: bandwidth * provisioned duration

4. **Scheduler updates metrics**:
   - Update as SRs are provisioned/unprovisioned
   - Update on path changes
   - Calculate metrics periodically or on state change

5. **Telemetry export**:
   - Include intent metrics in telemetry export
   - Support querying intent metrics via telemetry service

## Acceptance Criteria

- [ ] IntentMetrics type is defined correctly
- [ ] TelemetryState tracks intent metrics
- [ ] Metrics are calculated correctly
- [ ] Scheduler updates metrics on state changes
- [ ] Metrics are included in telemetry export
- [ ] Unit tests verify metrics calculation
- [ ] Unit tests verify metrics updates
- [ ] Integration tests verify metrics in realistic scenarios

## Dependencies

- #024: Active ServiceRequest Status Updates (needs status for metrics)

## Related Issues

- #026: Modem Metrics Collection (related telemetry)
- #028: Telemetry Aggregation (may aggregate intent metrics)

## Notes

Intent metrics provide ServiceRequest-level fulfillment statistics. This is important for understanding how well the scheduler is meeting service requirements.

