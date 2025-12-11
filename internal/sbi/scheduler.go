package sbi

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/signalsfoundry/constellation-simulator/timectrl"
)

// EventScheduler schedules callbacks to run at specific simulation times
// based on a SimClock implementation. This is used by agents and the controller
// scheduler to execute time-based actions (beam updates, route changes, telemetry, etc.).
//
// The main simulation loop (wired in Chunk 9) will:
// - Advance the simulation time using the time controller.
// - Call RunDue() on the EventScheduler after each time advance.
//
// Agents and the controller-side scheduler (Chunks 4 and 8) will:
// - Use Schedule / Cancel to manage time-based actions (beam updates, routes, telemetry, etc.).
type EventScheduler interface {
	// Schedule registers a callback f to run at simulation time 'at'.
	// It returns an opaque event ID that can be used to cancel the event.
	Schedule(at time.Time, f func()) (id string)

	// Cancel attempts to cancel a previously scheduled event.
	// It is a no-op if the ID is unknown or the event already ran.
	Cancel(id string)

	// Now returns the current simulation time, usually delegated to the underlying SimClock.
	Now() time.Time

	// RunDue executes all events whose scheduled time is <= Now().
	// It should be safe to call multiple times; already-run events must not run again.
	RunDue()
}

// scheduledEvent represents a single scheduled callback.
type scheduledEvent struct {
	id        string
	when      time.Time
	f         func()
	cancelled bool
}

// eventScheduler is a concrete implementation of EventScheduler that uses SimClock
// to determine current simulation time and stores events ordered by scheduled time.
type eventScheduler struct {
	clock timectrl.SimClock

	mu      sync.Mutex
	counter uint64
	events  []*scheduledEvent // ordered by 'when' (earliest first)
	index   map[string]*scheduledEvent
}

// NewEventScheduler creates a new event scheduler backed by the given SimClock.
// Other components (agents, controller scheduler, main runner) will call this with either:
// - The real TimeController (implementing SimClock) in normal runs.
// - A fake SimClock in unit tests.
func NewEventScheduler(clock timectrl.SimClock) EventScheduler {
	return &eventScheduler{
		clock: clock,
		index: make(map[string]*scheduledEvent),
	}
}

// Schedule registers a callback to run at the specified simulation time.
func (s *eventScheduler) Schedule(at time.Time, f func()) (id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.counter++
	id = fmt.Sprintf("ev-%d", s.counter)

	ev := &scheduledEvent{
		id:   id,
		when: at,
		f:    f,
	}

	// Insert into events slice in time order (earliest first).
	s.addEventLocked(ev)

	s.index[id] = ev

	return id
}

// addEventLocked inserts an event into the events slice maintaining time order.
// Caller must hold s.mu lock.
func (s *eventScheduler) addEventLocked(ev *scheduledEvent) {
	// Find insertion point using binary search
	idx := sort.Search(len(s.events), func(i int) bool {
		return !s.events[i].when.Before(ev.when)
	})

	// Insert at idx
	s.events = append(s.events, nil)
	copy(s.events[idx+1:], s.events[idx:])
	s.events[idx] = ev
}

// Cancel attempts to cancel a previously scheduled event.
func (s *eventScheduler) Cancel(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.index == nil {
		return
	}

	ev, ok := s.index[id]
	if !ok {
		return
	}

	ev.cancelled = true
	delete(s.index, id)
	// Actual removal from s.events can be lazy; RunDue will skip cancelled events.
}

// Now returns the current simulation time from the underlying clock.
func (s *eventScheduler) Now() time.Time {
	return s.clock.Now()
}

// peekNextLocked returns the next event to run (earliest non-cancelled event)
// without removing it. Returns nil if no events are due.
// Caller must hold s.mu lock.
func (s *eventScheduler) peekNextLocked() *scheduledEvent {
	now := s.clock.Now()
	for _, ev := range s.events {
		if ev.cancelled {
			continue
		}
		if !ev.when.After(now) {
			return ev
		}
		// Events are ordered by time, so if this one is in the future, all later ones are too
		break
	}
	return nil
}

// popNextLocked removes and returns the next event to run (earliest non-cancelled event).
// Returns nil if no events are due.
// Caller must hold s.mu lock.
func (s *eventScheduler) popNextLocked() *scheduledEvent {
	now := s.clock.Now()
	for len(s.events) > 0 {
		ev := s.events[0]
		if ev.cancelled {
			// Remove cancelled event from slice
			s.events = s.events[1:]
			continue
		}
		if !ev.when.After(now) {
			// Remove this event from slice
			s.events = s.events[1:]
			return ev
		}
		// Events are ordered by time, so if this one is in the future, all later ones are too
		break
	}
	return nil
}

// RunDue executes all events whose scheduled time is <= Now().
// It is safe to call multiple times; already-run events will not run again.
func (s *eventScheduler) RunDue() {
	for {
		s.mu.Lock()
		ev := s.peekNextLocked()
		if ev == nil {
			s.mu.Unlock()
			return
		}

		// Remove from events
		ev = s.popNextLocked()
		if ev == nil {
			s.mu.Unlock()
			continue
		}

		// Skip cancelled events (shouldn't happen after popNextLocked, but be safe)
		if ev.cancelled {
			s.mu.Unlock()
			continue
		}

		if s.index != nil {
			delete(s.index, ev.id)
		}
		s.mu.Unlock()

		// Execute callback OUTSIDE the lock to avoid deadlocks and allow re-entrancy.
		if ev.f != nil {
			ev.f()
		}
	}
}

