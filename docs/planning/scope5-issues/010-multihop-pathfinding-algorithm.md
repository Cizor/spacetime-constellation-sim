# Issue #010: Implement Multi-Hop Pathfinding Algorithm

**Labels:** `scope5`, `multihop`, `pathfinding`, `algorithm`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High

## Description

Implement a time-aware shortest path algorithm that finds multi-hop paths between source and destination nodes, considering time-varying link availability and contact windows.

## Tasks

1. **Define Path and PathHop types** in `internal/sbi/controller/pathfinding.go`:
   ```go
   type Path struct {
       Hops []PathHop
       TotalLatency time.Duration
       ValidFrom    time.Time
       ValidUntil   time.Time
   }

   type PathHop struct {
       FromNodeID string
       ToNodeID   string
       LinkID     string
       StartTime  time.Time
       EndTime    time.Time
   }
   ```

2. **Implement pathfinding function**:
   ```go
   func FindMultiHopPath(srcNodeID, dstNodeID string, startTime time.Time, horizon time.Duration) (*Path, error)
   ```

3. **Algorithm implementation**:
   - Use Dijkstra's algorithm on time-expanded graph (from #009)
   - Or A* with time-aware heuristics
   - Consider multiple cost metrics:
     - Path length (number of hops)
     - Total latency
     - Contact window overlap
     - Link quality

4. **Path validation**:
   - Ensure all hops have valid contact windows
   - Verify path continuity (each hop's destination = next hop's source)
   - Check time ordering (hops occur in sequence)

5. **Return best path**:
   - If multiple paths exist, return shortest/lowest cost
   - If no path exists, return appropriate error
   - Include path metadata (latency, quality, etc.)

## Acceptance Criteria

- [ ] Path and PathHop types are defined correctly
- [ ] FindMultiHopPath finds valid paths when they exist
- [ ] Algorithm considers time-varying connectivity
- [ ] Algorithm returns shortest/lowest cost path
- [ ] Algorithm handles cases where no path exists
- [ ] Path validation ensures correctness
- [ ] Unit tests verify pathfinding for simple topologies
- [ ] Unit tests verify pathfinding with time constraints
- [ ] Integration tests verify paths work in realistic scenarios

## Dependencies

- #009: Time-Expanded Graph Construction (needs graph to search)

## Related Issues

- #011: Path Validation and Scheduling (will use computed paths)
- #016: Reactive Re-Planning (will recompute paths)

## Notes

This is the core pathfinding algorithm for multi-hop routing. It must be efficient and correct, as it will be called frequently by the scheduler.

