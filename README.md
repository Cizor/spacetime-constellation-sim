# Spacetime-Compatible Constellation Simulator (Go)

Spacetime-Compatible Constellation Simulator is a headless Go backend for modeling space-to-ground constellation topologies and exposing control-plane automation inspired by Aalyria Spacetime’s data model. It is designed for developers who want to experiment with satellite + terrestrial networks without depending on a closed service.

## What it does today

- **Motion + Knowledge Bases**
  - Define platforms (satellites, ground stations, etc.) with orbital or static motion.
  - Attach network nodes and interfaces, backed by thread-safe knowledge bases.
  - Propagate motion every tick using SGP4 (for orbital platforms) or static motion, updating node positions in the process.

- **Connectivity evaluation**
  - Evaluate wired links as always available and wireless pairs via geometry (line-of-sight, horizon, Earth occlusion).
  - Track dynamic link statuses so downstream components know what’s “potentially up” at any instant.

- **Scope 4 SBI controller**
  - Integrated SBI runtime wired into the main gRPC server.
  - Event scheduler that keeps agents, beams, and routes in sync with the simulation clock.
  - Beam + route scheduling that uses precomputed contact windows, interfaces with the SBI CDPI server, and tracks DTN storage reservations.
  - Service request scheduler with priority ordering, DTN heuristics, and periodic re-planning within the simulation loop.
  - Time controller that exposes `Now()`/`SetTime()` so the scheduler and agents stay in lock-step with each tick.

- **Northbound interface (NBI)**
  - `cmd/nbi-server` exposes the Aalyria-style gRPC services (`Scheduling`, `Telemetry`, etc.) on the same server as the simulator.
  - Telemetry server collects interface-level metrics, beams, and has hooks for future enhancements (modem metrics, intents).
  - SBIRuntime brings up agents, the CDPI server, and the scheduler with TLS-aware dialing and in-process gRPC connection for agents.

## Highlights

- **Periodic re-planning**: the sim loop now updates the SBI event scheduler every tick and, every few simulated minutes, recomputes contact windows and reschedules service requests to react to topology change.  
- **DTN tracking**: storage reservations are enforced via the scenario state, with per-request accounting to prevent oversubscription.  
- **Conflict detection & power heuristics**: the scheduler logs when interfaces exceed their `MaxBeams` or power budgets, reduces window lengths for time-slicing, and reports storage usage.  
- **Extensive testing**: coverage includes contact-window computation, run loop event scheduling/re-planning, and TimeController behavior plus the existing scheduler/unit suites.

## Repository layout

- `cmd/nbi-server/`: gRPC server wiring the SBI runtime + ScenarioState alongside the traditional NBI services.  
- `internal/sbi/`: controller runtime, scheduler, agent, and telemetry implementations for Scope 4.  
- `internal/sim/state/`: ScenarioState that unifies Scope 1 (platforms) and Scope 2 (interfaces/links) knowledge bases plus service-request bookkeeping.  
- `core/`, `kb/`, `model/`, `timectrl/`: the motion/connectivity engine, in-memory KBs, data definitions, and time controller pieces previously described.  
- `docs/`, `docs/planning/`: architecture planning, requirements, and implementation plans for each scope.

## Scope 5 / future work

- Conflict resolution & time-slicing for multi-request contention  
- Time-aware multi-hop pathfinding (with DTN vs non-DTN awareness)  
- Reactive re-planning exposed as APIs + failure handling  
- Power-budget accounting + telemetry expansion (modem metrics, intents)  
- Region-based service requests, federation support, stronger observability

## Getting started

1. `go mod tidy` to pull dependencies  
2. `go build ./cmd/nbi-server` (or `./cmd/simulator` for the non-gRPC demo)  
3. Start the binary, load a scenario (see `docs/planning` for current plans), and use gRPC tooling to exercise the NBI/SBI endpoints.

Current tests cover the scheduler, event loop, contact windows, and time controller; run `go test ./...` to verify locally.
