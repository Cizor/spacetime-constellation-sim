# SBI Example Scenario Timeline

This document provides a chronological, event-based description of expected SBI behavior during the example scenario run.

## Timeline Overview

```
T₀ (Scenario Load)
  ├─ Agents start and connect
  ├─ CDPI handshake (Hello, token assignment)
  └─ Scheduler initializes

T₁ (Link Comes Into View)
  ├─ Scheduler detects potential link
  ├─ Scheduler emits UpdateBeam + SetRoute
  ├─ CDPI sends CreateEntryRequests
  ├─ Agent schedules actions locally
  ├─ Agent executes at T₁
  ├─ ScenarioState: Link becomes Active
  └─ Telemetry: Up=true, BytesTx starts

T₁+Δ (Link Active Period)
  ├─ Telemetry loop runs periodically
  ├─ BytesTx increases monotonically
  └─ TelemetryState updated

T₂ (Link Goes Out of View)
  ├─ Scheduler detects link out of view
  ├─ Scheduler emits DeleteBeam + DeleteRoute
  ├─ CDPI sends DeleteEntryRequests
  ├─ Agent executes deactivation
  ├─ ScenarioState: Link becomes Inactive
  └─ Telemetry: Up=false, BytesTx stops

T₂+Δ (After Deactivation)
  └─ System stable, no further activity
```

## Detailed Timeline

### T₀: Scenario Load and Initialization

**Time:** Simulation start (t=0)

**Events:**

1. **Agents Start**
   - Agent for `sat1` starts
   - Agent for `gs1` starts
   - Each agent establishes gRPC stream to CDPI server

2. **CDPI Handshake**
   ```
   Agent (sat1) → Controller: Hello {
     agent_id: "agent-sat1"
     node_id: "sat1"
   }
   
   Controller → Agent: (implicit acknowledgment)
   Controller assigns: schedule_manipulation_token = "token-abc-123"
   Controller initializes: seqno = 1
   ```

3. **Scheduler Initialization**
   - Scheduler calls `RunInitialSchedule()`
   - Scheduler scans for potential links
   - At T₀, link `link-sat1-gs1` is not yet in view
   - No actions scheduled yet

**Expected State:**
- Link `link-sat1-gs1`: `Status = LinkStatusPotential`, `IsUp = false`
- No routes installed
- Telemetry: `Up = false`, `BytesTx = 0` for both interfaces

**Expected Logs:**
```
[INFO] Agent: Starting agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Hello sent to controller
[INFO] CDPI: Agent Hello received agent_id=agent-sat1 node_id=sat1
[INFO] Scheduler: RunInitialSchedule completed links=0
```

---

### T₁: Link Comes Into View

**Time:** When satellite enters visibility window (e.g., t=60s)

**Events:**

1. **Scheduler Detection**
   - Connectivity service evaluates link `link-sat1-gs1`
   - Link geometry allows connection (in range, LoS clear)
   - Scheduler's `ScheduleLinkBeams()` detects potential link

2. **Scheduler Actions**
   ```
   Scheduler generates:
   - UpdateBeam for sat1:
     * EntryID: "entry-beam-001"
     * When: T₁ (or T₁ - small buffer)
     * BeamSpec: if-sat1-down → if-gs1-up
   
   - SetRoute for sat1:
     * EntryID: "entry-route-sat1-001"
     * When: T₁
     * Route: node:gs1/32 via if-sat1-down
   
   - SetRoute for gs1:
     * EntryID: "entry-route-gs1-001"
     * When: T₁
     * Route: node:sat1/32 via if-gs1-up
   ```

3. **CDPI Messages Sent**
   ```
   Controller → Agent (sat1): CreateEntryRequest {
     entry_id: "entry-beam-001"
     action: UpdateBeam { ... }
     when: "T₁"
     schedule_manipulation_token: "token-abc-123"
     seqno: 1
   }
   
   Controller → Agent (sat1): CreateEntryRequest {
     entry_id: "entry-route-sat1-001"
     action: SetRoute { ... }
     when: "T₁"
     schedule_manipulation_token: "token-abc-123"
     seqno: 2
   }
   
   Controller → Agent (gs1): CreateEntryRequest {
     entry_id: "entry-route-gs1-001"
     action: SetRoute { ... }
     when: "T₁"
     schedule_manipulation_token: "token-xyz-456"
     seqno: 1
   }
   ```

4. **Agent Processing**
   - Agent validates token (matches current token)
   - Agent validates seqno (monotonically increasing)
   - Agent stores actions in local schedule queue
   - Agent schedules execution via `EventScheduler.Schedule()`

5. **Agent Responses**
   ```
   Agent (sat1) → Controller: Response {
     request_id: "entry-beam-001"
     status: OK
     message: "scheduled"
   }
   
   Agent (sat1) → Controller: Response {
     request_id: "entry-route-sat1-001"
     status: OK
     message: "scheduled"
   }
   ```

6. **At Exactly T₁: Agent Execution**
   - `EventScheduler` triggers action execution
   - Agent calls `execute()` for UpdateBeam
   - Agent calls `ScenarioState.ApplyBeamUpdate()`
   - Agent calls `ScenarioState.InstallRoute()` for both nodes

7. **ScenarioState Updates**
   - Link `link-sat1-gs1`:
     * `Status = LinkStatusActive`
     * `IsUp = true`
   - Route on `sat1`:
     * Destination: `node:gs1/32`
     * NextHop: `gs1`
     * OutInterface: `if-sat1-down`
   - Route on `gs1`:
     * Destination: `node:sat1/32`
     * NextHop: `sat1`
     * OutInterface: `if-gs1-up`

8. **Execution Responses**
   ```
   Agent (sat1) → Controller: Response {
     request_id: "entry-beam-001"
     status: OK
     message: "executed"
   }
   ```

**Expected State:**
- Link `link-sat1-gs1`: `Status = LinkStatusActive`, `IsUp = true`
- Routes installed on both nodes
- Telemetry: `Up = true` (will be reported in next telemetry interval)

**Expected Logs:**
```
[INFO] Scheduler: ScheduleLinkBeams scheduled 1 beam actions
[INFO] CDPI: SendCreateEntry agent_id=agent-sat1 entry_id=entry-beam-001 action=UpdateBeam
[INFO] Agent: CreateEntry received entry_id=entry-beam-001 action=UpdateBeam when=T₁
[INFO] Agent: Action scheduled entry_id=entry-beam-001
[INFO] Agent: Executing action entry_id=entry-beam-001 type=UpdateBeam
[INFO] Agent: UpdateBeam applied beam=if-sat1-down→if-gs1-up
[INFO] Agent: Response sent request_id=entry-beam-001 status=OK
[INFO] CDPI: Response received agent_id=agent-sat1 request_id=entry-beam-001 status=OK
```

---

### T₁+Δ: Link Active Period

**Time:** Between T₁ and T₂ (e.g., t=60s to t=180s)

**Events:**

1. **Telemetry Loop (Every Interval)**
   - Agent telemetry loop runs every `TelemetryConfig.Interval` (default: 1s)
   - Agent collects metrics for all interfaces on its node
   - Agent builds `ExportMetricsRequest`

2. **Telemetry Export**
   ```
   Agent (sat1) → TelemetryServer: ExportMetricsRequest {
     interface_metrics: [
       {
         interface_id: "if-sat1-down"
         operational_state_data_points: [
           { time: "T₁+1s", value: UP },
           { time: "T₁+2s", value: UP },
           ...
         ]
         bytes_tx_data_points: [
           { time: "T₁+1s", value: 1024 },
           { time: "T₁+2s", value: 2048 },
           { time: "T₁+3s", value: 3072 },
           ...
         ]
       }
     ]
   }
   ```

3. **TelemetryState Updates**
   - TelemetryServer receives `ExportMetricsRequest`
   - TelemetryServer updates `TelemetryState`
   - Metrics available via NBI `TelemetryService.ListInterfaceMetrics`

**Expected State:**
- Link remains `Status = LinkStatusActive`, `IsUp = true`
- Routes remain installed
- Telemetry: `Up = true`, `BytesTx` increases monotonically

**Expected Logs:**
```
[INFO] Agent: Telemetry export node_id=sat1 interfaces=1
[INFO] Telemetry: ExportMetrics received node_id=sat1 interface_count=1
[INFO] Telemetry: Interface if-sat1-down Up=true BytesTx=1024
[INFO] Agent: Telemetry export node_id=sat1 interfaces=1
[INFO] Telemetry: Interface if-sat1-down Up=true BytesTx=2048
...
```

---

### T₂: Link Goes Out of View

**Time:** When satellite exits visibility window (e.g., t=180s)

**Events:**

1. **Scheduler Detection**
   - Connectivity service evaluates link `link-sat1-gs1`
   - Link geometry no longer allows connection (out of range or LoS blocked)
   - Scheduler's `ScheduleLinkBeams()` detects link is out of view

2. **Scheduler Actions**
   ```
   Scheduler generates:
   - DeleteBeam for sat1:
     * EntryID: "entry-beam-002"
     * When: T₂
     * BeamSpec: if-sat1-down → if-gs1-up
   
   - DeleteRoute for sat1:
     * EntryID: "entry-route-sat1-002"
     * When: T₂
     * Route: node:gs1/32
   
   - DeleteRoute for gs1:
     * EntryID: "entry-route-gs1-002"
     * When: T₂
     * Route: node:sat1/32
   ```

3. **CDPI Messages Sent**
   ```
   Controller → Agent (sat1): DeleteEntryRequest {
     entry_id: "entry-beam-001"  // original entry ID
     schedule_manipulation_token: "token-abc-123"
     seqno: 3
   }
   
   Controller → Agent (sat1): DeleteEntryRequest {
     entry_id: "entry-route-sat1-001"
     schedule_manipulation_token: "token-abc-123"
     seqno: 4
   }
   
   Controller → Agent (gs1): DeleteEntryRequest {
     entry_id: "entry-route-gs1-001"
     schedule_manipulation_token: "token-xyz-456"
     seqno: 2
   }
   ```

4. **Agent Processing**
   - Agent validates token
   - Agent cancels scheduled action (if not yet executed)
   - Agent removes action from local schedule queue
   - Agent calls `ScenarioState.ApplyBeamDelete()`
   - Agent calls `ScenarioState.RemoveRoute()` for both nodes

5. **Agent Responses**
   ```
   Agent (sat1) → Controller: Response {
     request_id: "entry-beam-001"
     status: OK
     message: "deleted"
   }
   ```

6. **ScenarioState Updates**
   - Link `link-sat1-gs1`:
     * `Status = LinkStatusPotential` (or `LinkStatusUnknown`)
     * `IsUp = false`
   - Routes removed from both nodes

**Expected State:**
- Link `link-sat1-gs1`: `Status = LinkStatusPotential`, `IsUp = false`
- No routes installed
- Telemetry: `Up = false` (will be reported in next telemetry interval)

**Expected Logs:**
```
[INFO] Scheduler: ScheduleLinkBeams scheduled 1 delete actions
[INFO] CDPI: SendDeleteEntry agent_id=agent-sat1 entry_id=entry-beam-001 action=DeleteBeam
[INFO] Agent: DeleteEntry received entry_id=entry-beam-001
[INFO] Agent: Action cancelled entry_id=entry-beam-001
[INFO] Agent: DeleteBeam applied beam=if-sat1-down→if-gs1-up
[INFO] Agent: Response sent request_id=entry-beam-001 status=OK
[INFO] CDPI: Response received agent_id=agent-sat1 request_id=entry-beam-001 status=OK
```

---

### T₂+Δ: After Deactivation

**Time:** After T₂ (e.g., t > 180s)

**Events:**

1. **Telemetry Loop Continues**
   - Agent telemetry loop continues running
   - Telemetry reports `Up = false`
   - `BytesTx` stops increasing (remains at last value)

2. **System Stable**
   - No further scheduler actions
   - No further CDPI messages
   - Link remains inactive

**Expected State:**
- Link `link-sat1-gs1`: `Status = LinkStatusPotential`, `IsUp = false`
- No routes installed
- Telemetry: `Up = false`, `BytesTx` stable

**Expected Logs:**
```
[INFO] Agent: Telemetry export node_id=sat1 interfaces=1
[INFO] Telemetry: Interface if-sat1-down Up=false BytesTx=3072
[INFO] Agent: Telemetry export node_id=sat1 interfaces=1
[INFO] Telemetry: Interface if-sat1-down Up=false BytesTx=3072
...
```

## Message Flow Summary

### CDPI Messages (Controller → Agent)

| Time | Message Type | EntryID | Action | Agent |
|------|-------------|---------|--------|-------|
| T₀ | (Hello) | - | Hello | sat1, gs1 |
| T₁ | CreateEntry | entry-beam-001 | UpdateBeam | sat1 |
| T₁ | CreateEntry | entry-route-sat1-001 | SetRoute | sat1 |
| T₁ | CreateEntry | entry-route-gs1-001 | SetRoute | gs1 |
| T₂ | DeleteEntry | entry-beam-001 | DeleteBeam | sat1 |
| T₂ | DeleteEntry | entry-route-sat1-001 | DeleteRoute | sat1 |
| T₂ | DeleteEntry | entry-route-gs1-001 | DeleteRoute | gs1 |

### Agent Responses (Agent → Controller)

| Time | RequestID | Status | Message |
|------|-----------|--------|---------|
| T₁ | entry-beam-001 | OK | scheduled |
| T₁ | entry-beam-001 | OK | executed |
| T₁ | entry-route-sat1-001 | OK | scheduled |
| T₁ | entry-route-sat1-001 | OK | executed |
| T₁ | entry-route-gs1-001 | OK | scheduled |
| T₁ | entry-route-gs1-001 | OK | executed |
| T₂ | entry-beam-001 | OK | deleted |
| T₂ | entry-route-sat1-001 | OK | deleted |
| T₂ | entry-route-gs1-001 | OK | deleted |

### ScenarioState Transitions

| Time | Link Status | Link IsUp | Routes Installed |
|------|-------------|-----------|------------------|
| T₀ | Potential | false | No |
| T₁ | Active | true | Yes (both nodes) |
| T₁+Δ | Active | true | Yes (both nodes) |
| T₂ | Potential | false | No |
| T₂+Δ | Potential | false | No |

### Telemetry Evolution

| Time | Interface | Up | BytesTx | BytesRx |
|------|-----------|----|---------|---------| 
| T₀ | if-sat1-down | false | 0 | 0 |
| T₀ | if-gs1-up | false | 0 | 0 |
| T₁+1s | if-sat1-down | true | 1024 | 0 |
| T₁+1s | if-gs1-up | true | 1024 | 0 |
| T₁+2s | if-sat1-down | true | 2048 | 0 |
| T₁+2s | if-gs1-up | true | 2048 | 0 |
| ... | ... | true | increasing | ... |
| T₂+1s | if-sat1-down | false | 3072 | 0 |
| T₂+1s | if-gs1-up | false | 3072 | 0 |
| T₂+2s | if-sat1-down | false | 3072 | 0 |
| T₂+2s | if-gs1-up | false | 3072 | 0 |

## Verification Checklist

Use this checklist to verify SBI correctness:

### T₀ Verification
- [ ] Agents send Hello messages
- [ ] Controller assigns tokens
- [ ] Scheduler initializes without errors
- [ ] Link status is Potential, IsUp is false

### T₁ Verification
- [ ] Scheduler emits UpdateBeam action
- [ ] Scheduler emits SetRoute actions (for both nodes)
- [ ] CDPI sends CreateEntryRequests with correct tokens and seqnos
- [ ] Agent receives and schedules actions
- [ ] Agent executes UpdateBeam at T₁
- [ ] Link Status becomes Active
- [ ] Link IsUp becomes true
- [ ] Routes installed on both nodes
- [ ] Agent sends Response with status OK

### T₁+Δ Verification
- [ ] Telemetry loop runs every interval
- [ ] Telemetry shows Up=true for both interfaces
- [ ] BytesTx increases monotonically
- [ ] TelemetryServer receives ExportMetricsRequest
- [ ] TelemetryState updated with latest metrics

### T₂ Verification
- [ ] Scheduler emits DeleteBeam action
- [ ] Scheduler emits DeleteRoute actions
- [ ] CDPI sends DeleteEntryRequests
- [ ] Agent cancels scheduled actions (if applicable)
- [ ] Agent executes DeleteBeam at T₂
- [ ] Link Status becomes Potential/Unknown
- [ ] Link IsUp becomes false
- [ ] Routes removed from both nodes
- [ ] Agent sends Response with status OK

### T₂+Δ Verification
- [ ] Telemetry shows Up=false for both interfaces
- [ ] BytesTx stops increasing (remains stable)
- [ ] No further scheduler actions
- [ ] System remains stable

## ServiceRequest Behavior (Optional)

If a ServiceRequest is added (e.g., from `gs1` to `sat1`):

1. **At T₁:**
   - Scheduler's `ScheduleServiceRequests()` detects active ServiceRequest
   - Scheduler builds connectivity graph
   - Scheduler finds path: `gs1` → `sat1` (single hop via `link-sat1-gs1`)
   - Scheduler emits UpdateBeam + SetRoute along the path
   - Same CDPI/Agent flow as link-driven scheduling

2. **Between T₁ and T₂:**
   - ServiceRequest remains active
   - Path remains valid
   - Telemetry reflects active link

3. **At T₂:**
   - Link goes out of view
   - Path becomes invalid
   - Scheduler may emit DeleteBeam/DeleteRoute (same as link-driven)

## References

- **Chunk 4**: Agent execution semantics (`internal/sbi/agent/agent.go`)
- **Chunk 5**: CDPI message handling (`internal/sbi/controller/cdpi_server.go`)
- **Chunk 6**: Telemetry behavior (`internal/sbi/controller/telemetry_server.go`)
- **Chunk 8**: Scheduling algorithm (`internal/sbi/controller/scheduler.go`)
- **Chunk 9**: Agent lifecycle and wiring (`internal/sbi/runtime/runtime.go`)

