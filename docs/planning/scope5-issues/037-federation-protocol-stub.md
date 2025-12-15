# Issue #037: Define Federation Protocol (Stubbed Implementation)

**Labels:** `scope5`, `federation`, `protocol`, `stub`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** Low

## Description

Define inter-domain coordination protocol for federation. For Scope 5, implement stubbed version (in-memory coordination) but define complete protocol for future implementation.

## Tasks

1. **Define FederationRequest type**:
   ```go
   type FederationRequest struct {
       RequestID    string
       SourceDomain string
       DestDomain   string
       Requirements FlowRequirements
       Token        string
   }
   ```

2. **Define FederationResponse type**:
   ```go
   type FederationResponse struct {
       RequestID string
       PathSegment *PathSegment // path within responding domain
       Status    string
   }
   ```

3. **Stub federation client**:
   - For Scope 5: in-memory coordination
   - Simulate inter-domain requests/responses
   - Log federation operations

4. **Protocol definition**:
   - Document request/response format
   - Document authentication (token-based)
   - Document error handling
   - Document timeout/retry logic

5. **Future extension points**:
   - Interface for real gRPC federation client
   - Placeholder for actual network coordination
   - Clear separation between stub and real implementation

## Acceptance Criteria

- [ ] FederationRequest and FederationResponse types are defined
- [ ] Stub federation client works for in-memory coordination
- [ ] Protocol is documented
- [ ] Stub can be replaced with real implementation
- [ ] Unit tests verify stub federation
- [ ] Integration tests verify cross-domain coordination

## Dependencies

- #034: Federation Domain Model (needs domains)
- #036: Inter-Domain Path Computation (uses federation protocol)

## Related Issues

- #036: Inter-Domain Path Computation (uses this protocol)

## Notes

Federation protocol enables real multi-domain coordination. For Scope 5, stubbed implementation is sufficient but protocol should be complete and documented.

## Stub Behavior & Workflow

- **Invocation chain**: `FindFederatedPath` now accepts an optional `FederationClient`. The scheduler/pathfinding helper calls `FindFederatedPathWithClient` using the `InMemoryFederationClient` stub while tests inject `testFederationClient` to assert requests.
- **Request/response contract**: `FederationRequest` carries `RequestID`, `SourceDomain`, `DestDomain`, `Requirements`, and `Token`. The stub replies with `FederationResponse` containing `Status`, `PathSegment`, and optional `Error`. A missing token yields `Status=error` and `Error="missing token"`; valid requests return a `PathSegment` rooted at `border-<destDomain>`.
- **Logging / Diagnostics**: The stub logs each request with structured `source`/`dest` fields through `logging.String` helpers so CI-friendly logging fields can be added in future gRPC client implementations.
- **Integration expectations**: `FindFederatedPathWithClient` builds the final `FederatedPath` by merging the source-domain segment (simple single-node hop) with the segment returned by the federation client. The scheduler/pathfinding logic should call this helper when `ServiceRequest.CrossDomain` is true and supply the current `FederationClient` (stub by default, real gRPC client later).
