# Issue #009: Implement Time-Expanded Graph Construction

**Labels:** `scope5`, `multihop`, `pathfinding`, `graph-algorithms`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High

## Description

Build a time-expanded graph representation of the network where nodes exist at multiple time steps. This enables pathfinding algorithms to find paths that account for time-varying connectivity.

## Tasks

1. **Define time-expanded graph types** in `internal/sbi/controller/pathfinding.go`:
   ```go
   type TimeExpandedNode struct {
       NodeID string
       Time   time.Time
   }

   type TimeExpandedEdge struct {
       From   TimeExpandedNode
       To     TimeExpandedNode
       LinkID string
       Cost   int
   }

   type TimeExpandedGraph struct {
       Nodes []TimeExpandedNode
       Edges []TimeExpandedEdge
   }
   ```

2. **Implement graph construction**:
   ```go
   func BuildTimeExpandedGraph(srcNodeID, dstNodeID string, startTime, endTime time.Time) (*TimeExpandedGraph, error)
   ```

3. **Graph construction logic**:
   - Create nodes at each time step where contacts are available
   - Create edges representing links active during specific time windows (from #008)
   - Add "wait" edges (same node, different time) for DTN/store-and-forward
   - Set edge costs based on latency, hops, or other metrics

4. **Time discretization**:
   - Decide on time step granularity (e.g., 1 second, 10 seconds)
   - Balance between accuracy and graph size
   - Make configurable

5. **Graph optimization**:
   - Prune unreachable nodes early
   - Cache graph structure for reuse
   - Support incremental updates

## Acceptance Criteria

- [ ] TimeExpandedNode and TimeExpandedEdge types are defined
- [ ] TimeExpandedGraph structure is defined
- [ ] BuildTimeExpandedGraph constructs graph correctly
- [ ] Graph includes nodes at appropriate time steps
- [ ] Graph includes edges for active contact windows
- [ ] Graph includes wait edges for DTN
- [ ] Edge costs are computed correctly
- [ ] Unit tests verify graph construction
- [ ] Unit tests verify graph structure correctness
- [ ] Performance tests verify graph construction time

## Dependencies

- #008: Contact Window Data Structure (needs contact windows to build graph)

## Related Issues

- #010: Multi-Hop Pathfinding Algorithm (will use this graph)
- #014: DTN Storage & Store-and-Forward (will use wait edges)

## Notes

The time-expanded graph is the data structure that enables time-aware pathfinding. It represents the network topology across time, allowing algorithms to find paths that account for when links are available.

