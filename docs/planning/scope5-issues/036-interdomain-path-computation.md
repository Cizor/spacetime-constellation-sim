# Issue #036: Implement Inter-Domain Path Computation

**Labels:** `scope5`, `federation`, `pathfinding`, `multi-domain`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** Low

## Description

For cross-domain ServiceRequests, compute paths that span multiple domains. Coordinate with other domain schedulers (stubbed for Scope 5) to combine path segments.

## Tasks

1. **Define FederatedPath type**:
   ```go
   type FederatedPath struct {
       Segments []PathSegment
       DomainHops []string // [domain1, domain2, domain3]
   }

   type PathSegment struct {
       DomainID string
       Path     *Path // path within this domain
       BorderNodes []string // nodes that connect to other domains
   }
   ```

2. **Implement federated pathfinding**:
   ```go
   func FindFederatedPath(sr *ServiceRequest) (*FederatedPath, error)
   ```

3. **Within-domain path computation**:
   - Compute best path segment within each domain
   - Identify border nodes (nodes that can connect to other domains)
   - Return path segment with border nodes

4. **Between-domain coordination** (stubbed):
   - Find border node pairs that can connect
   - Coordinate with other domain's scheduler (via federation endpoint)
   - For Scope 5: in-memory coordination or stub
   - Combine path segments

5. **Path assembly**:
   - Combine segments from all domains
   - Ensure continuity at border nodes
   - Return complete federated path

## Acceptance Criteria

- [ ] FederatedPath type represents multi-domain paths
- [ ] FindFederatedPath computes paths spanning domains
- [ ] Within-domain segments are computed correctly
- [ ] Border nodes are identified correctly
- [ ] Path segments are combined correctly
- [ ] Stubbed coordination works (for Scope 5)
- [ ] Unit tests verify federated pathfinding
- [ ] Unit tests verify segment combination
- [ ] Integration tests verify cross-domain paths

## Dependencies

- #034: Federation Domain Model (needs domains)
- #035: Inter-Domain ServiceRequest Model (needs cross-domain SRs)
- #010: Multi-Hop Pathfinding Algorithm (needs base pathfinding)

## Related Issues

- #037: Federation Protocol (stub) (defines coordination protocol)

## Notes

Inter-domain pathfinding enables federation. For Scope 5, coordination can be stubbed (in-memory) but the protocol should be defined.

## Testing & Documentation Notes

- The stubbed `FindFederatedPath` implementation lives in `internal/sbi/controller/federation_path.go`; tests in `federation_path_test.go` verify cross-domain and single-domain behavior, mentioning `State.CreateDomain` + node setup.
- Document the simple combination of per-domain segments, border node tracking, and domain hops so future work can replace the stub with real coordination.
