# Issue #023: Implement Message Lifecycle Tracking for DTN

**Labels:** `scope5`, `dtn`, `lifecycle`, `tracking`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** Low

## Description

Track message state as messages move through the DTN network. Update state from pending → in_transit → stored → delivered or expired.

## Tasks

1. **Define MessageState type**:
   ```go
   type MessageState string
   const (
       MessagePending MessageState = "pending"
       MessageInTransit MessageState = "in_transit"
       MessageStored MessageState = "stored"
       MessageDelivered MessageState = "delivered"
       MessageExpired MessageState = "expired"
   )
   ```

2. **Extend StoredMessage** (from #020):
   - Add `State MessageState` field
   - Add state transition timestamps

3. **State transition logic**:
   - Update state as message moves through network
   - Pending → InTransit (when sent)
   - InTransit → Stored (when arrives at storage node)
   - Stored → InTransit (when forwarded)
   - InTransit → Delivered (when reaches destination)
   - Any → Expired (when expiry time reached)

4. **State update scheduling**:
   - Schedule state updates at appropriate times
   - Use event scheduler for state transitions
   - Handle expiry checks

5. **State query API**:
   - Query message state by message ID
   - Query all messages in a state
   - Track state history

## Acceptance Criteria

- [ ] MessageState type is defined correctly
- [ ] StoredMessage includes state field
- [ ] State transitions occur at correct times
- [ ] State updates are scheduled correctly
- [ ] Expiry is handled correctly
- [ ] State query API works
- [ ] Unit tests verify state transitions
- [ ] Unit tests verify expiry handling
- [ ] Integration tests verify message lifecycle

## Dependencies

- #020: DTN Storage Model (needs StoredMessage type)
- #022: Storage-Aware Scheduling (schedules state updates)

## Related Issues

- #020, #021, #022: All DTN issues (related)

## Notes

Message lifecycle tracking provides observability into DTN message flow. It's useful for debugging and monitoring DTN operations.

