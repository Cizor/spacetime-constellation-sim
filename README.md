# Spacetime-Compatible Constellation Simulator (Go)

A Go-based, API-compatible engine for modeling satellite constellations, ground infrastructure, and control-plane automation inspired by Aalyria Spacetime. Run it locally to experiment with constellations, schedule beams/routes, and expose telemetry without relying on a hosted service.

## Key features

- **Entity modeling**
  - Define platforms (orbital or static), attach network nodes, and assign interfaces.
  - Maintain thread-safe knowledge bases (`kb.Platform`, `kb.NetworkNode`, `core.NetworkLink`) shared between simulation and APIs.

- **Time-stepped simulation**
  - `timectrl` drives the clock on a configurable tick interval with `Now()`/`SetTime()` hooks.
  - Motion models update platform positions every tick (SGP4 for orbits, static for fixed platforms).
  - Connectivity evaluates wired/wireless links each tick (geometry, horizon, occlusion) to keep a live view of potential/activity.

- **Control-plane automation**
  - SBI runtime sits inside `cmd/nbi-server`, registering CDPI + telemetry on the same gRPC server as the NBI services.
  - Scheduler precomputes contact windows, normalizes beam/interface IDs, batches service requests by priority, tracks DTN storage, and logs conflict/power heuristics.
  - Simulation loop runs `EventScheduler.RunDue()` each tick and periodically recomputes contact windows plus reruns service request planning.

- **Northbound API**
  - The gRPC server exposes both Aalyria-style NBI services (platforms, nodes, links, service requests, scenarios) and SBI services (`Scheduling`, `Telemetry`), all configurable with TLS.
  - SBIRuntime wires telemetry clients, CDPI, and agents together so you can drive beams/routes through gRPC clients.

- **Telemetry + observability**
  - Interface-level metrics flow through the telemetry service, with foundations in place for future modem, intent, and observability enhancements.
  - Structured logging and Prometheus-friendly metrics provide insight when running locally or in CI.

## Getting started

```bash
git clone https://github.com/Cizor/spacetime-constellation-sim.git
cd spacetime-constellation-sim

# fetch dependencies
go mod tidy

# build the gRPC server
go build ./cmd/nbi-server

# run the server and point your gRPC client at $LISTEN_ADDRESS
./cmd/nbi-server/nbi-server
```

Inspect `cmd/nbi-server/README.md` (if available) or the docs in `docs/planning/` for sample scenarios and API usage. Run all tests with:

```bash
go test ./...
```

## Repository layout

- `cmd/nbi-server/`: Unified gRPC server hosting both NBI and SBI services.
- `internal/sbi/`: SBI runtime, scheduler, controller, agent, and telemetry implementations.
- `internal/sim/state/`: ScenarioState unifying Scope 1 and Scope 2 knowledge plus service request and DTN storage bookkeeping.
- `core/`, `kb/`, `model/`, `timectrl/`: Motion/connectivity engine, knowledge bases, data models, and time controller utilities.
- `docs/`, `docs/planning/`: Architectural docs, roadmaps, and requirements that describe what’s next.

## What’s next

Roadmap items include scheduler improvements (conflict handling, time-aware multi-hop paths, reactive re-planning), expanded telemetry (modem metrics, intents), and features like region-based requests and federation support. Use `docs/planning/` for the latest planning artifacts and to get involved in the work.
