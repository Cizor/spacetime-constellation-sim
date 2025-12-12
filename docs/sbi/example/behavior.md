# Expected SBI Behavior for Example Scenario

This document describes the expected behavior of the SBI (Southbound Interface) system when running the minimal example scenario (one LEO satellite, one ground station, and a single potential link).

## Scenario Description

The example scenario consists of:
- **LEO Satellite** (`sat1`) with downlink interface (`if-sat1-down`)
- **Ground Station** (`gs1`) with uplink interface (`if-gs1-up`)
- **Potential Link** (`link-sat1-gs1`) between the two interfaces
- **ServiceRequest** (optional): from `gs1` to `sat1`

## Timeline Overview

```
T₀ (sim start)
  ↓
  Agent Hello messages sent
  Scheduler initializes
  ↓
T₁ (link becomes visible)
  ↓
  Scheduler: UpdateBeam + SetRoute
  Agent: Executes UpdateBeam/SetRoute
  ScenarioState: Link becomes Active
  Telemetry: Up=true, BytesTx starts increasing
  ↓
T₂ (link goes out-of-view)
  ↓
  Scheduler: DeleteBeam + DeleteRoute
  Agent: Executes DeleteBeam/DeleteRoute
  ScenarioState: Link becomes Inactive
  Telemetry: Up=false, BytesTx stops increasing
```

## Detailed Behavior

### 1. Scheduling Events

#### When Link Becomes Visible (T₁)

**Scheduler Actions:**
- Detects potential link `link-sat1-gs1` is now in view
- Generates `UpdateBeam` action for `sat1` agent:
  - EntryID: unique identifier
  - When: T₁ (or slightly before for scheduling)
  - BeamSpec: `if-sat1-down` → `if-gs1-up`
- Generates `SetRoute` actions for both endpoints:
  - For `sat1`: Route to `gs1` via `if-sat1-down`
  - For `gs1`: Route to `sat1` via `if-gs1-up`

**CDPI Messages:**
- Controller sends `CreateEntryRequest` to `sat1` agent:
  - Contains `UpdateBeam` action
  - Contains `SetRoute` action
  - Includes `schedule_manipulation_token` and `seqno`
- Controller sends `CreateEntryRequest` to `gs1` agent:
  - Contains `SetRoute` action
  - Includes `schedule_manipulation_token` and `seqno`

#### When Link Goes Out-of-View (T₂)

**Scheduler Actions:**
- Detects link `link-sat1-gs1` is no longer in view
- Generates `DeleteBeam` action for `sat1` agent
- Generates `DeleteRoute` actions for both endpoints

**CDPI Messages:**
- Controller sends `DeleteEntryRequest` to `sat1` agent:
  - Contains `DeleteBeam` action
  - Contains `DeleteRoute` action
- Controller sends `DeleteEntryRequest` to `gs1` agent:
  - Contains `DeleteRoute` action

### 2. CDPI Messaging Flow

#### Initial Handshake

1. **Agent → Controller: Hello**
   ```
   Agent (sat1) sends Hello message:
   - agent_id: "agent-sat1"
   - node_id: "sat1"
   ```

2. **Controller Response:**
   - Controller creates `AgentHandle` for `agent-sat1`
   - Controller generates and assigns `schedule_manipulation_token`
   - Controller initializes `seqno = 1` for this agent

#### Scheduling Messages

3. **Controller → Agent: CreateEntryRequest**
   ```
   CreateEntryRequest {
     entry_id: "entry-123"
     action: UpdateBeam {
       beam: { ... }
     }
     when: "2024-01-01T12:00:00Z"
     schedule_manipulation_token: "token-abc"
     seqno: 1
   }
   ```

4. **Agent Processing:**
   - Agent validates token (matches current token)
   - Agent validates seqno (monotonically increasing)
   - Agent stores action in local schedule queue
   - Agent schedules execution via `EventScheduler`

5. **Agent → Controller: Response**
   ```
   Response {
     request_id: "entry-123"
     status: OK
     message: "scheduled"
   }
   ```

#### Execution Flow

6. **At Scheduled Time:**
   - `EventScheduler` triggers action execution
   - Agent calls `execute()` method
   - Agent applies `UpdateBeam` to `ScenarioState`
   - Agent sends `Response` to controller:
     ```
     Response {
       request_id: "entry-123"
       status: OK
       message: "executed"
     }
     ```

### 3. ScenarioState Changes

#### Link Activation (T₁)

**Before T₁:**
- Link `link-sat1-gs1` has `Status = LinkStatusPotential`
- Link `IsUp = false`
- No routes installed

**After T₁ (UpdateBeam executed):**
- Link `Status = LinkStatusActive`
- Link `IsUp = true`
- Route installed on `sat1`:
  - Destination: `node:gs1/32`
  - NextHop: `gs1`
  - OutInterface: `if-sat1-down`
- Route installed on `gs1`:
  - Destination: `node:sat1/32`
  - NextHop: `sat1`
  - OutInterface: `if-gs1-up`

#### Link Deactivation (T₂)

**After T₂ (DeleteBeam executed):**
- Link `Status = LinkStatusPotential` (or `LinkStatusUnknown`)
- Link `IsUp = false`
- Routes removed from both nodes

### 4. Telemetry Behavior

#### Interface Metrics

**For `sat1/if-sat1-down`:**

- **Before T₁:**
  - `Up = false`
  - `BytesTx = 0`
  - `BytesRx = 0`

- **Between T₁ and T₂:**
  - `Up = true`
  - `BytesTx` increases monotonically (simulated traffic)
  - `BytesRx` may increase if receiving data
  - `SNRdB` reported (if available)
  - `Modulation` reported (if available)

- **After T₂:**
  - `Up = false`
  - `BytesTx` stops increasing
  - `BytesRx` stops increasing

**For `gs1/if-gs1-up`:**

- Similar behavior to `sat1/if-sat1-down`
- Metrics reflect the ground station interface state

#### Telemetry Export Flow

1. **Agent Telemetry Loop:**
   - Runs every `TelemetryConfig.Interval` (default: 1 second)
   - Collects metrics for all interfaces on the agent's node
   - Builds `ExportMetricsRequest`

2. **Agent → TelemetryServer: ExportMetricsRequest**
   ```
   ExportMetricsRequest {
     interface_metrics: [
       {
         interface_id: "if-sat1-down"
         operational_state_data_points: [
           { time: "T₁", value: UP }
         ]
         bytes_tx_data_points: [
           { time: "T₁", value: 0 },
           { time: "T₁+1s", value: 1024 },
           { time: "T₁+2s", value: 2048 }
         ]
       }
     ]
   }
   ```

3. **TelemetryServer Processing:**
   - Updates `TelemetryState` with latest metrics
   - Metrics available via NBI `TelemetryService.ListInterfaceMetrics`

### 5. Logging Expectations

#### Controller-Side Logs

```
[INFO] CDPI: Agent Hello received agent_id=agent-sat1 node_id=sat1
[INFO] CDPI: SendCreateEntry agent_id=agent-sat1 entry_id=entry-123 action=UpdateBeam
[INFO] Scheduler: ScheduleLinkBeams scheduled 1 beam actions
[INFO] CDPI: Response received agent_id=agent-sat1 request_id=entry-123 status=OK
[INFO] CDPI: SendDeleteEntry agent_id=agent-sat1 entry_id=entry-123 action=DeleteBeam
```

#### Agent-Side Logs

```
[INFO] Agent: Starting agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Hello sent to controller
[INFO] Agent: CreateEntry received entry_id=entry-123 action=UpdateBeam when=T₁
[INFO] Agent: Action scheduled entry_id=entry-123
[INFO] Agent: Executing action entry_id=entry-123 type=UpdateBeam
[INFO] Agent: UpdateBeam applied beam=if-sat1-down→if-gs1-up
[INFO] Agent: Response sent request_id=entry-123 status=OK
[INFO] Agent: Telemetry export node_id=sat1 interfaces=1
```

#### Telemetry Logs

```
[INFO] Telemetry: ExportMetrics received node_id=sat1 interface_count=1
[INFO] Telemetry: Interface if-sat1-down Up=true BytesTx=1024
```

## Diagrams

### Link In-View Timeline

```
Time →
     T₀          T₁          T₂          T₃
     │           │           │           │
Link │ Potential │  Active   │ Potential │
     └───────────┴────────────┴───────────┘
         ↓           ↓           ↓
      Unknown    UpdateBeam   DeleteBeam
                 SetRoute     DeleteRoute
```

### Agent Scheduling Pipeline

```
┌──────────┐
│ Scheduler│
│          │
│ Detects  │
│ potential│
│ link     │
└────┬─────┘
     │
     │ Generate ScheduledAction
     ↓
┌──────────┐
│  CDPI    │
│  Server  │
│          │
│ Create   │
│ Entry    │
│ Request  │
└────┬─────┘
     │
     │ gRPC stream
     ↓
┌──────────┐
│  Agent   │
│          │
│ Schedule │
│ locally  │
└────┬─────┘
     │
     │ EventScheduler triggers
     ↓
┌──────────┐
│ Execute  │
│ Action   │
└────┬─────┘
     │
     │ Apply to ScenarioState
     ↓
┌──────────┐
│Scenario  │
│ State    │
│          │
│ Link     │
│ Active   │
└──────────┘
```

## Validation Checklist

Use this checklist to manually verify SBI correctness:

### Initialization
- [ ] Agent sends Hello message to controller
- [ ] Controller creates AgentHandle and assigns token
- [ ] Agent receives and stores token

### Link Activation (T₁)
- [ ] Scheduler detects potential link is in view
- [ ] Scheduler emits UpdateBeam action before T₁
- [ ] CDPI sends CreateEntryRequest with UpdateBeam
- [ ] CDPI sends CreateEntryRequest with SetRoute (for both nodes)
- [ ] Agent receives and schedules actions
- [ ] Agent executes UpdateBeam at T₁
- [ ] ScenarioState: Link Status becomes Active
- [ ] ScenarioState: Link IsUp becomes true
- [ ] ScenarioState: Routes installed on both nodes
- [ ] Agent sends Response with status OK

### Telemetry During Active Period
- [ ] Telemetry loop runs every interval (default 1s)
- [ ] Telemetry shows Up=true for both interfaces
- [ ] BytesTx increases monotonically while link is active
- [ ] TelemetryServer receives ExportMetricsRequest
- [ ] TelemetryState updated with latest metrics

### Link Deactivation (T₂)
- [ ] Scheduler detects link is out of view
- [ ] Scheduler emits DeleteBeam action
- [ ] CDPI sends DeleteEntryRequest with DeleteBeam
- [ ] CDPI sends DeleteEntryRequest with DeleteRoute
- [ ] Agent cancels scheduled action (if not yet executed)
- [ ] Agent executes DeleteBeam at T₂
- [ ] ScenarioState: Link Status becomes Potential/Unknown
- [ ] ScenarioState: Link IsUp becomes false
- [ ] ScenarioState: Routes removed from both nodes
- [ ] Agent sends Response with status OK

### Telemetry After Deactivation
- [ ] Telemetry shows Up=false for both interfaces
- [ ] BytesTx stops increasing
- [ ] TelemetryState reflects interface down state

### Error Handling
- [ ] Invalid token rejected by agent
- [ ] Out-of-order seqno rejected by agent
- [ ] Missing agent returns error from CDPI
- [ ] Failed action execution returns error Response

## Notes

- **Timing**: Actions are scheduled slightly before the actual event time to account for processing delays
- **Idempotency**: Calling UpdateBeam multiple times with the same beam spec has the same effect as calling it once
- **Concurrency**: All state updates are thread-safe via mutexes
- **Telemetry Interval**: Default is 1 second, but can be configured via `TelemetryConfig`

