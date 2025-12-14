# Issue #005: Implement Priority Queue for ServiceRequest Scheduling

**Labels:** `scope5`, `scheduling`, `priority`, `queue`

**Milestone:** Scope 5 - Advanced Scheduling

**Priority:** High

## Description

Implement a priority queue data structure to sort ServiceRequests by priority. This enables the scheduler to process higher-priority requests first when multiple ServiceRequests contend for limited resources.

## Tasks

1. **Create PriorityQueue type** in `internal/sbi/controller/scheduler.go`:
   ```go
   type PriorityQueue struct {
       items []*ServiceRequest
       mu    sync.Mutex
   }
   ```

2. **Implement priority queue methods**:
   - `Push(sr *ServiceRequest)` - add ServiceRequest to queue
   - `Pop() *ServiceRequest` - remove and return highest priority item
   - `SortByPriority()` - sort items by priority (higher first)
   - `Len() int` - return queue length
   - `Peek() *ServiceRequest` - return highest priority without removing

3. **Priority comparison logic**:
   - Higher priority value = higher priority (e.g., priority 10 > priority 5)
   - Handle edge cases (equal priorities, negative priorities)

4. **Thread-safety**:
   - All operations must be thread-safe
   - Use mutex to protect internal state

5. **Integration with scheduler**:
   - Replace simple ServiceRequest list with PriorityQueue
   - Ensure scheduler processes items in priority order

## Acceptance Criteria

- [ ] PriorityQueue type is implemented with all required methods
- [ ] Push correctly adds items maintaining priority order
- [ ] Pop returns highest priority item
- [ ] SortByPriority correctly orders by priority (descending)
- [ ] All operations are thread-safe
- [ ] Unit tests verify priority ordering
- [ ] Unit tests verify thread-safety
- [ ] Integration with scheduler maintains priority order

## Dependencies

- None (can be implemented independently)

## Related Issues

- #006: Bandwidth-aware allocation (will use priority queue)
- #007: Preemption logic (will use priority queue to identify conflicts)

## Notes

This is the foundation for priority-based scheduling. The queue will be used throughout the scheduler to ensure higher-priority ServiceRequests are processed first.

