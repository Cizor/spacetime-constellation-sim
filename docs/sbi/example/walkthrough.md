# SBI Example Scenario Walkthrough

This document provides a step-by-step walkthrough for loading, running, and observing the SBI example scenario, including expected behavior and logs.

## Overview

This walkthrough demonstrates:
1. Loading the example scenario
2. Starting the simulator with SBI enabled
3. Observing agent connection and handshake
4. Watching link activation (T₁)
5. Monitoring telemetry during active period
6. Observing link deactivation (T₂)
7. Verifying final state

## Step 1: Prepare the Environment

### 1.1 Build the Simulator

```bash
cd /path/to/spacetime-constellation-sim
go build ./cmd/nbi-server
```

### 1.2 Verify Scenario Files

Ensure the following files exist:
- `configs/scope4_sbi_example.json` - The SBI example scenario
- `configs/transceivers.json` - Transceiver models (must include `trx-ku`)

### 1.3 Verify Scenario Content

The scenario should define:
- 2 interfaces: `if-sat1-down`, `if-gs1-up`
- 1 link: `link-sat1-gs1`
- 2 node positions: `sat1`, `gs1`

## Step 2: Start the Simulator

### 2.1 Basic Startup

```bash
./nbi-server \
  --network-scenario=configs/scope4_sbi_example.json \
  --transceivers=configs/transceivers.json \
  --tick=1s \
  --accelerated=true \
  --log-level=info
```

### 2.2 Expected Startup Output

You should see:

```
[INFO] Loading network scenario from configs/scope4_sbi_example.json
[INFO] Loaded network scenario: 2 interfaces, 1 links, 2 nodes with positions
[INFO] Starting NBI gRPC server addr=0.0.0.0:50051
[INFO] SBI: Creating agents for 2 nodes
[INFO] SBI: Starting agents
```

## Step 3: Observe Agent Connection

### 3.1 Agent Startup

Watch for agent startup logs:

```
[INFO] Agent: Starting agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Starting agent_id=agent-gs1 node_id=gs1
```

### 3.2 CDPI Handshake

Each agent sends a Hello message:

```
[INFO] Agent: Hello sent to controller agent_id=agent-sat1 node_id=sat1
[INFO] CDPI: Agent Hello received agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Hello sent to controller agent_id=agent-gs1 node_id=gs1
[INFO] CDPI: Agent Hello received agent_id=agent-gs1 node_id=gs1
```

**What's happening:**
- Agents establish gRPC streams to the CDPI server
- Controller creates `AgentHandle` for each agent
- Controller assigns `schedule_manipulation_token` to each agent
- Controller initializes sequence numbers

### 3.3 Verification

At this point:
- ✅ Two agents are running (one per node)
- ✅ Agents are connected to CDPI server
- ✅ Tokens are assigned
- ✅ Links are not yet active

## Step 4: Observe Initial Scheduling

### 4.1 Scheduler Initialization

```
[INFO] Scheduler: RunInitialSchedule starting
[INFO] Scheduler: ScheduleLinkBeams scanning for potential links
[INFO] Scheduler: ScheduleLinkBeams found 1 potential links
```

### 4.2 Link Evaluation

The scheduler evaluates potential links:

```
[INFO] Scheduler: Evaluating link link-sat1-gs1
[INFO] Scheduler: Link link-sat1-gs1 status=Potential
```

**What's happening:**
- Scheduler calls `ScheduleLinkBeams()` to scan for potential links
- Connectivity service evaluates link geometry
- If link is in view, scheduler generates actions

### 4.3 Action Generation (if link is in view)

If the link is already in view at startup:

```
[INFO] Scheduler: ScheduleLinkBeams scheduled 1 beam actions
[INFO] Scheduler: ScheduleLinkRoutes scheduled 2 route actions
```

**What's happening:**
- Scheduler generates `UpdateBeam` action for the satellite agent
- Scheduler generates `SetRoute` actions for both nodes
- Actions are sent via CDPI to agents

## Step 5: Observe Link Activation (T₁)

### 5.1 Scheduler Detection

When the link comes into view:

```
[INFO] Scheduler: ScheduleLinkBeams detected link link-sat1-gs1 is in view
[INFO] Scheduler: Generating UpdateBeam action for link link-sat1-gs1
[INFO] Scheduler: Generating SetRoute actions for link link-sat1-gs1
```

### 5.2 CDPI Message Delivery

Controller sends CreateEntryRequests:

```
[INFO] CDPI: SendCreateEntry agent_id=agent-sat1 entry_id=entry-beam-001 action=UpdateBeam
[INFO] CDPI: SendCreateEntry agent_id=agent-sat1 entry_id=entry-route-sat1-001 action=SetRoute
[INFO] CDPI: SendCreateEntry agent_id=agent-gs1 entry_id=entry-route-gs1-001 action=SetRoute
[INFO] CDPI: Message sent agent_id=agent-sat1 seqno=1
[INFO] CDPI: Message sent agent_id=agent-sat1 seqno=2
[INFO] CDPI: Message sent agent_id=agent-gs1 seqno=1
```

**What's happening:**
- CDPI server looks up agent handles
- Builds `CreateEntryRequest` messages with tokens and seqnos
- Sends messages on agent's outgoing channel

### 5.3 Agent Reception and Scheduling

Agents receive and schedule actions:

```
[INFO] Agent: CreateEntry received agent_id=agent-sat1 entry_id=entry-beam-001 action=UpdateBeam when=2024-01-01T12:00:00Z
[INFO] Agent: Token validated agent_id=agent-sat1 token=token-abc-123
[INFO] Agent: SeqNo validated agent_id=agent-sat1 seqno=1
[INFO] Agent: Action scheduled agent_id=agent-sat1 entry_id=entry-beam-001 event_id=evt-001
[INFO] Agent: Response sent agent_id=agent-sat1 request_id=entry-beam-001 status=OK message=scheduled
```

**What's happening:**
- Agent validates token (must match current token)
- Agent validates seqno (must be monotonically increasing)
- Agent stores action in local schedule queue
- Agent schedules execution via `EventScheduler`
- Agent sends Response back to controller

### 5.4 Agent Execution

At the scheduled time, agents execute actions:

```
[INFO] Agent: Executing action agent_id=agent-sat1 entry_id=entry-beam-001 type=UpdateBeam when=2024-01-01T12:00:00Z
[INFO] Agent: UpdateBeam applied agent_id=agent-sat1 beam=if-sat1-down→if-gs1-up
[INFO] Agent: Response sent agent_id=agent-sat1 request_id=entry-beam-001 status=OK message=executed
```

**What's happening:**
- `EventScheduler` triggers action execution at scheduled time
- Agent calls `ScenarioState.ApplyBeamUpdate()`
- Link status changes to `LinkStatusActive`
- Link `IsUp` becomes `true`
- Routes are installed on both nodes

### 5.5 Verification

At this point, verify:
- ✅ Link `link-sat1-gs1` has `Status = LinkStatusActive`
- ✅ Link `link-sat1-gs1` has `IsUp = true`
- ✅ Route installed on `sat1`: `node:gs1/32` via `if-sat1-down`
- ✅ Route installed on `gs1`: `node:sat1/32` via `if-gs1-up`
- ✅ Agent sent execution Response with status OK

## Step 6: Monitor Telemetry (T₁+Δ)

### 6.1 Telemetry Export

Every telemetry interval (default: 1 second), agents export metrics:

```
[INFO] Agent: Telemetry export starting agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Building interface metrics agent_id=agent-sat1 interface_count=1
[INFO] Agent: Interface if-sat1-down Up=true BytesTx=1024 BytesRx=0
[INFO] Agent: Telemetry export completed agent_id=agent-sat1 interfaces=1
```

### 6.2 TelemetryServer Reception

TelemetryServer receives and processes metrics:

```
[INFO] Telemetry: ExportMetrics received node_id=sat1 interface_count=1
[INFO] Telemetry: Processing interface node_id=sat1 interface_id=if-sat1-down
[INFO] Telemetry: Interface metrics updated node_id=sat1 interface_id=if-sat1-down Up=true BytesTx=1024
[INFO] Telemetry: ExportMetrics completed node_id=sat1 processed=1
```

### 6.3 BytesTx Evolution

Watch `BytesTx` increase over time:

```
[INFO] Telemetry: Interface if-sat1-down Up=true BytesTx=1024
[INFO] Telemetry: Interface if-sat1-down Up=true BytesTx=2048
[INFO] Telemetry: Interface if-sat1-down Up=true BytesTx=3072
[INFO] Telemetry: Interface if-sat1-down Up=true BytesTx=4096
```

**What's happening:**
- Agent's telemetry loop runs every interval
- Agent derives interface state from `ScenarioState`
- Agent calculates `BytesTx` based on bandwidth and time
- TelemetryServer updates `TelemetryState`
- Metrics available via NBI `TelemetryService`

### 6.4 Query Telemetry via NBI

You can query telemetry metrics:

```bash
# Using grpcurl
grpcurl -plaintext localhost:50051 \
  aalyria.spacetime.api.nbi.v1alpha.TelemetryService/ListInterfaceMetrics \
  '{}'
```

Expected response:
```json
{
  "metrics": [
    {
      "node_id": "sat1",
      "interface_id": "if-sat1-down",
      "up": true,
      "bytes_tx": 4096,
      "bytes_rx": 0
    },
    {
      "node_id": "gs1",
      "interface_id": "if-gs1-up",
      "up": true,
      "bytes_tx": 4096,
      "bytes_rx": 0
    }
  ]
}
```

## Step 7: Observe Link Deactivation (T₂)

### 7.1 Scheduler Detection

When the link goes out of view:

```
[INFO] Scheduler: ScheduleLinkBeams detected link link-sat1-gs1 is out of view
[INFO] Scheduler: Generating DeleteBeam action for link link-sat1-gs1
[INFO] Scheduler: Generating DeleteRoute actions for link link-sat1-gs1
```

### 7.2 CDPI Message Delivery

Controller sends DeleteEntryRequests:

```
[INFO] CDPI: SendDeleteEntry agent_id=agent-sat1 entry_id=entry-beam-001 action=DeleteBeam
[INFO] CDPI: SendDeleteEntry agent_id=agent-sat1 entry_id=entry-route-sat1-001 action=DeleteRoute
[INFO] CDPI: SendDeleteEntry agent_id=agent-gs1 entry_id=entry-route-gs1-001 action=DeleteRoute
[INFO] CDPI: Message sent agent_id=agent-sat1 seqno=3
[INFO] CDPI: Message sent agent_id=agent-sat1 seqno=4
[INFO] CDPI: Message sent agent_id=agent-gs1 seqno=2
```

### 7.3 Agent Processing

Agents receive and process DeleteEntry requests:

```
[INFO] Agent: DeleteEntry received agent_id=agent-sat1 entry_id=entry-beam-001
[INFO] Agent: Token validated agent_id=agent-sat1 token=token-abc-123
[INFO] Agent: SeqNo validated agent_id=agent-sat1 seqno=3
[INFO] Agent: Action cancelled agent_id=agent-sat1 entry_id=entry-beam-001 event_id=evt-001
[INFO] Agent: DeleteBeam applied agent_id=agent-sat1 beam=if-sat1-down→if-gs1-up
[INFO] Agent: Response sent agent_id=agent-sat1 request_id=entry-beam-001 status=OK message=deleted
```

**What's happening:**
- Agent validates token and seqno
- Agent cancels scheduled action (if not yet executed)
- Agent removes action from local schedule queue
- Agent calls `ScenarioState.ApplyBeamDelete()`
- Agent calls `ScenarioState.RemoveRoute()` for both nodes
- Agent sends Response back to controller

### 7.4 Verification

At this point, verify:
- ✅ Link `link-sat1-gs1` has `Status = LinkStatusPotential` (or `LinkStatusUnknown`)
- ✅ Link `link-sat1-gs1` has `IsUp = false`
- ✅ Routes removed from both nodes
- ✅ Agent sent deletion Response with status OK

## Step 8: Observe Post-Deactivation Telemetry

### 8.1 Telemetry After Deactivation

Telemetry should reflect interface down state:

```
[INFO] Agent: Telemetry export starting agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Interface if-sat1-down Up=false BytesTx=4096 BytesRx=0
[INFO] Agent: Telemetry export completed agent_id=agent-sat1 interfaces=1

[INFO] Telemetry: ExportMetrics received node_id=sat1 interface_count=1
[INFO] Telemetry: Interface metrics updated node_id=sat1 interface_id=if-sat1-down Up=false BytesTx=4096
```

**What's happening:**
- Agent's telemetry loop continues running
- Agent derives interface state: `Up = false` (link is down)
- `BytesTx` stops increasing (remains at last value)
- TelemetryServer updates `TelemetryState`

### 8.2 Verification

Verify telemetry shows interface down:
- ✅ `Up = false` for both interfaces
- ✅ `BytesTx` remains stable (no further increase)
- ✅ TelemetryState reflects down state

## Step 9: Controller/Agent Behavior Summary

### 9.1 Controller-Side Behavior

**Scheduler:**
- Scans for potential links periodically
- Generates `UpdateBeam`/`DeleteBeam` actions based on link visibility
- Generates `SetRoute`/`DeleteRoute` actions for link endpoints
- Generates actions for ServiceRequests (if any)

**CDPI Server:**
- Manages agent connections (AgentHandle per agent)
- Maintains tokens and sequence numbers per agent
- Sends CreateEntry/DeleteEntry/Finalize messages
- Receives and processes agent Responses

**Telemetry Server:**
- Receives `ExportMetricsRequest` from agents
- Updates `TelemetryState` with latest metrics
- Exposes metrics via NBI `TelemetryService`

### 9.2 Agent-Side Behavior

**Agent Lifecycle:**
- Starts and connects to CDPI server
- Sends Hello message
- Receives and validates tokens
- Maintains local schedule queue
- Executes scheduled actions at correct times
- Exports telemetry periodically

**Action Execution:**
- Validates token and seqno for each message
- Schedules actions via `EventScheduler`
- Executes actions at scheduled time
- Applies changes to `ScenarioState`
- Sends Responses back to controller

**Telemetry Export:**
- Runs telemetry loop every interval
- Derives interface state from `ScenarioState`
- Calculates `BytesTx`/`BytesRx` based on bandwidth
- Sends `ExportMetricsRequest` to TelemetryServer

## Step 10: Validation Checklist

Use this checklist to verify SBI correctness:

### Initialization
- [ ] Agents start and connect to CDPI
- [ ] Agents send Hello messages
- [ ] Controller assigns tokens
- [ ] Scheduler initializes

### Link Activation (T₁)
- [ ] Scheduler detects link in view
- [ ] Scheduler generates UpdateBeam + SetRoute actions
- [ ] CDPI sends CreateEntryRequests
- [ ] Agents receive and schedule actions
- [ ] Agents execute actions at T₁
- [ ] Link Status becomes Active
- [ ] Link IsUp becomes true
- [ ] Routes installed on both nodes
- [ ] Agents send execution Responses

### Telemetry During Active Period
- [ ] Telemetry loop runs every interval
- [ ] Telemetry shows Up=true
- [ ] BytesTx increases monotonically
- [ ] TelemetryServer receives ExportMetricsRequest
- [ ] TelemetryState updated

### Link Deactivation (T₂)
- [ ] Scheduler detects link out of view
- [ ] Scheduler generates DeleteBeam + DeleteRoute actions
- [ ] CDPI sends DeleteEntryRequests
- [ ] Agents cancel scheduled actions
- [ ] Agents execute deactivation
- [ ] Link Status becomes Potential/Unknown
- [ ] Link IsUp becomes false
- [ ] Routes removed from both nodes
- [ ] Agents send deletion Responses

### Post-Deactivation
- [ ] Telemetry shows Up=false
- [ ] BytesTx stops increasing
- [ ] System remains stable

## Troubleshooting

### Agents Not Connecting

**Check:**
- Agent startup logs for errors
- gRPC server is running
- Network connectivity (if not in-process)

**Solution:**
- Verify nodes exist in ScenarioState
- Check agent configuration
- Review CDPI server logs

### No Scheduled Actions

**Check:**
- Scheduler logs for link detection
- Connectivity service evaluation
- Link status in ScenarioState

**Solution:**
- Verify links are marked as "potential"
- Check link geometry (range, LoS)
- Review scheduler configuration

### Telemetry Not Updating

**Check:**
- Agent telemetry loop logs
- Telemetry configuration (enabled, interval)
- TelemetryServer registration

**Solution:**
- Verify telemetry is enabled
- Check telemetry interval > 0
- Review TelemetryServer logs

## Next Steps

- **Behavior Documentation**: See `docs/sbi/example/behavior.md`
- **Timeline Documentation**: See `docs/sbi/example/timeline.md`
- **Sample Logs**: See `docs/sbi/example/sample_logs.md`
- **Running Guide**: See `docs/sbi/example/running.md`

