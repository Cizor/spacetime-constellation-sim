# Issue #022: Implement Storage-Aware Scheduling for DTN Paths

**Labels:** `scope5`, `dtn`, `scheduling`, `store-and-forward`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** Medium

## Description

When scheduling DTN paths, schedule StoreMessage and ForwardMessage actions at storage nodes. Handle storage capacity constraints and message eviction.

## Tasks

1. **Schedule StoreMessage actions**:
   - When message arrives at storage node
   - Schedule StoreMessage action at arrival time
   - Check storage capacity before scheduling

2. **Schedule ForwardMessage actions**:
   - When next contact window opens
   - Schedule ForwardMessage action at contact start time
   - Remove message from storage after forwarding

3. **Storage capacity handling**:
   - If node storage full → evict oldest/lowest-priority messages
   - Or reject new message if no space available
   - Log storage capacity issues

4. **Message lifecycle scheduling**:
   - Track message state transitions
   - Schedule state updates (stored → in_transit → delivered)
   - Handle message expiry (schedule expiration check)

5. **Integration with path scheduling**:
   - DTN paths include storage actions
   - Storage actions are scheduled alongside beam/route actions
   - Use same event scheduler

## Acceptance Criteria

- [ ] StoreMessage actions are scheduled correctly
- [ ] ForwardMessage actions are scheduled correctly
- [ ] Storage capacity is checked before storing
- [ ] Message eviction works when storage is full
- [ ] Message lifecycle is tracked and scheduled
- [ ] Unit tests verify storage scheduling
- [ ] Unit tests verify capacity handling
- [ ] Integration tests verify end-to-end DTN scheduling

## Dependencies

- #020: DTN Storage Model (needs storage operations)
- #021: Store-and-Forward Path Computation (needs DTN paths)

## Related Issues

- #023: Message Lifecycle Tracking (will track message states)

## Notes

Storage-aware scheduling ensures DTN messages are properly stored and forwarded at the right times. This completes the DTN functionality.

