# Issue #034: Implement Federation Domain Model

**Labels:** `scope5`, `federation`, `domain`, `multi-domain`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** Low

## Description

Define scheduling domains for federation scenarios. Domains represent different operators or regions that coordinate to fulfill cross-domain ServiceRequests.

## Tasks

1. **Define SchedulingDomain type** in `model/domain.go` or appropriate location:
   ```go
   type SchedulingDomain struct {
       DomainID     string
       Name         string
       Nodes        []string // nodes in this domain
       Capabilities map[string]interface{} // domain-specific capabilities
       FederationEndpoint string // gRPC endpoint for inter-domain coordination
   }
   ```

2. **Add domain management to ScenarioState** in `internal/sim/state/state.go`:
   - `CreateDomain(domain *SchedulingDomain) error`
   - `GetDomain(domainID string) (*SchedulingDomain, error)`
   - `GetDomain(nodeID string) (string, error)` // which domain owns a node
   - `ListDomains() []*SchedulingDomain`
   - `DeleteDomain(domainID string) error`

3. **Domain-node association**:
   - Track which nodes belong to which domain
   - Support node-to-domain lookup
   - Validate domain assignments

4. **Domain validation**:
   - Validate domain IDs are unique
   - Validate nodes exist
   - Validate federation endpoints (if provided)

5. **Thread-safe operations**:
   - All domain operations must be thread-safe

## Acceptance Criteria

- [ ] SchedulingDomain type is defined correctly
- [ ] ScenarioState can create, get, list, and delete domains
- [ ] GetDomain(nodeID) returns correct domain
- [ ] Domain-node associations are tracked correctly
- [ ] Domain validation prevents invalid domains
- [ ] All operations are thread-safe
- [ ] Unit tests verify domain CRUD operations
- [ ] Unit tests verify domain-node associations
- [ ] Integration tests verify domains in realistic scenarios

## Dependencies

- None (foundation issue)

## Related Issues

- #035: Inter-Domain ServiceRequest Model (will use domains)
- #036: Inter-Domain Path Computation (will use domains)

## Notes

Federation domains enable multi-operator scenarios. For Scope 5, this can be stubbed (in-memory coordination) but the model should be complete.

## Documentation & Testing Additions

- Extend the domain model documentation with examples of node associations and domain validation checks (see `internal/sim/state/state_domain_test.go` for baseline coverage).
- Add integration-style tests ensuring domain deletion frees node assignments so they can be reused by subsequent domains (we now have `TestScenarioStateDomainReassignment` in `state_domain_test.go`).
- Mention the need to keep service request or scheduler components aware of node-to-domain mapping once the higher-level federation work begins.
