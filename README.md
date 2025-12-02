# Spacetime-Compatible Constellation Simulator (Go)

An experimental backend for simulating satellite constellations and ground networks in Go, designed to be **API-compatible with Aalyria Spacetime’s concepts** (platforms, nodes, links, service requests, etc.).

The goal is to provide a **local, headless engine** you can run on your own infrastructure to:

- Model satellites, ground stations, and other platforms
- Propagate their motion over time (e.g. using TLE + SGP4)
- Attach network nodes and interfaces to those platforms
- Evaluate when links are geometrically possible (line-of-sight, horizon limits, Earth occlusion)
- Eventually expose all of this via a Spacetime-style gRPC Northbound Interface (NBI)

It is intended for:

- Developers who want a **Spacetime-like sandbox** for testing higher-level logic
- Researchers and engineers who need a **programmable constellation model** without relying on a closed service
- People experimenting with satellite + terrestrial network design and automation

---

## Current Capabilities

At the moment, the simulator focuses on **core constellation modelling and basic connectivity**.

### Constellation model

- **Platforms**: physical objects like satellites, ground stations, airborne relays, etc.
  - Unique IDs, names, type tags
  - Motion data in Earth-centred, Earth-fixed (ECEF) coordinates
  - Support for:
    - Orbital platforms using **TLE + SGP4** propagation
    - Static platforms (e.g. ground stations)

- **Network nodes**: logical endpoints hosted on platforms
  - Nodes can be attached to platforms to **inherit their position**
  - Node state is kept in sync as platforms move

- **In-memory Knowledge Base**
  - Thread-safe store for platforms and nodes
  - Simple CRUD-style operations
  - Designed to be accessed from both the simulation loop and future APIs

### Time-stepped simulation

- **Time Controller**
  - Drives the simulation with a configurable tick interval
  - Emits ticks to subscribers (motion models, connectivity evaluator, etc.)
  - Can be run in “accelerated” mode for fast-forward scenarios

- **Motion models**
  - Static: fixed ECEF coordinates
  - Orbital:
    - Uses an SGP4 implementation to propagate satellites from TLE
    - Updates platform positions every tick
    - Positions are stored in a consistent coordinate frame for downstream use

### Network interfaces & connectivity (early)

- **Network interfaces per node**
  - Multiple interfaces per node (wired and wireless)
  - Fields for IDs, type, attachment to a node, and basic addressing / metadata

- **Transceiver models (wireless)**
  - Separate model for RF characteristics:
    - Frequency band, antenna parameters (e.g. gain / pattern placeholders)
    - Power, sensitivity, etc. (as required by the current connectivity logic)
  - Wireless interfaces reference a transceiver model

- **Links & connectivity evaluation**
  - Support for:
    - **Wired links** (e.g. terrestrial fiber/Ethernet) treated as always-available, with fixed characteristics
    - **Wireless links** evaluated on each tick:
      - Line-of-sight checks between platforms
      - Horizon angle constraints for ground stations
      - Earth occlusion checks for space-space links
    - (Currently assumes simple omni-directional coverage by default)
  - Produces a **time-varying connectivity map**:
    - At any given tick, the engine knows which links are geometrically “up”
    - This is the foundation for later routing/scheduling logic

---

## Planned / In Progress

The next major step is to add a **Spacetime-style Northbound Interface (NBI)** and richer scenario controls:

- gRPC services for:
  - Defining platforms, nodes, interfaces, links, service requests
  - Bulk load / snapshot / clear of entire scenarios
- Validation & referential integrity:
  - Strong checks for IDs, references, and motion sources
- Observability:
  - Structured logging, metrics (Prometheus-friendly), and optional tracing
- End-to-end tests and example clients:
  - Spin up the engine + gRPC server and drive it via generated clients

These are being tracked as “Scope 3” internally, but from the outside you can just think of it as “add NBI + scenario API on top of the current engine”.

---

## Repository Layout

> Names may evolve a bit as things grow, but this is the current structure.

- `cmd/simulator/`  
  CLI entrypoint that wires the core engine together and runs a demo scenario.

- `model/`  
  Core data structures:
  - `PlatformDefinition`, `NetworkNode`, motion/orbit-related structs
  - Pure data containers; no heavy logic

- `kb/`  
  Thread-safe in-memory state store:
  - Platforms, nodes, and other entities the sim tracks
  - Designed for concurrent reads/writes from sim loop and APIs

- `timectrl/`  
  Time Controller that:
  - Emits ticks on a configured schedule
  - Coordinates time advancement for motion and connectivity evaluation

- `core/`  
  Simulation engine layer:
  - Motion models (SGP4, static)
  - Connectivity / link evaluation
  - Orchestration glue between TimeController, KnowledgeBase, and entity models

- `testdata/`  
  Sample TLEs and fixtures used by tests and demo runs.

- `docs/adr/`  
  Architecture Decision Records, capturing high-level design choices over time:
  - `0001-aalyria-proto-integration.md` – how Aalyria Spacetime API protos will be integrated for future NBI work.

As the NBI work lands, you will also see:

- `third_party/aalyria/` – vendored Aalyria `.proto` files (NBI + common types)
- `nbi/gen/` – generated Go code from those protos
- `cmd/nbi-server/` – a standalone gRPC server that exposes the simulator via NBI

---

## Scenario State

ScenarioState is the central in-memory facade that keeps the Scope 1 physical KB, Scope 2 network KB, and in-memory service request store consistent for the simulator and future NBI handlers. It is the expected entry point for scenario CRUD, snapshots, and clears; see [docs/architecture/scenario-state.md](docs/architecture/scenario-state.md) for design and usage guidance.

## Getting Started

### Prerequisites

- Go 1.21+ (or a reasonably recent Go toolchain)
- Git
- Internet access for `go mod tidy` (to pull dependencies like SGP4)

### Clone and build

```bash
git clone https://github.com/<your-username>/spacetime-constellation-sim.git
cd spacetime-constellation-sim

go mod tidy          # fetch dependencies
go build ./cmd/simulator

