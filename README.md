# Spacetime-Compatible Constellation Simulator

This repository implements a **Spacetime-compatible constellation simulator** in Go.

It currently covers:

- ✅ **Scope 1 – Core entities & orbital dynamics**
  - Platform & network node models
  - Thread-safe in-memory Knowledge Base
  - Time Controller for driving simulation time
  - Motion models including SGP4-based orbital propagation
  - CLI that runs a demo scenario (1 LEO + 1 ground station)

- ✅ **Scope 2 – Network interfaces & connectivity evaluation**
  - Network interfaces attached to nodes
  - Basic link / connectivity modelling between interfaces
  - Evaluation of link availability based on geometry and motion

- 🚧 **Scope 3 – Northbound API & scenario configuration (planned)**
  - Aalyria-compatible NBI services over gRPC
  - Scenario-level snapshot / load / clear
  - Validation, observability, and end-to-end tests

The design follows the **requirements, roadmap, architecture**, and **Scope 1/2/3 implementation plans** you wrote separately.

---

## Project Layout

> Note: exact package names may evolve, but this is the current intent.

- `cmd/simulator/`  
  CLI entrypoint that wires Scope 1 & 2 pieces together and runs a demo scenario.

- `model/`  
  Core data models for **Scope 1**:
  - `PlatformDefinition`
  - `NetworkNode`
  - Motion / orbit-related structs, etc.

- `kb/`  
  Thread-safe Knowledge Base:
  - Stores platforms and nodes (Scope 1).
  - Extended to track network-related state used by Scope 2.

- `timectrl/`  
  Time Controller that:
  - Emits ticks at a configured interval.
  - Drives the motion & connectivity updates.

- `core/`  
  Simulation “engine” layer:
  - Motion models (static + SGP4-based orbital propagation).
  - Orchestration helpers that connect TimeController, KnowledgeBase, and connectivity evaluation.
  - Scope 2 logic for evaluating connectivity based on node/platform positions and interfaces.

- `testdata/`  
  Sample TLEs and other fixtures used by tests and demo scenarios.

- `docs/adr/`  
  Architecture Decision Records:
  - `0001-aalyria-proto-integration.md` – how we will integrate Aalyria Spacetime API protos for Scope 3.

As Scope 3 is implemented, you’ll also see:

- `third_party/aalyria/` – vendored Aalyria `.proto` files (NBI + common types).
- `nbi/gen/` – generated Go code from those protos.
- `cmd/nbi-server/` – NBI gRPC server binary (Scope 3).

---

## Building

From the repo root:

```bash
go mod tidy   # fetch external dependencies (e.g. go-satellite)
go build ./cmd/simulator

