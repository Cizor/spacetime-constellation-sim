# Federation

Scope 5 currently includes a lightweight federation stub that allows other components to reason about inter-domain service requests without implementing a full gRPC federation mesh.

## Existing implementation

- `internal/sbi/controller/federation_protocol.go` defines `FederationClient`, `FederationRequest`, and `FederationResponse`.
- `InMemoryFederationClient` is a no-op implementation that accepts a valid `schedule_manipulation_token` and always returns a single `PathSegment` wrapping the destination domain. It is used by `FindFederatedPath`/`FindFederatedPathWithClient` (`internal/sbi/controller/federation_path.go`) to short-circuit cross-domain path construction.
- Tests exercise the stub (`federation_protocol_test.go`, `federation_path_test.go`) so the controller behaves deterministically when the federation surface is present.

## Operator guidance

The current stub is intentionally simple: it does not reach across network boundaries, and it only supports requests that explicitly set `ServiceRequest.CrossDomain`, `SourceDomain`, `DestDomain`, and `FederationToken`. Treat the stub as "simulated federation only" in documentation and system tests.

## Next steps

1. Implement a production `FederationClient` that dials a real gRPC federation service (e.g., Aalyria's `interconnect` API), serializes `FederationRequest`, and parses multi-domain `PathSegment` responses.
2. Add configuration knobs (certs, endpoints) to `cmd/nbi-server` and wire them into `SBIRuntime` so each controller instance knows how to reach its federation peers.
3. Keep the current stub for local simulations by gating it behind a feature flag or by providing a `--use-federation-stub` configuration mode.
