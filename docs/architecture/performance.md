# Performance baseline

This document captures initial Scope-3 performance targets for the NBI bulk-create paths and records a baseline measurement gathered on a developer machine. These numbers are guidance targets, not guarantees.

## Target scale
- Platforms: 1,000
- Network nodes: 1,000–3,000 (benchmarks cover 1,000 by default; 3,000 under `perf_large`)
- Interfaces per node: 2 (wired, for consistency)
- Links: 1,000 (default) / 3,000 (`perf_large`)
- Service requests: 1,000 (default) / 3,000 (`perf_large`)

## How to run
- Default perf suite (not run in CI): `go test -run=^$ -bench=. -benchmem -tags=perf ./internal/nbi/perf`
- Larger scale: `go test -run=^$ -bench=. -benchmem -tags=perf_large ./internal/nbi/perf`
- Tests live in `internal/nbi/perf` and are guarded by build tags so `go test ./...` remains fast.

## Baseline results
- Environment: Windows dev machine, Go 1.24.10 (w/ default GC settings)
- Hardware: (see below from `Get-CimInstance Win32_Processor` / `Win32_ComputerSystem`)
- Command: `go test -run=^$ -bench=. -benchmem -tags=perf ./internal/nbi/perf`
- Results (ops/sec approximate; lower is faster):
  - `BenchmarkPlatformCreateSmall`: ~0.60 ms (≈1.7k ops/sec), 686 KB allocs, 15.8k allocs/op
  - `BenchmarkNodeCreateSmall`: ~2.17 ms (≈460 ops/sec), 2.69 MB allocs, 54.8k allocs/op
  - `BenchmarkLinkCreateSmall`: ~2.80 ms (≈360 ops/sec), 3.02 MB allocs, 60.1k allocs/op
  - `BenchmarkServiceRequestCreateSmall`: ~0.60 ms (≈1.7k ops/sec), 1.04 MB allocs, 22.8k allocs/op
  - Command used single-iteration timing (`-benchtime=1x`) to keep the run fast while capturing wall time and allocations.
  - Hardware sample: 12th Gen Intel(R) Core(TM) i7-12700K (12 cores / 20 threads), ~128 GB RAM (137,167,814,656 bytes)

## Notes and future work
- Benchmarks use in-memory ScenarioState and NBI services to exercise the same validation and locking paths as gRPC handlers.
- Interfaces are wired-only to keep setup light; wireless paths can be added once transceiver catalog seeding is cheap.
- If contention shows up, likely areas to explore:
  - Finer-grained read paths inside ScenarioState
  - Pooling for frequently allocated protos / domain structs
  - Batch APIs for bulk ingest
