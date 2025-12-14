# Issue #021: Implement Store-and-Forward Path Computation

**Labels:** `scope5`, `dtn`, `pathfinding`, `store-and-forward`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** Medium

## Description

Extend pathfinding to include storage nodes. Compute paths that use store-and-forward when direct paths don't exist.

## Tasks

1. **Extend time-expanded graph** (from #009):
   - Add "wait" edges at storage-capable nodes
   - Messages can wait at nodes until next contact window
   - Wait edges represent storage duration

2. **Implement DTN pathfinding**:
   ```go
   func FindDTNPath(srcNodeID, dstNodeID string, msgSize uint64, startTime time.Time) (*DTNPath, error)
   ```

3. **Path computation logic**:
   - If direct path exists → use it
   - If not, find path with storage nodes:
     - A → B (store) → wait → B → C (forward)
   - Consider storage capacity at intermediate nodes
   - Consider message expiry time

4. **DTNPath type**:
   ```go
   type DTNPath struct {
       Hops []DTNHop
       StorageNodes []string
       TotalDelay time.Duration
   }

   type DTNHop struct {
       FromNodeID string
       ToNodeID   string
       LinkID     string
       StartTime  time.Time
       EndTime    time.Time
       StorageAt  string // node where message is stored (if any)
       StorageDuration time.Duration
   }
   ```

5. **Integration with existing pathfinding**:
   - Extend FindMultiHopPath to support DTN
   - Or create separate DTN pathfinding function
   - Use same time-expanded graph structure

## Acceptance Criteria

- [ ] Time-expanded graph includes wait edges
- [ ] FindDTNPath finds paths with storage when needed
- [ ] DTNPath type represents store-and-forward paths
- [ ] Storage capacity is considered in pathfinding
- [ ] Message expiry is considered
- [ ] Unit tests verify DTN pathfinding
- [ ] Unit tests verify wait edge handling
- [ ] Integration tests verify DTN paths in realistic scenarios

## Dependencies

- #009: Time-Expanded Graph Construction (needs graph with wait edges)
- #020: DTN Storage Model (needs storage capacity info)

## Related Issues

- #022: Storage-Aware Scheduling (will schedule DTN paths)

## Notes

DTN pathfinding enables routing when direct paths don't exist. Messages can be stored at intermediate nodes until connectivity becomes available.

