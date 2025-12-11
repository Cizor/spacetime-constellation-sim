# Scope 4 Issues - Action Plan

## ✅ Summary

**Status:** All 13 chunks from planning document are covered by GitHub issues.

**Total Issues:** 77 Scope 4 issues (119-196)

**Missing Issues:** 2 (ready to create)
**Duplicates:** 6 pairs to review/consolidate

---

## 📋 Part 1: Create Missing Issues

### Issue 1: Agent Schedule Execution Tests
- **Title:** `[Scope 4][Chunk 10] Add agent schedule execution unit tests`
- **Covers:** Planning requirement 10.1
- **File:** `missing_issues.json` (first entry)
- **Labels:** `scope:4-sbi`, `chunk:10-scope4-tests`, `type:test`

### Issue 2: DumpAgentState Helper
- **Title:** `[Scope 4][Chunk 12] Add DumpAgentState developer helper function`
- **Covers:** Planning requirement 12.3
- **File:** `missing_issues.json` (second entry)
- **Labels:** `scope:4-sbi`, `chunk:12-scope4-observability`, `type:feature`

**To Create:**
```powershell
# Option 1: Use the script (requires GitHub PAT)
.\create_missing_issues.ps1 -Token "your_github_pat_token"

# Option 2: Manual creation via GitHub web UI
# Copy content from missing_issues.json
```

---

## 🔄 Part 2: Consolidate Duplicates

### Clear Duplicates (Close One)

#### 1. Issue #164 → Close as duplicate of #163
- **Reason:** Exact duplicate title
- **Action:** Close #164, add comment: "Duplicate of #163"

#### 2. Issue #180 → Close as duplicate of #179 (or merge)
- **Reason:** #179 includes scheduler, #180 doesn't
- **Action:** Review #180 for unique content, then close or merge into #179

#### 3. Issue #181 → Close as duplicate of #182 (or merge)
- **Reason:** #182 is more specific (in-memory, includes Scheduler)
- **Action:** Review #181 for unique content, then close or merge into #182

### Review Required (Compare Descriptions)

#### 4. Issues #161 & #162
- **Topic:** Static route scheduling for single-hop links
- **Action:** 
  - Read both descriptions
  - If duplicates: Keep the more detailed one
  - If complementary: Update titles to clarify scope

#### 5. Issues #166 & #167
- **Topic:** Scenario startup wiring
- **Action:**
  - #166 is more specific (lists components)
  - Likely keep #166, close #167
  - Or merge if #167 has unique details

#### 6. Issues #168 & #169
- **Topic:** Run loop integration
- **Action:**
  - #168 is more specific about EventScheduler
  - Likely keep #168, close #169
  - Or merge if #169 has unique SBI integration details

---

## 🛠️ Tools Created

1. **missing_issues.json** - Ready-to-use JSON for 2 new issues
2. **create_missing_issues.ps1** - Script to create issues via API
3. **review_duplicates.ps1** - Script to compare duplicate pairs
4. **duplicate_consolidation_plan.md** - Detailed analysis
5. **scope4_review.md** - Complete chunk-by-chunk review

---

## 📝 Next Steps

### Immediate Actions:

1. **Create Missing Issues:**
   ```powershell
   # If you have GitHub PAT:
   .\create_missing_issues.ps1 -Token "your_token"
   
   # Or manually create via GitHub web UI using missing_issues.json
   ```

2. **Review Duplicates:**
   ```powershell
   # With token (for higher rate limit):
   .\review_duplicates.ps1 -Token "your_token"
   
   # Or manually review in GitHub:
   # - #163 vs #164
   # - #179 vs #180
   # - #181 vs #182
   # - #161 vs #162
   # - #166 vs #167
   # - #168 vs #169
   ```

3. **Close Duplicates:**
   - After reviewing, close the less detailed/duplicate issues
   - Add comments referencing the kept issue

---

## ✅ Verification After Actions

- [ ] 2 missing issues created
- [ ] 3-6 duplicate issues closed/merged
- [ ] All planning requirements still covered
- [ ] Epic issues properly track chunks
- [ ] Dependencies remain clear

---

## 📊 Final Status

After completing actions:
- **Total Issues:** 77 → ~73-75 (after consolidation)
- **Coverage:** 100% of planning requirements ✅
- **Ready for Implementation:** Yes ✅




