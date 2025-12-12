# Sample SBI Logs

This document provides sample log output from a typical run of the SBI example scenario, demonstrating the expected logging patterns for scheduling, agent execution, and telemetry.

## Log Format

All logs use structured logging with key-value pairs. The format shown here is simplified for readability.

## Startup Logs

```
[INFO] Loading network scenario from configs/scope4_sbi_example.json
[INFO] Loaded network scenario: 2 interfaces, 1 links, 2 nodes with positions
[INFO] Starting NBI gRPC server addr=0.0.0.0:50051
[INFO] SBI: Creating agents for 2 nodes
[INFO] SBI: Starting agents
[INFO] Agent: Starting agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Starting agent_id=agent-gs1 node_id=gs1
```

## Agent Connection Logs

```
[INFO] Agent: Hello sent to controller agent_id=agent-sat1 node_id=sat1
[INFO] CDPI: Agent Hello received agent_id=agent-sat1 node_id=sat1
[INFO] CDPI: Agent Hello received agent_id=agent-gs1 node_id=gs1
[INFO] Agent: Hello sent to controller agent_id=agent-gs1 node_id=gs1
```

## Initial Scheduling Logs

```
[INFO] Scheduler: RunInitialSchedule starting
[INFO] Scheduler: ScheduleLinkBeams scanning for potential links
[INFO] Scheduler: ScheduleLinkBeams found 1 potential links
[INFO] Scheduler: ScheduleLinkBeams scheduled 1 beam actions
[INFO] Scheduler: ScheduleLinkRoutes scheduled 2 route actions
[INFO] Scheduler: RunInitialSchedule completed links=1 routes=2
```

## Link Activation Logs (T₁)

### Controller-Side (Scheduler + CDPI)

```
[INFO] Scheduler: ScheduleLinkBeams detected link link-sat1-gs1 is in view
[INFO] Scheduler: Generating UpdateBeam action for link link-sat1-gs1
[INFO] Scheduler: Generating SetRoute actions for link link-sat1-gs1
[INFO] CDPI: SendCreateEntry agent_id=agent-sat1 entry_id=entry-beam-001 action=UpdateBeam
[INFO] CDPI: SendCreateEntry agent_id=agent-sat1 entry_id=entry-route-sat1-001 action=SetRoute
[INFO] CDPI: SendCreateEntry agent_id=agent-gs1 entry_id=entry-route-gs1-001 action=SetRoute
[INFO] CDPI: Message sent agent_id=agent-sat1 seqno=1
[INFO] CDPI: Message sent agent_id=agent-sat1 seqno=2
[INFO] CDPI: Message sent agent_id=agent-gs1 seqno=1
```

### Agent-Side (Receiving and Scheduling)

```
[INFO] Agent: CreateEntry received agent_id=agent-sat1 entry_id=entry-beam-001 action=UpdateBeam when=2024-01-01T12:00:00Z
[INFO] Agent: Token validated agent_id=agent-sat1 token=token-abc-123
[INFO] Agent: SeqNo validated agent_id=agent-sat1 seqno=1
[INFO] Agent: Action scheduled agent_id=agent-sat1 entry_id=entry-beam-001 event_id=evt-001
[INFO] Agent: Response sent agent_id=agent-sat1 request_id=entry-beam-001 status=OK message=scheduled

[INFO] Agent: CreateEntry received agent_id=agent-sat1 entry_id=entry-route-sat1-001 action=SetRoute when=2024-01-01T12:00:00Z
[INFO] Agent: SeqNo validated agent_id=agent-sat1 seqno=2
[INFO] Agent: Action scheduled agent_id=agent-sat1 entry_id=entry-route-sat1-001 event_id=evt-002
[INFO] Agent: Response sent agent_id=agent-sat1 request_id=entry-route-sat1-001 status=OK message=scheduled

[INFO] Agent: CreateEntry received agent_id=agent-gs1 entry_id=entry-route-gs1-001 action=SetRoute when=2024-01-01T12:00:00Z
[INFO] Agent: Token validated agent_id=agent-gs1 token=token-xyz-456
[INFO] Agent: SeqNo validated agent_id=agent-gs1 seqno=1
[INFO] Agent: Action scheduled agent_id=agent-gs1 entry_id=entry-route-gs1-001 event_id=evt-003
[INFO] Agent: Response sent agent_id=agent-gs1 request_id=entry-route-gs1-001 status=OK message=scheduled
```

### Controller-Side (Receiving Responses)

```
[INFO] CDPI: Response received agent_id=agent-sat1 request_id=entry-beam-001 status=OK message=scheduled
[INFO] CDPI: Response received agent_id=agent-sat1 request_id=entry-route-sat1-001 status=OK message=scheduled
[INFO] CDPI: Response received agent_id=agent-gs1 request_id=entry-route-gs1-001 status=OK message=scheduled
```

### Agent-Side (Execution)

```
[INFO] Agent: Executing action agent_id=agent-sat1 entry_id=entry-beam-001 type=UpdateBeam when=2024-01-01T12:00:00Z
[INFO] Agent: UpdateBeam applied agent_id=agent-sat1 beam=if-sat1-down→if-gs1-up
[INFO] Agent: Response sent agent_id=agent-sat1 request_id=entry-beam-001 status=OK message=executed

[INFO] Agent: Executing action agent_id=agent-sat1 entry_id=entry-route-sat1-001 type=SetRoute when=2024-01-01T12:00:00Z
[INFO] Agent: SetRoute applied agent_id=agent-sat1 route=node:gs1/32 via if-sat1-down
[INFO] Agent: Response sent agent_id=agent-sat1 request_id=entry-route-sat1-001 status=OK message=executed

[INFO] Agent: Executing action agent_id=agent-gs1 entry_id=entry-route-gs1-001 type=SetRoute when=2024-01-01T12:00:00Z
[INFO] Agent: SetRoute applied agent_id=agent-gs1 route=node:sat1/32 via if-gs1-up
[INFO] Agent: Response sent agent_id=agent-gs1 request_id=entry-route-gs1-001 status=OK message=executed
```

### Controller-Side (Execution Responses)

```
[INFO] CDPI: Response received agent_id=agent-sat1 request_id=entry-beam-001 status=OK message=executed
[INFO] CDPI: Response received agent_id=agent-sat1 request_id=entry-route-sat1-001 status=OK message=executed
[INFO] CDPI: Response received agent_id=agent-gs1 request_id=entry-route-gs1-001 status=OK message=executed
```

## Telemetry Logs (During Active Period)

### Agent-Side (Telemetry Export)

```
[INFO] Agent: Telemetry export starting agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Building interface metrics agent_id=agent-sat1 interface_count=1
[INFO] Agent: Interface if-sat1-down Up=true BytesTx=0 BytesRx=0
[INFO] Agent: Telemetry export completed agent_id=agent-sat1 interfaces=1

[INFO] Agent: Telemetry export starting agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Interface if-sat1-down Up=true BytesTx=1024 BytesRx=0
[INFO] Agent: Telemetry export completed agent_id=agent-sat1 interfaces=1

[INFO] Agent: Telemetry export starting agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Interface if-sat1-down Up=true BytesTx=2048 BytesRx=0
[INFO] Agent: Telemetry export completed agent_id=agent-sat1 interfaces=1

[INFO] Agent: Telemetry export starting agent_id=agent-gs1 node_id=gs1
[INFO] Agent: Interface if-gs1-up Up=true BytesTx=1024 BytesRx=0
[INFO] Agent: Telemetry export completed agent_id=agent-gs1 interfaces=1
```

### TelemetryServer-Side (Receiving Metrics)

```
[INFO] Telemetry: ExportMetrics received node_id=sat1 interface_count=1
[INFO] Telemetry: Processing interface node_id=sat1 interface_id=if-sat1-down
[INFO] Telemetry: Interface metrics updated node_id=sat1 interface_id=if-sat1-down Up=true BytesTx=0
[INFO] Telemetry: ExportMetrics completed node_id=sat1 processed=1

[INFO] Telemetry: ExportMetrics received node_id=sat1 interface_count=1
[INFO] Telemetry: Interface metrics updated node_id=sat1 interface_id=if-sat1-down Up=true BytesTx=1024
[INFO] Telemetry: ExportMetrics completed node_id=sat1 processed=1

[INFO] Telemetry: ExportMetrics received node_id=sat1 interface_count=1
[INFO] Telemetry: Interface metrics updated node_id=sat1 interface_id=if-sat1-down Up=true BytesTx=2048
[INFO] Telemetry: ExportMetrics completed node_id=sat1 processed=1

[INFO] Telemetry: ExportMetrics received node_id=gs1 interface_count=1
[INFO] Telemetry: Interface metrics updated node_id=gs1 interface_id=if-gs1-up Up=true BytesTx=1024
[INFO] Telemetry: ExportMetrics completed node_id=gs1 processed=1
```

## Link Deactivation Logs (T₂)

### Controller-Side (Scheduler + CDPI)

```
[INFO] Scheduler: ScheduleLinkBeams detected link link-sat1-gs1 is out of view
[INFO] Scheduler: Generating DeleteBeam action for link link-sat1-gs1
[INFO] Scheduler: Generating DeleteRoute actions for link link-sat1-gs1
[INFO] CDPI: SendDeleteEntry agent_id=agent-sat1 entry_id=entry-beam-001 action=DeleteBeam
[INFO] CDPI: SendDeleteEntry agent_id=agent-sat1 entry_id=entry-route-sat1-001 action=DeleteRoute
[INFO] CDPI: SendDeleteEntry agent_id=agent-gs1 entry_id=entry-route-gs1-001 action=DeleteRoute
[INFO] CDPI: Message sent agent_id=agent-sat1 seqno=3
[INFO] CDPI: Message sent agent_id=agent-sat1 seqno=4
[INFO] CDPI: Message sent agent_id=agent-gs1 seqno=2
```

### Agent-Side (Receiving and Processing)

```
[INFO] Agent: DeleteEntry received agent_id=agent-sat1 entry_id=entry-beam-001
[INFO] Agent: Token validated agent_id=agent-sat1 token=token-abc-123
[INFO] Agent: SeqNo validated agent_id=agent-sat1 seqno=3
[INFO] Agent: Action cancelled agent_id=agent-sat1 entry_id=entry-beam-001 event_id=evt-001
[INFO] Agent: DeleteBeam applied agent_id=agent-sat1 beam=if-sat1-down→if-gs1-up
[INFO] Agent: Response sent agent_id=agent-sat1 request_id=entry-beam-001 status=OK message=deleted

[INFO] Agent: DeleteEntry received agent_id=agent-sat1 entry_id=entry-route-sat1-001
[INFO] Agent: SeqNo validated agent_id=agent-sat1 seqno=4
[INFO] Agent: Action cancelled agent_id=agent-sat1 entry_id=entry-route-sat1-001 event_id=evt-002
[INFO] Agent: DeleteRoute applied agent_id=agent-sat1 route=node:gs1/32
[INFO] Agent: Response sent agent_id=agent-sat1 request_id=entry-route-sat1-001 status=OK message=deleted

[INFO] Agent: DeleteEntry received agent_id=agent-gs1 entry_id=entry-route-gs1-001
[INFO] Agent: Token validated agent_id=agent-gs1 token=token-xyz-456
[INFO] Agent: SeqNo validated agent_id=agent-gs1 seqno=2
[INFO] Agent: Action cancelled agent_id=agent-gs1 entry_id=entry-route-gs1-001 event_id=evt-003
[INFO] Agent: DeleteRoute applied agent_id=agent-gs1 route=node:sat1/32
[INFO] Agent: Response sent agent_id=agent-gs1 request_id=entry-route-gs1-001 status=OK message=deleted
```

### Controller-Side (Receiving Responses)

```
[INFO] CDPI: Response received agent_id=agent-sat1 request_id=entry-beam-001 status=OK message=deleted
[INFO] CDPI: Response received agent_id=agent-sat1 request_id=entry-route-sat1-001 status=OK message=deleted
[INFO] CDPI: Response received agent_id=agent-gs1 request_id=entry-route-gs1-001 status=OK message=deleted
```

## Telemetry Logs (After Deactivation)

```
[INFO] Agent: Telemetry export starting agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Interface if-sat1-down Up=false BytesTx=3072 BytesRx=0
[INFO] Agent: Telemetry export completed agent_id=agent-sat1 interfaces=1

[INFO] Telemetry: ExportMetrics received node_id=sat1 interface_count=1
[INFO] Telemetry: Interface metrics updated node_id=sat1 interface_id=if-sat1-down Up=false BytesTx=3072
[INFO] Telemetry: ExportMetrics completed node_id=sat1 processed=1

[INFO] Agent: Telemetry export starting agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Interface if-sat1-down Up=false BytesTx=3072 BytesRx=0
[INFO] Agent: Telemetry export completed agent_id=agent-sat1 interfaces=1
```

## Error Logs

### Invalid Token

```
[WARN] Agent: Token mismatch agent_id=agent-sat1 expected=token-abc-123 received=token-wrong
[WARN] Agent: Rejecting message agent_id=agent-sat1 entry_id=entry-beam-001 reason=invalid_token
[INFO] Agent: Response sent agent_id=agent-sat1 request_id=entry-beam-001 status=ERROR message=invalid_token
```

### Out-of-Order SeqNo

```
[WARN] Agent: SeqNo out of order agent_id=agent-sat1 expected=3 received=1
[WARN] Agent: Rejecting message agent_id=agent-sat1 entry_id=entry-beam-002 reason=invalid_seqno
[INFO] Agent: Response sent agent_id=agent-sat1 request_id=entry-beam-002 status=ERROR message=invalid_seqno
```

### Missing Agent

```
[ERROR] CDPI: Agent not found agent_id=agent-missing
[ERROR] CDPI: SendCreateEntry failed agent_id=agent-missing error=agent not found
```

### Execution Failure

```
[ERROR] Agent: Execution failed agent_id=agent-sat1 entry_id=entry-beam-001 error=link not found
[INFO] Agent: Response sent agent_id=agent-sat1 request_id=entry-beam-001 status=ERROR message=execution_failed
```

## Metrics Logs

### SBI Metrics (if exposed)

```
[INFO] SBI Metrics: CreateSent=3 DeleteSent=3 FinalizeSent=0 ResetRecv=0 ActionsExecuted=3 RespOK=6 RespErr=0 ExportCalls=10 IfaceSamples=10
```

## Complete Log Transcript Example

Here's a complete log transcript for a short simulation run:

```
[INFO] Loading network scenario from configs/scope4_sbi_example.json
[INFO] Loaded network scenario: 2 interfaces, 1 links, 2 nodes with positions
[INFO] Starting NBI gRPC server addr=0.0.0.0:50051
[INFO] SBI: Creating agents for 2 nodes
[INFO] SBI: Starting agents
[INFO] Agent: Starting agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Starting agent_id=agent-gs1 node_id=gs1
[INFO] Agent: Hello sent to controller agent_id=agent-sat1 node_id=sat1
[INFO] CDPI: Agent Hello received agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Hello sent to controller agent_id=agent-gs1 node_id=gs1
[INFO] CDPI: Agent Hello received agent_id=agent-gs1 node_id=gs1
[INFO] Scheduler: RunInitialSchedule starting
[INFO] Scheduler: ScheduleLinkBeams found 1 potential links
[INFO] Scheduler: ScheduleLinkBeams scheduled 1 beam actions
[INFO] CDPI: SendCreateEntry agent_id=agent-sat1 entry_id=entry-beam-001 action=UpdateBeam
[INFO] Agent: CreateEntry received agent_id=agent-sat1 entry_id=entry-beam-001 action=UpdateBeam when=2024-01-01T12:00:00Z
[INFO] Agent: Action scheduled agent_id=agent-sat1 entry_id=entry-beam-001
[INFO] Agent: Response sent agent_id=agent-sat1 request_id=entry-beam-001 status=OK message=scheduled
[INFO] CDPI: Response received agent_id=agent-sat1 request_id=entry-beam-001 status=OK
[INFO] Agent: Executing action agent_id=agent-sat1 entry_id=entry-beam-001 type=UpdateBeam when=2024-01-01T12:00:00Z
[INFO] Agent: UpdateBeam applied agent_id=agent-sat1 beam=if-sat1-down→if-gs1-up
[INFO] Agent: Response sent agent_id=agent-sat1 request_id=entry-beam-001 status=OK message=executed
[INFO] CDPI: Response received agent_id=agent-sat1 request_id=entry-beam-001 status=OK
[INFO] Agent: Telemetry export starting agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Interface if-sat1-down Up=true BytesTx=1024 BytesRx=0
[INFO] Telemetry: ExportMetrics received node_id=sat1 interface_count=1
[INFO] Telemetry: Interface metrics updated node_id=sat1 interface_id=if-sat1-down Up=true BytesTx=1024
[INFO] Agent: Telemetry export starting agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Interface if-sat1-down Up=true BytesTx=2048 BytesRx=0
[INFO] Telemetry: ExportMetrics received node_id=sat1 interface_count=1
[INFO] Telemetry: Interface metrics updated node_id=sat1 interface_id=if-sat1-down Up=true BytesTx=2048
[INFO] Scheduler: ScheduleLinkBeams detected link link-sat1-gs1 is out of view
[INFO] CDPI: SendDeleteEntry agent_id=agent-sat1 entry_id=entry-beam-001 action=DeleteBeam
[INFO] Agent: DeleteEntry received agent_id=agent-sat1 entry_id=entry-beam-001
[INFO] Agent: DeleteBeam applied agent_id=agent-sat1 beam=if-sat1-down→if-gs1-up
[INFO] Agent: Response sent agent_id=agent-sat1 request_id=entry-beam-001 status=OK message=deleted
[INFO] CDPI: Response received agent_id=agent-sat1 request_id=entry-beam-001 status=OK
[INFO] Agent: Telemetry export starting agent_id=agent-sat1 node_id=sat1
[INFO] Agent: Interface if-sat1-down Up=false BytesTx=3072 BytesRx=0
[INFO] Telemetry: ExportMetrics received node_id=sat1 interface_count=1
[INFO] Telemetry: Interface metrics updated node_id=sat1 interface_id=if-sat1-down Up=false BytesTx=3072
```

## Notes

- **Timestamps**: Actual logs include timestamps; shown here without for clarity
- **Structured Fields**: Real logs include structured key-value pairs (e.g., `agent_id`, `node_id`, `entry_id`)
- **Log Levels**: Most operational logs are `INFO`; errors are `ERROR` or `WARN`
- **Concurrency**: Multiple agents may log concurrently; order may vary
- **Telemetry Interval**: Default is 1 second; adjust via `TelemetryConfig.Interval`

