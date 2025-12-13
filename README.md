# Spacetime-Compatible Constellation Simulator (Go)

Spacetime-Compatible Constellation Simulator is a headless Go service that models satellite constellations, ground infrastructure, and their control-plane automation through Aalyria Spacetime-inspired concepts. Run it locally to experiment with platforms, nodes, links, telemetry, and SBI/NBI scheduling without relying on the production service.

## What the project does today

- **Entity modeling**
  - Platforms (orbital or static) with motion propagation via SGP4 or a fixed position.
  - Network nodes attached to platforms, each with multiple wired or wireless interfaces.
  - Knowledge bases (`kb.Platform`, `kb.NetworkNode`, `core.NetworkLink`) kept in memory, thread-safe, and shared between the simulation loop and APIs.

- **Simulation engine**
  - Time controller (`timectrl`) that advances the clock on a configurable tick interval and exposes `Now()`/`SetTime()`.
  - Motion models that update platform positions every tick and write them back to the knowledge base.
  - Connectivity evaluator that marks wired links as available and evaluates wireless links based on geometry (line-of-sight, horizon, occlusion), creating a time-varying view of potential and active links.

- **Scope 4 SBI controller**
  - SBI runtime integrated into `cmd/nbi-server` with TCP/TLS server, CDPI + telemetry gRPC services, and in-process agent connections.
  - Scheduler that precomputes contact windows, normalizes beam interface IDs, orders service requests by priority, logs conflicts/power issues, and tracks DTN storage reservations via the scenario state.
  - Simulation loop that advances the clock, runs `EventScheduler.RunDue()` each tick, and periodically recomputes windows and re-runs service requests to react to topology changes.
  - Telemetry/agent stack that builds interface metrics and has hooks for future modem metrics, intents, and expanded observability.

- **Northbound interface**
  - `cmd/nbi-server` hosts Aalyria-style gRPC services (`Scheduling`, `Telemetry`, etc.) alongside NBI services (platform, node, link, service request, scenario), so clients can manage the scenario and read telemetry from a single endpoint.

- **Testing**
  - Tests cover the scheduler, run loop, contact-window logic, and time controller (`go test ./...`) to keep the newer reactive behavior validated.

## Repository layout (highlighted)

- `cmd/nbi-server/`: gRPC server wiring ScenarioState, SBI runtime, telemetry, and NBI services.
- `internal/sbi/`: SBI controller, scheduler, runtime, agent, and telemetry implementations.
- `internal/sim/state/`: ScenarioState unifying Scope 1 and Scope 2 knowledge bases plus service request bookkeeping and DTN storage tracking.
- `core/`, `kb/`, `model/`, `timectrl/`: motion/connectivity engine, in-memory KBs, data definitions, and the time controller.
- `docs/` & `docs/planning/`: design records, requirements, roadmaps, and planning artifacts that document ongoing work.

## Getting started

1. Install Go 1.21+ and ensure the Go tooling is on your PATH.
2. `go mod tidy` to download dependencies.
3. `go build ./cmd/nbi-server` (or `./cmd/simulator` for the demo CLI).
4. Run the binary, load a scenario via the gRPC NBI (or REST shim), and watch the SBI scheduler manage beams/routes.
5. `go test ./...` verifies the scheduler, run loop, and time controller logic.

Run `git status` to see the files touched during development and consult `docs/planning/` for the broader roadmap if you want to contribute new features.
