# Concurrency model

This repository uses ScenarioState as the concurrency boundary for scenario-level operations. The two underlying KBs remain internally thread-safe for per-collection access, but all multi-entity work is coordinated through ScenarioState.

## Locks and ownership
- ScenarioState owns a coarse `sync.RWMutex` that guards cross-entity operations and Scenario-level invariants (e.g., nodes + interfaces + links + service requests).
- `kb.KnowledgeBase` (platforms/nodes) and `core.KnowledgeBase` (interfaces/links/transceivers) each use their own internal `sync.RWMutex` for per-collection safety.
- Lock ordering is strict: take the ScenarioState lock first (read for reads, write for mutations), then call into KB methods which take their own internal locks. Never acquire ScenarioState while already inside a KB method.

## NBI and mutating operations
- All NBI-driven writes must go through ScenarioState mutators so they take the ScenarioState write lock before touching either KB.
- Reads that need a coherent view across multiple entities (snapshots, node + interfaces, etc.) should go through ScenarioState read helpers or wrap logic in `ScenarioState.WithReadLock`.
- Do not bypass ScenarioState to talk directly to the KBs from NBI handlers; doing so risks violating lock ordering and cross-KB consistency.

## Simulation loop
- TimeController/MotionModel/ConnectivityService should treat ScenarioState as the authoritative view and use read-only access (Snapshot or helpers under the ScenarioState read lock). Avoid taking write locks from the sim loop except for explicit mutation workflows.

## Practical rules of thumb
- Mutations: ScenarioState write lock -> KB write locks as needed.
- Consistent reads: ScenarioState read lock -> KB read locks as needed.
- Lock ordering: ScenarioState first, KBs second; never the inverse.
