# Scope 4 Implementation Workflow

## Overview

This document outlines the recommended workflow for implementing Scope 4 issues systematically, ensuring proper dependency management, testing, and code quality.

## Workflow Strategy

### Recommended Approach: **Sequential Chunk-by-Chunk with Branch-per-Issue**

**Why this approach?**
- ✅ Respects dependencies (Chunk 0 → 1 → 2 → ... → 13)
- ✅ Each issue is independently testable
- ✅ Clean git history with clear PRs
- ✅ Easy to track progress
- ✅ Can parallelize within a chunk (if issues are independent)

### Workflow Steps (Per Issue)

1. **Select Issue** (respecting chunk order)
2. **Create Branch** (`issue-<number>-<slug>`)
3. **Implement Changes**
4. **Run Tests** (unit tests, build)
5. **Commit** (with issue reference)
6. **Push Branch**
7. **Create PR** (auto-link to issue)
8. **Review & Merge** (manual or auto-merge)
9. **Close Issue** (via PR merge)
10. **Move to Next Issue**

---

## Implementation Order

### Phase 1: Foundation (Chunks 0-3)
**Must be done sequentially, no parallelization**

- **Chunk 0**: Prep Scope 1-3 base (#119-122)
  - Dependencies: None (builds on Scope 1-3)
  - Can parallelize: #120, #121, #122 (after #119 epic setup)
  
- **Chunk 1**: SBI Protos (#123-126)
  - Dependencies: Chunk 0
  - Can parallelize: #124, #125 (after #123 epic)
  - #126 depends on #124, #125

- **Chunk 2**: Domain Model (#127-131)
  - Dependencies: Chunk 1
  - Can parallelize: #129, #130, #131 (after #127, #128)
  
- **Chunk 3**: Event Scheduling (#132-134)
  - Dependencies: Chunk 2
  - Sequential: #133 → #134

### Phase 2: Core SBI (Chunks 4-7)
**Some parallelization possible**

- **Chunk 4**: Agent Model (#135-139)
  - Dependencies: Chunk 3
  - Sequential: #136 → #137 → #138 → #139

- **Chunk 5**: CDPI Server (#140-143)
  - Dependencies: Chunk 4
  - Sequential: #141 → #142 → #143

- **Chunk 6**: Telemetry (#144-151)
  - Dependencies: Chunk 5
  - Can parallelize: #146, #147, #150 (after #145)
  - #151 depends on #146, #147

- **Chunk 7**: Protocol Completeness (#152-158)
  - Dependencies: Chunk 6
  - Can parallelize: #153, #154, #155, #156, #157, #158 (after #152 epic)

### Phase 3: Integration (Chunks 8-9)
**Sequential**

- **Chunk 8**: Scheduling Engine (#159-163)
  - Dependencies: Chunk 7
  - Sequential: #160 → #162 → #163

- **Chunk 9**: Lifecycle Integration (#165, #167, #168)
  - Dependencies: Chunk 8
  - Sequential: #167 → #168

### Phase 4: Quality & Docs (Chunks 10-13)
**Can parallelize chunks 10-12, but 13 depends on all**

- **Chunk 10**: Unit Tests (#170-175, #197)
  - Dependencies: Chunks 4-9
  - Can parallelize: #171, #172, #173, #175, #197

- **Chunk 11**: gRPC Tests (#174, #176, #177)
  - Dependencies: Chunks 4-9
  - Can parallelize: #176, #177

- **Chunk 12**: Observability (#178-179, #182, #198)
  - Dependencies: Chunks 4-9
  - Can parallelize: #179, #182, #198

- **Chunk 13**: Documentation (#183-196)
  - Dependencies: All previous chunks
  - Can parallelize: Most issues (all are docs)

---

## Branch Naming Convention

```
issue-<number>-<slug>
```

Examples:
- `issue-120-link-status-activation`
- `issue-124-sbi-protos-generation`
- `issue-136-agent-lifecycle`

**Slug rules:**
- Lowercase
- Hyphens for spaces
- Short (max 30 chars)
- Descriptive

---

## Commit Message Format

```
[Scope 4][Chunk X] <Brief description>

Fixes #<issue-number>

<Detailed description if needed>
```

Example:
```
[Scope 4][Chunk 0] Add explicit link status and activation helpers

Fixes #120

- Add LinkStatus enum (Potential, Active, Impaired)
- Add ActivateLink/DeactivateLink helpers to ScenarioState
- Update KnowledgeBase to track link status
- Add unit tests for link activation
```

---

## PR Template

```markdown
## Description
Implements [Scope 4][Chunk X] <issue title>

Fixes #<issue-number>

## Changes
- <Change 1>
- <Change 2>
- ...

## Testing
- [ ] Unit tests pass
- [ ] Integration tests pass (if applicable)
- [ ] Manual testing completed

## Checklist
- [ ] Code follows project style guidelines
- [ ] Tests added/updated
- [ ] Documentation updated (if needed)
- [ ] No breaking changes (or documented)

## Related Issues
Part of epic #<epic-number>
Depends on: #<dependency-issue> (if any)
```

---

## Automation Scripts

### 1. `start-issue.ps1`
- Creates branch from main
- Checks out branch
- Optionally creates initial commit structure

### 2. `finish-issue.ps1`
- Runs tests
- Commits changes
- Pushes branch
- Creates PR via API
- Links PR to issue

### 3. `create-pr.ps1`
- Creates PR from current branch
- Auto-fills template
- Links to issue
- Sets labels

### 4. `check-dependencies.ps1`
- Checks if dependency issues are closed
- Warns if dependencies not met

---

## Testing Strategy

### Per Issue
1. **Unit Tests**: Run `go test ./...` in affected packages
2. **Build**: Ensure `go build ./...` succeeds
3. **Linting**: Run linter (if configured)

### Per Chunk
1. **Integration Tests**: Run chunk-specific integration tests
2. **E2E Tests**: If chunk adds E2E capability

### Before Merging PR
1. All unit tests pass
2. Build succeeds
3. No linter errors
4. Manual review (if not auto-merge)

---

## PR Merge Strategy

### Option A: Auto-Merge (Recommended for Speed)
- Enable auto-merge on PR creation
- Merge when:
  - Tests pass
  - No conflicts
  - Approved (if required)

### Option B: Manual Review (Recommended for Quality)
- Review each PR before merge
- Ensure code quality
- Verify tests
- Check documentation

**Recommendation**: Start with manual review for Chunks 0-3 (foundation), then consider auto-merge for later chunks if quality is consistent.

---

## Issue Tracking

### Linking PRs to Issues
- Use "Fixes #<number>" in commit message
- PR will auto-close issue on merge

### Epic Progress
- Update epic issue with progress
- Link child PRs to epic
- Close epic when all children done

---

## Rollback Strategy

If an issue causes problems:
1. **Revert PR**: Create revert PR
2. **Fix in New Branch**: Create new branch from before problematic PR
3. **Document**: Update issue with what went wrong

---

## Progress Tracking

### Per Chunk
- Track: Issues open, in progress, closed
- Update epic issue with status

### Overall
- Maintain checklist of chunks
- Update main tracking document

---

## Best Practices

1. **One Issue = One PR**: Don't combine multiple issues in one PR
2. **Small, Focused PRs**: Easier to review and test
3. **Test Before PR**: Don't create PR with failing tests
4. **Update Docs**: Update relevant docs with each change
5. **Follow Dependencies**: Don't start issue if dependencies not met
6. **Clean Commits**: Use meaningful commit messages
7. **Rebase Before PR**: Rebase on main before creating PR

---

## Estimated Timeline

Assuming ~2-4 hours per issue:
- **Chunk 0**: 1-2 days (4 issues)
- **Chunk 1**: 1-2 days (4 issues)
- **Chunk 2**: 2-3 days (5 issues)
- **Chunk 3**: 1-2 days (3 issues)
- **Chunk 4**: 2-3 days (5 issues)
- **Chunk 5**: 1-2 days (4 issues)
- **Chunk 6**: 2-3 days (6 issues)
- **Chunk 7**: 2-3 days (7 issues)
- **Chunk 8**: 1-2 days (4 issues)
- **Chunk 9**: 1-2 days (3 issues)
- **Chunk 10**: 2-3 days (6 issues)
- **Chunk 11**: 1-2 days (3 issues)
- **Chunk 12**: 1-2 days (4 issues)
- **Chunk 13**: 3-5 days (14 issues, mostly docs)

**Total**: ~25-40 days of focused work

---

## Next Steps

1. Review and approve this workflow
2. Set up automation scripts
3. Start with Chunk 0, Issue #120
4. Iterate and refine workflow as needed




