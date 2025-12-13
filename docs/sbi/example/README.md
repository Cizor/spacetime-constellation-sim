# SBI Example Scenario

This directory contains the minimal SBI (Southbound Interface) example scenario that demonstrates end-to-end SBI scheduling, agent execution, link activation, routing, and telemetry.

## Scenario Overview

The example scenario consists of:
- **1 LEO satellite** (`sat1`) with a downlink interface (`if-sat1-down`)
- **1 ground station** (`gs1`) with an uplink interface (`if-gs1-up`)
- **1 potential wireless link** between the satellite and ground station
- **1 ServiceRequest** (optional, can be added via NBI)

This minimal setup exercises the full SBI pipeline:
- CDPI scheduling (CreateEntry / DeleteEntry / Finalize)
- Agent-side schedule execution (UpdateBeam / DeleteBeam, SetRoute / DeleteRoute)
- Telemetry loop (ExportMetrics → TelemetryState)
- Basic scheduler logic (link-driven beams, ServiceRequest-aware routing)

## Scenario File

The network scenario is defined in `configs/scope4_sbi_example.json`. This file contains:
- Network interfaces for both nodes
- A single potential link between them
- Initial node positions (ground station at Earth surface, satellite at ~500km altitude)

## How to Run

### Prerequisites

1. Build the simulator:
   ```bash
   go build ./cmd/simulator
   ```

2. Ensure transceiver models are available:
   - The scenario uses `trx-ku` transceiver model
   - This should be defined in `configs/transceivers.json`

### Running the Scenario

The scenario can be loaded using the existing simulator with the network scenario file:

```bash
./simulator \
  --duration=300s \
  --tick=1s \
  --accelerated=true
```

Note: The simulator currently loads `configs/network_scenario.json` by default. To use the SBI example scenario, you'll need to either:
- Replace the default scenario file, or
- Modify `cmd/simulator/main.go` to load `configs/scope4_sbi_example.json`

### Expected Behavior

When running with SBI enabled:

1. **At T₀ (link becomes visible):**
   - Scheduler issues `UpdateBeam` + `SetRoute` for the sat-ground link
   - Agent applies `UpdateBeam`/`SetRoute`; link becomes active
   - Telemetry metrics for `sat1/if-sat1-down` and `gs1/if-gs1-up` show `Up=true` and `BytesTx` increasing

2. **At T₁ (link goes out-of-view):**
   - Scheduler issues `DeleteBeam` + `DeleteRoute`
   - Agent deactivates the link and removes routes
   - Telemetry shows `Up=false`

## Platform Setup

The scenario requires platforms to be created separately. For a complete setup:

1. **LEO Satellite Platform:**
   - Platform ID: `sat1`
   - Type: `SATELLITE`
   - Motion: TLE-based orbit (can use ISS TLE as example)
   - Node: `node-sat1` attached to platform `sat1`

2. **Ground Station Platform:**
   - Platform ID: `gs1`
   - Type: `GROUND_STATION`
   - Motion: Static position at Earth surface
   - Node: `node-gs1` attached to platform `gs1`

## ServiceRequest (Optional)

A ServiceRequest can be added via NBI to exercise ServiceRequest-aware scheduling:
- Source: `gs1` (ground station)
- Destination: `sat1` (satellite)
- Flow requirements: bandwidth, latency constraints

## Next Steps

For detailed documentation on:
- Expected SBI behavior timeline: see `docs/sbi/example/behavior.md`
- How to run SBI in the simulator: see `docs/sbi/example/running.md`
- Sample logs and walkthrough: see `docs/sbi/example/walkthrough.md`

