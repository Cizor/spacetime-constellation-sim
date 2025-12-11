# Duplicate Issues Consolidation Plan

## Analysis of Potential Duplicates

### Chunk 8: Basic Scheduling Engine

#### Issue Pair: #161 & #162
- **#161**: [Scope 4][Chunk 8] Implement static route scheduling for single-hop links
- **#162**: [Scope 4][Chunk 8] Add static single-hop routes based on link availability

**Analysis:**
- Both address the same requirement (8.3: Static routes for single-hop paths)
- Likely duplicates or very similar scope
- **Recommendation:** Review both issues. If #162 is more specific (link availability), keep #162 and close #161. If #161 is more comprehensive, keep #161.

#### Issue Pair: #163 & #164
- **#163**: [Scope 4][Chunk 8] Implement minimal ServiceRequest-aware scheduling in controller
- **#164**: [Scope 4][Chunk 8] Implement minimal ServiceRequest-aware scheduling in controller

**Analysis:**
- **EXACT DUPLICATE** - Same title
- **Recommendation:** Close one (preferably #164) and keep #163. Update #163 if needed to ensure it covers all aspects.

---

### Chunk 9: Wiring into Scenario Lifecycle

#### Issue Pair: #166 & #167
- **#166**: [Scope 4][Chunk 9] Wire Scenario Startup: Instantiate Scheduler, CDPI Server, Agents
- **#167**: [Scope 4][Chunk 9] Wire SBI components into simulator scenario startup

**Analysis:**
- Both address requirement 9.1 (Scenario startup flow)
- #166 is more specific (lists components)
- #167 is more general
- **Recommendation:** Keep #166 (more specific), close #167, or merge content from #167 into #166 if it has unique details.

#### Issue Pair: #168 & #169
- **#168**: [Scope 4][Chunk 9] Implement simulation run loop & EventScheduler integration
- **#169**: [Scope 4][Chunk 9] Integrate EventScheduler & SBI into main simulation loop

**Analysis:**
- Both address requirement 9.2 (Run loop integration)
- Very similar scope
- **Recommendation:** Keep #168 (more specific about EventScheduler), close #169, or merge if #169 has unique SBI integration details.

---

### Chunk 12: Observability

#### Issue Pair: #179 & #180
- **#179**: [Scope 4][Chunk 12] Add structured logging for CDPI, agents, scheduler
- **#180**: [Scope 4][Chunk 12] Add structured logging for CDPI server and agents

**Analysis:**
- Both address requirement 12.1 (Structured logging)
- #179 includes scheduler, #180 doesn't
- **Recommendation:** Keep #179 (more comprehensive), close #180, or update #179 to explicitly mention it covers all three components.

#### Issue Pair: #181 & #182
- **#181**: [Scope 4][Chunk 12] Add minimal SBI metrics counters for controller and agents
- **#182**: [Scope 4][Chunk 12] Add in-memory SBI metrics counters for CDPI, Agents, Scheduler

**Analysis:**
- Both address requirement 12.2 (Minimal metrics hooks)
- #182 is more specific (in-memory, includes Scheduler)
- **Recommendation:** Keep #182 (more comprehensive), close #181, or merge content if #181 has unique details.

---

## Consolidation Actions

### Immediate Actions (Clear Duplicates)

1. **Issue #164** → Close as duplicate of #163
   - Action: Close #164, add comment referencing #163

2. **Issue #180** → Close as duplicate of #179 (or merge into #179)
   - Action: Review #180 for unique content, then close or merge

3. **Issue #181** → Close as duplicate of #182 (or merge into #182)
   - Action: Review #181 for unique content, then close or merge

### Review Required (Potential Overlaps)

4. **Issues #161 & #162** → Review and consolidate
   - Action: Read both issue descriptions
   - If duplicates: Keep the more detailed one
   - If complementary: Update titles/descriptions to clarify scope

5. **Issues #166 & #167** → Review and consolidate
   - Action: Read both issue descriptions
   - If duplicates: Keep #166 (more specific)
   - If complementary: Merge or clarify scope

6. **Issues #168 & #169** → Review and consolidate
   - Action: Read both issue descriptions
   - If duplicates: Keep #168 (more specific)
   - If complementary: Merge or clarify scope

---

## Verification Checklist

After consolidation, verify:
- [ ] All planning requirements still covered
- [ ] No functionality gaps introduced
- [ ] Epic issues (#159, #165, #178) still properly track chunks
- [ ] Dependencies between issues remain clear
- [ ] Issue descriptions updated if merged

---

## Script to Help Review

You can use this to fetch issue details for comparison:

```powershell
# Fetch issue pairs for comparison
$pairs = @(
    @(161, 162),
    @(163, 164),
    @(166, 167),
    @(168, 169),
    @(179, 180),
    @(181, 182)
)

foreach ($pair in $pairs) {
    Write-Host "`n=== Comparing #$($pair[0]) vs #$($pair[1]) ==="
    $issue1 = Invoke-RestMethod -Uri "https://api.github.com/repos/Cizor/spacetime-constellation-sim/issues/$($pair[0])"
    $issue2 = Invoke-RestMethod -Uri "https://api.github.com/repos/Cizor/spacetime-constellation-sim/issues/$($pair[1])"
    
    Write-Host "Issue #$($pair[0]): $($issue1.title)"
    Write-Host "Body length: $($issue1.body.Length) chars"
    Write-Host "Issue #$($pair[1]): $($issue2.title)"
    Write-Host "Body length: $($issue2.body.Length) chars"
    
    Start-Sleep -Seconds 1  # Rate limit
}
```




