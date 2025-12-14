# Issue #020: Implement DTN Storage Model

**Labels:** `scope5`, `dtn`, `storage`, `store-and-forward`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** Medium

## Description

Add per-node storage capacity model for Delay-Tolerant Networking (DTN). This enables store-and-forward routing when end-to-end paths are not immediately available.

## Tasks

1. **Define DTNStorage type** in `internal/sim/state/state.go` or new file:
   ```go
   type DTNStorage struct {
       NodeID        string
       CapacityBytes uint64
       UsedBytes     uint64
       Messages      []StoredMessage
       mu            sync.RWMutex
   }
   ```

2. **Define StoredMessage type**:
   ```go
   type StoredMessage struct {
       MessageID    string
       ServiceRequestID string
       SizeBytes    uint64
       ArrivalTime  time.Time
       ExpiryTime   time.Time
       Destination  string
   }
   ```

3. **Add DTN storage to ScenarioState**:
   - `StoreMessage(nodeID string, msg StoredMessage) error`
   - `RetrieveMessage(nodeID string, msgID string) (*StoredMessage, error)`
   - `GetStorageUsage(nodeID string) (used, capacity uint64)`
   - `EvictExpiredMessages(nodeID string, now time.Time) error`

4. **Storage management**:
   - Track storage per node
   - Check capacity before storing
   - Handle storage full scenarios (evict or reject)
   - Thread-safe operations

5. **Message lifecycle tracking**:
   - Track message state (pending, stored, forwarded, expired)
   - Update state as messages move through network
   - Clean up expired messages

## Acceptance Criteria

- [ ] DTNStorage type is defined correctly
- [ ] StoredMessage type captures all required fields
- [ ] StoreMessage correctly stores messages
- [ ] RetrieveMessage correctly retrieves messages
- [ ] GetStorageUsage returns accurate usage
- [ ] EvictExpiredMessages removes expired messages
- [ ] Storage capacity is enforced
- [ ] Unit tests verify storage operations
- [ ] Unit tests verify capacity enforcement
- [ ] Integration tests verify message lifecycle

## Dependencies

- None (foundation issue)

## Related Issues

- #021: Store-and-Forward Path Computation (will use storage)
- #022: Storage-Aware Scheduling (will schedule storage operations)

## Notes

DTN storage enables messages to wait at intermediate nodes until next contact windows open. This is essential for delay-tolerant networking scenarios.

