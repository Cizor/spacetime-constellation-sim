# Duplicate Issues - Recommendations

## ✅ Already Closed

### Issue #164 → Closed
- **Reason:** Exact duplicate of #163
- **Action Taken:** Closed with comment referencing #163

---

## Recommendations for Remaining Pairs

### 1. Issues #179 & #180 (Structured Logging)

**Analysis:**
- **#179**: "Add structured logging for CDPI, agents, scheduler & telemetry" (12,397 chars)
- **#180**: "Add structured logging for CDPI server and agents" (14,238 chars)

**Recommendation:** 
- **Keep #179** (more comprehensive - includes scheduler & telemetry)
- **Close #180** (less comprehensive, but has longer description - review first to ensure no unique content)

**Action:** Review #180's full description. If it has unique details about CDPI server logging, merge those into #179, then close #180. Otherwise, close #180 as duplicate.

---

### 2. Issues #181 & #182 (Metrics Counters)

**Analysis:**
- **#181**: "Add minimal SBI metrics counters for controller and agents" (10,770 chars)
- **#182**: "Add in-memory SBI metrics counters for CDPI, Agents, and Telemetry" (11,480 chars)

**Recommendation:**
- **Keep #182** (more specific - mentions "in-memory" and includes Telemetry)
- **Close #181** (less comprehensive)

**Action:** Review #181's full description. If it has unique details, merge into #182, then close #181. Otherwise, close #181 as duplicate.

---

### 3. Issues #161 & #162 (Static Route Scheduling)

**Analysis:**
- **#161**: "Implement static route scheduling for single-hop links" (11,523 chars)
- **#162**: "Add static single-hop routes based on link availability in Scheduler" (11,783 chars)

**Recommendation:**
- These appear **complementary** rather than duplicates:
  - #161: General implementation
  - #162: Specific to link availability in Scheduler
- **Action:** Review both descriptions. If they're truly complementary, keep both but update titles to clarify scope. If duplicates, keep the more detailed one.

---

### 4. Issues #166 & #167 (Scenario Startup)

**Analysis:**
- **#166**: "Wire Scenario Startup: Instantiate Scheduler, CDPI Server, Telemetry Server, and Agents" (3,926 chars)
- **#167**: "Wire SBI components into simulator scenario startup" (10,786 chars)

**Recommendation:**
- **#167 is more comprehensive** (longer description)
- **#166 is more specific** (lists exact components)
- **Action:** Review both. Likely #167 covers #166's scope. If so, close #166. If #166 has unique component-specific details, merge into #167 or keep both with clarified scopes.

---

### 5. Issues #168 & #169 (Run Loop Integration)

**Analysis:**
- **#168**: "Implement simulation run loop & EventScheduler integration" (12,644 chars)
- **#169**: "Integrate EventScheduler & SBI into main simulation run loop" (10,557 chars)

**Recommendation:**
- **#168 is more comprehensive** (longer description, more specific about EventScheduler)
- **#169 focuses on SBI integration**
- **Action:** Review both. They might be complementary:
  - #168: EventScheduler integration (core)
  - #169: SBI integration on top (higher-level)
- If complementary, keep both. If duplicates, keep #168.

---

## Summary of Actions

### Immediate (Clear Duplicates):
- ✅ #164 → Closed (duplicate of #163)

### Review Required:
1. **#180** → Review, then close if duplicate of #179
2. **#181** → Review, then close if duplicate of #182
3. **#161/#162** → Review to determine if complementary or duplicate
4. **#166/#167** → Review to determine if complementary or duplicate
5. **#168/#169** → Review to determine if complementary or duplicate

---

## Next Steps

1. Review the full descriptions of #180, #181, #161, #162, #166, #167, #168, #169
2. For each pair, determine if they're:
   - **Duplicates** → Close one, merge content if needed
   - **Complementary** → Keep both, clarify titles/descriptions
3. Update epic issues if needed to reflect consolidated structure




