# DTN Lifecycle

## Purpose
Disruption tolerant networking in the simulator tracks every bundle that is stored, forwarded, or eventually expired. This document explains how DTN messages move through `ScenarioState` and the scheduler, how the message state machine is updated, and how operators can inspect the resulting metadata.

## Message states
- `pending`: a message has been scheduled by the controller but has not yet been placed into a node's DTN storage. Scheduler decisions (`FindDTNPath`, `ScheduleStoreMessage`) create the initial entry with this state.
- `stored`: `ScenarioState.StoreMessage` writes the bundle into the destination node's `DTNStorage`, records the arrival time, and emits `MessageStateStored`. If the storage node is also the eventual destination, the state immediately upgrades to `delivered`.
- `in_transit`: when the agent handles a `ScheduledForwardMessage`, it removes the bundle from storage via `ScenarioState.RetrieveMessage` and records `MessageStateInTransit` so downstream actions know the message is being forwarded.
- `delivered`: storing the bundle at the destination node or explicitly delivering it via `ScenarioState.StoreMessage` marks the write as `MessageStateDelivered`.
- `expired`: each stored message can carry an expiry timestamp; `ScenarioState.scheduleExpiryEvent` registers a callback in the event scheduler, and `handleMessageExpiry`/`EvictExpiredMessages` record `MessageStateExpired` when the timestamp is reached.

## Transition drivers
- The scheduler creates `ScheduledStoreMessage`/`ScheduledForwardMessage` actions for each hop of a DTN path. When an agent executes those actions, it calls into `ScenarioState.StoreMessage` or `ScenarioState.RetrieveMessage`, which in turn update `messageStates` and `messageHistory` via `recordMessageState`.
- Storage capacity is enforced through `canStoreMessage`/`StorageUsage`. The scheduler consults `ScenarioState.GetStorageUsage` before scheduling another store action and removes the reservation via `releaseStorageForSR` when the message leaves the node.
- Expiry handling is tied to the simulator clock: `scheduleExpiryEvent` keeps a map of pending timers and `handleMessageExpiry` clears storage plus records the `MessageStateExpired` transition if the bundle is still present.
- `MessagesInState` and `GetMessageState` expose the current state and history entries for debugging, while `ScenarioState.messageHistory` keeps every transition so inspection can show the `pending → stored → in_transit → delivered/expired` flow for a given `MessageID`.

## Integration points
- `Scheduler.findDTNPath` builds the multi-hop plan (`DTNPath`) and emits the sequence of storage hops that must execute ordered store/forward actions.
- `SimAgent.execute` responds to each scheduled action and translates the `DTNMessageSpec` payload into a `StoredMessage` structure with size, expiry, and destination metadata before calling into `ScenarioState`.
- `ScenarioState.StorageUsage`, `GetStorageUsage`, and `StorageCapacityBytes` on `model.NetworkNode` keep track of per-node quotas so multiple messages can be simulated without overflowing buffers.
- Backpressure and cleanup happen when the scheduler calls `releaseStorageForSR` (after dropping a request) or when `EvictExpiredMessages` removes stale bundles from a node.

## Observability
- Operators can call `ServiceRequestService.GetServiceRequestStatus` to learn when a disruption-tolerant request was scheduled and when it last left/entered a node. The `model.TimeInterval.Path` field contains the node sequence, while DTN-specific metadata lives in `messageStates`.
- `ScenarioState.recordMessageState` updates the metering hooks that Prometheus collects via the telemetry service (see `internal/sim/state/telemetry.go`), so new histograms can join the lifecycle events if needed.
