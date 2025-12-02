# ScenarioState

## Purpose
ScenarioState is a facade that coordinates the simulator's scenario data. It sits on top of the Scope 1 physical knowledge base (platforms/nodes), the Scope 2 network knowledge base (interfaces/links), and an in-memory store of active service requests. NBI handlers and other services should treat ScenarioState as the single entry point for reading and mutating scenario data instead of talking to the KBs directly.

## Key responsibilities
- Entity-level CRUD:
  - Platforms via Create/Get/List/Update/DeletePlatform.
  - Network nodes and network interfaces live in the underlying KBs; ScenarioState owns those KB instances so handlers can reach them via PhysicalKB() and NetworkKB() until dedicated helpers land.
  - Network links via Create/Get/List/Update/DeleteLink (applies adjacency changes).
  - Service requests via Create/Get/List/Update/DeleteServiceRequest (in-memory store keyed by internal ID).
- Scenario-level operations:
  - Snapshot() grabs a read lock and returns a consistent view across both KBs and the service request map; callers must treat returned pointers as read-only.
  - ClearScenario() wipes both KBs and the service request map so a new scenario can be loaded cleanly.
- Consistency: all mutating operations take the ScenarioState write lock to keep the two KBs and the service request map in sync and to provide a single serialization point for NBI writes.

## Concurrency model
ScenarioState wraps a sync.RWMutex around the two KB handles and the service request map. Read-only operations (Snapshot, getters, list helpers) take the read lock; mutations take the write lock. The underlying KBs are individually thread-safe, but the ScenarioState lock is what makes cross-KB reads/writes (e.g., link creation that relies on interfaces, or snapshots that span both KBs) coherent for callers.

### Simulation loop interaction
Motion/position propagation and connectivity evaluation should treat ScenarioState as the authoritative view. Use read-only paths (Snapshot or KB getters accessed while holding the ScenarioState read lock) when sampling state inside the sim loop. NBI handlers must perform writes through ScenarioState's mutating methods so they go through the shared write lock and keep sim-time readers consistent.

## Integration points
- Construction: NewScenarioState(physKB, netKB) receives existing Scope 1 and Scope 2 knowledge bases and wires them under the facade.
- NBI services (Scope 3): per-request handlers should depend on a shared ScenarioState instance rather than touching KBs directly. This ensures service request CRUD, platform/node/interface/link updates, and scenario clears go through one lock and remain consistent for concurrent readers.
- Main server binaries: components such as cmd/nbi-server should instantiate the two KBs, wrap them in a ScenarioState, and pass that into both the simulation loop and the NBI service layer so they operate over the same synchronized view.
