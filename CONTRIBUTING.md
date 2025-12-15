# Contributing

Thank you for improving the Constellation Simulator. To keep the project healthy: add tests for new behavior, run the suite to confirm nothing breaks, and describe your change clearly in the pull request.

## Workflow
1. Create an issue describing the problem or enhancement.
2. Work on a feature branch. Keep changes focused on a single concern.
3. Format Go files with gofmt/goimports and keep imports tidy.
4. Run go test ./... locally before committing.
5. Update the README or other docs if the user-facing behavior changed.
6. Open a pull request referencing the issue and summarize the change in the description.

## Project Layout
- internal/sbi/: Scheduler, controller, agents, and telemetry internals for the SBI runtime.
- internal/sim/state/: Scenario state, service requests, DTN bookkeeping, and telemetry helpers.
- core/, kb/, model/, 	imectrl/: Motion models, knowledge bases, data models, and the time controller.
- cmd/nbi-server/: gRPC server wiring together NBI and SBI services with telemetry support.

## Style Guidelines
- Prefer simple, testable interfaces rather than large concrete types.
- Keep packages small and focused to avoid dependency cycles.
- Make telemetry/logging optional so tests remain deterministic.

## Testing
- Add unit tests under the packages you touch.
- Use table-driven tests when covering multiple cases.
- Keep integration tests deterministic by seeding clocks and using fake telemetry clients.
