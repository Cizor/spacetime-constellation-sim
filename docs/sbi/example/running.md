# How to Run SBI in the Simulator

This guide explains how to run the simulator with SBI (Southbound Interface) enabled and observe the scheduling, agent execution, and telemetry behavior.

## Prerequisites

1. **Build the simulator:**
   ```bash
   go build ./cmd/nbi-server
   ```

2. **Ensure transceiver models exist:**
   - The scenario requires transceiver models defined in `configs/transceivers.json`
   - Default path: `configs/transceivers.json`
   - Must include at least the `trx-ku` model used by the example scenario

3. **Prepare the scenario:**
   - Use the SBI example scenario: `configs/scope4_sbi_example.json`
   - Or use your own scenario with nodes, interfaces, and links

## Running the Simulator

### Basic Command

```bash
./nbi-server \
  --network-scenario=configs/scope4_sbi_example.json \
  --transceivers=configs/transceivers.json \
  --tick=1s \
  --accelerated=true
```

### Command-Line Options

- `--network-scenario`: Path to network scenario JSON file (default: empty, loads via NBI)
- `--transceivers`: Path to transceiver models JSON file (default: `configs/transceivers.json`)
- `--tick`: Simulation tick interval (default: `1s`)
- `--accelerated`: Run in accelerated mode vs real-time (default: `true`)
- `--listen-address`: gRPC server listen address (default: `0.0.0.0:50051`)
- `--log-level`: Log level: `debug`, `info`, `warn` (default: `info`)
- `--log-format`: Log format: `text` or `json` (default: `text`)

### Environment Variables

You can also configure via environment variables:

```bash
export NBI_NETWORK_SCENARIO=configs/scope4_sbi_example.json
export NBI_TRANSCEIVERS_PATH=configs/transceivers.json
export NBI_TICK_INTERVAL=1s
export NBI_ACCELERATED=true
export LOG_LEVEL=info
export LOG_FORMAT=text

./nbi-server
```

## SBI Components

When the simulator starts with SBI enabled, the following components are automatically initialized:

1. **EventScheduler**: Bound to the simulation time controller
2. **TelemetryState**: Stores metrics received from agents
3. **TelemetryServer**: gRPC server for receiving telemetry from agents
4. **CDPIServer**: gRPC server for controller-agent communication
5. **Scheduler**: Controller-side scheduler that generates scheduled actions
6. **Agents**: One agent per network node, connecting to CDPI and Telemetry servers

## Startup Sequence

When you start the simulator, you should see logs indicating:

1. **Scenario Loading:**
   ```
   [INFO] Loading network scenario from configs/scope4_sbi_example.json
   [INFO] Loaded network scenario: 2 interfaces, 1 links, 2 nodes with positions
   ```

2. **SBI Initialization:**
   ```
   [INFO] Starting NBI gRPC server addr=0.0.0.0:50051
   [INFO] SBI: Creating agents for 2 nodes
   [INFO] SBI: Starting agents
   ```

3. **Agent Connection:**
   ```
   [INFO] Agent: Starting agent_id=agent-sat1 node_id=sat1
   [INFO] Agent: Hello sent to controller
   [INFO] CDPI: Agent Hello received agent_id=agent-sat1 node_id=sat1
   [INFO] Agent: Starting agent_id=agent-gs1 node_id=gs1
   [INFO] Agent: Hello sent to controller
   [INFO] CDPI: Agent Hello received agent_id=agent-gs1 node_id=gs1
   ```

4. **Initial Scheduling:**
   ```
   [INFO] Scheduler: RunInitialSchedule completed links=1
   [INFO] Scheduler: ScheduleLinkBeams scheduled 1 beam actions
   ```

## Observing SBI Behavior

### 1. Watch for Link Activation

When a link comes into view, you should see:

```
[INFO] Scheduler: ScheduleLinkBeams scheduled 1 beam actions
[INFO] CDPI: SendCreateEntry agent_id=agent-sat1 entry_id=entry-beam-001 action=UpdateBeam
[INFO] Agent: CreateEntry received entry_id=entry-beam-001 action=UpdateBeam when=2024-01-01T12:00:00Z
[INFO] Agent: Action scheduled entry_id=entry-beam-001
[INFO] Agent: Executing action entry_id=entry-beam-001 type=UpdateBeam
[INFO] Agent: UpdateBeam applied beam=if-sat1-down→if-gs1-up
[INFO] Agent: Response sent request_id=entry-beam-001 status=OK
```

### 2. Watch for Telemetry

Telemetry is exported periodically (default: every 1 second):

```
[INFO] Agent: Telemetry export node_id=sat1 interfaces=1
[INFO] Telemetry: ExportMetrics received node_id=sat1 interface_count=1
[INFO] Telemetry: Interface if-sat1-down Up=true BytesTx=1024
[INFO] Agent: Telemetry export node_id=gs1 interfaces=1
[INFO] Telemetry: Interface if-gs1-up Up=true BytesTx=1024
```

### 3. Watch for Link Deactivation

When a link goes out of view:

```
[INFO] Scheduler: ScheduleLinkBeams scheduled 1 delete actions
[INFO] CDPI: SendDeleteEntry agent_id=agent-sat1 entry_id=entry-beam-001 action=DeleteBeam
[INFO] Agent: DeleteEntry received entry_id=entry-beam-001
[INFO] Agent: DeleteBeam applied beam=if-sat1-down→if-gs1-up
[INFO] Agent: Response sent request_id=entry-beam-001 status=OK
```

## Querying Telemetry via NBI

You can query telemetry metrics via the NBI TelemetryService:

```bash
# Using grpcurl (if installed)
grpcurl -plaintext localhost:50051 \
  aalyria.spacetime.api.nbi.v1alpha.TelemetryService/ListInterfaceMetrics \
  '{}'

# Or with filters
grpcurl -plaintext localhost:50051 \
  aalyria.spacetime.api.nbi.v1alpha.TelemetryService/ListInterfaceMetrics \
  '{"node_id": "sat1"}'
```

## Debugging Tips

### Enable Debug Logging

To see more detailed logs:

```bash
./nbi-server \
  --network-scenario=configs/scope4_sbi_example.json \
  --log-level=debug \
  --log-format=text
```

### Verify Agent State

You can use the `DumpAgentState` helper (if exposed via a debug endpoint) to inspect agent state:

- Pending scheduled actions
- Current token
- Last sequence number seen
- SR policies (if any)
- Telemetry metrics snapshot

### Verify Scheduler State

Check scheduler logs for:
- Link-driven beam scheduling
- ServiceRequest-aware routing
- Action generation and CDPI delivery

### Verify TelemetryState

Check telemetry logs for:
- ExportMetrics requests received
- Interface metrics updates
- BytesTx/BytesRx evolution

## Common Issues

### Agents Not Connecting

**Symptoms:**
- No "Agent Hello received" logs
- CDPI errors about missing agents

**Solutions:**
- Verify nodes exist in ScenarioState
- Check that gRPC server is running
- Verify agent startup logs for errors

### No Scheduled Actions

**Symptoms:**
- No "ScheduleLinkBeams" logs
- No CreateEntry messages

**Solutions:**
- Verify links exist in ScenarioState
- Check that links are marked as "potential"
- Verify connectivity service is evaluating links
- Check scheduler logs for errors

### Telemetry Not Updating

**Symptoms:**
- No "Telemetry export" logs
- TelemetryState shows stale data

**Solutions:**
- Verify telemetry is enabled in agent config
- Check telemetry interval is > 0
- Verify TelemetryServer is registered
- Check agent telemetry loop logs

## Next Steps

- **Expected Behavior**: See `docs/sbi/example/behavior.md` for detailed behavior documentation
- **Timeline**: See `docs/sbi/example/timeline.md` for chronological event sequence
- **Walkthrough**: See `docs/sbi/example/walkthrough.md` for step-by-step walkthrough

