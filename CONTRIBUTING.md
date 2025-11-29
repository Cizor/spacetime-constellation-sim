# Contributing

This repository is a reference implementation of **Scope 1** of a Spacetime‑compatible constellation simulator.

The intent is:

- Clear, idiomatic Go
- A clean separation between **models**, **storage (Knowledge Base)**, **time control**, and **motion/physics**
- Easy extension into future scopes (connectivity, routing, planning, external APIs)

## Project Structure

- `model/`  
  Pure data models for platforms, nodes, and motion.

- `kb/`  
  The in‑memory Knowledge Base; responsible for:
  - Thread‑safe CRUD of platforms and nodes
  - Optional event subscriptions (e.g. for metrics or visualisation)

- `timectrl/`  
  The Time Controller; responsible for:
  - Managing simulation time (start, duration, tick interval)
  - Emitting ticks to registered listeners

- `core/`  
  Motion models and orchestration logic:
  - Static motion (no change with time)
  - Orbital SGP4 motion (TLE‑driven satellites)

- `cmd/simulator/`  
  A small CLI that:
  - Builds a demo scenario
  - Wires together KB, motion models, and Time Controller
  - Prints positions to stdout

## Development Workflow

1. **Add or update tests first** (TDD).
2. Implement or modify the feature under `model/`, `kb/`, `timectrl/`, or `core/`.
3. Run tests:

   ```bash
   go test ./... -race -v
   ```

4. Keep packages small and focused. Avoid introducing cross‑package cycles.
5. Prefer explicit interfaces where they help testing and decoupling.

## Coding Style

- Follow standard Go idioms (`go fmt`, simple error handling).
- Avoid premature optimisation; only optimise when needed and measurable.
- Keep public APIs small and coherent.

## External Libraries

For Scope 1 we intentionally keep external dependencies minimal. The main non‑standard dependency is:

- `github.com/joshuaferrara/go-satellite` for SGP4/TLE orbit propagation.

If you add more libraries, prefer:

- Actively maintained projects
- Permissive licences compatible with open‑source use (e.g. MIT, BSD, Apache‑2.0)

## Extending Beyond Scope 1

Later scopes can introduce:

- Connectivity / visibility computations
- Contact plans and routing plans
- Northbound / southbound APIs (gRPC/REST)
- Persistence layers (PostgreSQL, etc.)
- Metrics and observability

Try to keep Scope 1 relatively stable and clean so that later scopes can build on it without large refactors.
