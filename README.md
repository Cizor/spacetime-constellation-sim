# Constellation Simulator – Scope 1

This repository implements **Scope 1** of a Spacetime‑compatible constellation simulator in Go:

- Core platform & network node models
- Thread‑safe in‑memory Knowledge Base
- Time Controller for driving simulation time
- Motion models including SGP4‑based orbital propagation
- A small CLI that runs a demo scenario (1 LEO + 1 ground station)

The design follows the requirements, roadmap, architecture, and Scope 1 implementation plan you created.


## Project Layout

- `cmd/simulator/` – CLI entrypoint that wires everything together
- `model/` – Core data models (PlatformDefinition, NetworkNode, Motion, etc.)
- `kb/` – Thread‑safe Knowledge Base for platforms and nodes
- `timectrl/` – Time Controller that emits ticks to listeners
- `core/` – Motion models and orchestration helpers
- `testdata/` – Sample TLEs and other fixtures

## Building

From the repo root:

```bash
go mod tidy   # fetch external dependencies (e.g. go-satellite)
go build ./cmd/simulator
```

## Running the demo

The demo scenario is implemented in `cmd/simulator/main.go`.

Example:

```bash
go run ./cmd/simulator   -duration 60s   -tick 1s
```

This will:

- Create a LEO satellite platform using an ISS TLE
- Create a static ground station at an approximate equatorial point
- Run the Time Controller in accelerated mode
- On each tick, propagate the satellite using SGP4 and update the Knowledge Base
- Print the ECEF coordinates of both platforms at each tick

## Testing

Run all tests with:

```bash
go test ./... -race -v
```

The tests cover:

- Knowledge Base CRUD and concurrency behaviour
- Time Controller tick dispatch
- Motion model behaviour (static vs orbital)
- A small integration test wiring the pieces together

You can extend tests as you expand to later scopes (connectivity, planning, NBI/SBI services, etc.).

## External Dependencies

This project uses:

- [`github.com/joshuaferrara/go-satellite`](https://github.com/joshuaferrara/go-satellite) for SGP4/TLE orbit propagation.

Dependencies are resolved automatically when you run `go mod tidy` or `go test` / `go build`.

## Notes

- All positions are expressed in **ECEF meters** internally.
- Time is always represented as `time.Time` in UTC.
- Scope 1 intentionally does **not** include connectivity, scheduling, or planning – those appear in later scopes.
