# Spacetime-Compatible Constellation Simulator (Go)

A Go-native simulation engine that models satellite constellations, ground infrastructure, and scheduling logic inspired by Aalyria Spacetime. Combine the scheduler, SBI runtime, and telemetry services in a single repo so you can prototype high-fidelity constellation automation locally.

## Highlights

- **Entity modeling**
  - Build platforms (orbital or static), attach network nodes/interfaces, and track links in thread-safe knowledge bases (kb.Platform, kb.NetworkNode, core.NetworkLink).
  - Capture service requests, priorities, and metadata in internal/sim/state while keeping the motion/geometry loop separate.
- **Scheduler + SBI runtime**
  - Scheduler precomputes contact windows, assesses conflict/power/frequency rules, handles DTN store-and-forward, and emits structured scheduled actions.
  - The SBI controller and agents run inside cmd/nbi-server, exposing CDPI actions (beams, routes, SR policies) plus telemetry hooks.
- **Telemetry & observability**
  - Instrumented telemetry clients gather interface, intent, and scheduler metrics.
  - Exporters and tests exercise the telemetry service so you can connect Prometheus or custom dashboards.

## Getting started

git clone https://github.com/Cizor/spacetime-constellation-sim.git
cd spacetime-constellation-sim

# Download dependencies
go mod tidy

# Build the gRPC server
go build ./cmd/nbi-server

# Run the server
cmd\nbi-server\nbi-server.exe
`

The gRPC server listens for both NBI-style services (platforms, nodes, links, service requests) and SBI services (Scheduling, Telemetry). Check cmd/nbi-server/README.md for configuration knobs and TLS setup.

## Testing

`
go test ./...
`

Unit and integration suites cover scheduler logic, agent behaviors, and telemetry interactions under the internal/sbi/ and internal/sim/state/ packages.

## Contributing

1. Open an issue describing the feature, bug, or doc change.
2. Draft your changes in a feature branch following the existing Go formatting (gofmt/goimports).
3. Add or adjust tests wherever behavior changes.
4. Run go test ./... locally before submitting a PR.
5. Reference the relevant README sections in your PR summary.

Need help? Reach out via GitHub discussions or review the code in internal/sbi/ for the scheduler, controller, and telemetry layers.
