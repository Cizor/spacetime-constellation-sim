# Issue #035: Extend ServiceRequest Model for Cross-Domain Support

**Labels:** `scope5`, `federation`, `servicerequest`, `cross-domain`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** Low

## Description

Extend ServiceRequest model to support cross-domain scenarios. Track source/destination domains and federation tokens for inter-domain coordination.

## Tasks

1. **Extend ServiceRequest** in `model/servicerequest.go`:
   ```go
   type ServiceRequest struct {
       // existing fields...
       CrossDomain   bool
       SourceDomain  string
       DestDomain    string
       FederationToken string // for authentication
   }
   ```

2. **Cross-domain detection**:
   - Detect if SR spans multiple domains
   - Set CrossDomain = true if source and dest domains differ
   - Validate domains exist

3. **Federation token**:
   - Generate or accept federation token
   - Use for inter-domain authentication (future)
   - Store with SR

4. **Validation logic**:
   - If CrossDomain = true, validate SourceDomain and DestDomain
   - Validate domains exist
   - Ensure federation token is present (if required)

5. **Backward compatibility**:
   - Existing single-domain SRs continue to work
   - CrossDomain defaults to false
   - Optional fields for federation

## Acceptance Criteria

- [ ] ServiceRequest supports cross-domain fields
- [ ] CrossDomain is set correctly based on domains
- [ ] Federation token is stored
- [ ] Validation checks cross-domain requirements
- [ ] Backward compatibility maintained
- [ ] Unit tests verify cross-domain SR creation
- [ ] Unit tests verify validation logic
- [ ] Integration tests verify cross-domain SRs

## Dependencies

- #034: Federation Domain Model (needs domains)

## Related Issues

- #036: Inter-Domain Path Computation (will use cross-domain SRs)

## Notes

Cross-domain SRs enable federation scenarios. For Scope 5, the model is complete but coordination can be stubbed.

## Documentation & Testing Notes

- Mention how `ScenarioState.CreateServiceRequest` now populates `SourceDomain`/`DestDomain` from node assignments and enforces federation token requirements when domains differ.  
- The new coverage in `internal/sim/state/state_domain_test.go` demonstrates cross-domain creation and token validation; reference it for future integration testing.  
