package sbi

import (
	"fmt"
	"sync"
	"time"
)

// FakeEventScheduler is a test-only implementation of EventScheduler that maintains
// its own internal notion of simulation time and allows tests to advance time explicitly.
//
// This fake implementation is intended for unit tests of SBI agents, controller scheduler,
// and other components that rely on EventScheduler but should not depend on the real
// time controller. Tests can call AdvanceTo(t) to move fake time forward and execute
// due events deterministically.
type FakeEventScheduler struct {
	mu      sync.Mutex
	now     time.Time
	counter uint64

	// Events ordered by 'when' (earliest first).
	events []*fakeScheduledEvent
	index  map[string]*fakeScheduledEvent
}

type fakeScheduledEvent struct {
	id        string
	when      time.Time
	f         func()
	cancelled bool
}

// NewFakeEventScheduler creates a new fake event scheduler starting at the given time.
func NewFakeEventScheduler(start time.Time) *FakeEventScheduler {
	return &FakeEventScheduler{
		now:    start,
		events: make([]*fakeScheduledEvent, 0),
		index:  make(map[string]*fakeScheduledEvent),
	}
}

// Now returns the current fake simulation time.
func (s *FakeEventScheduler) Now() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.now
}

// Schedule registers a callback to run at the specified simulation time.
func (s *FakeEventScheduler) Schedule(at time.Time, f func()) (id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.counter++
	id = fmt.Sprintf("fake-ev-%d", s.counter)

	ev := &fakeScheduledEvent{
		id:   id,
		when: at,
		f:    f,
	}

	// Insert into events slice in time order (earliest first).
	inserted := false
	for i, existing := range s.events {
		if at.Before(existing.when) {
			s.events = append(s.events[:i], append([]*fakeScheduledEvent{ev}, s.events[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		s.events = append(s.events, ev)
	}

	s.index[id] = ev
	return id
}

// Cancel attempts to cancel a previously scheduled event.
func (s *FakeEventScheduler) Cancel(id string) {
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
	// We keep ev in s.events; RunDue will skip cancelled ones.
}

// RunDue executes all events whose scheduled time is <= now.
func (s *FakeEventScheduler) RunDue() {
	for {
		s.mu.Lock()

		if len(s.events) == 0 {
			s.mu.Unlock()
			return
		}

		ev := s.events[0]
		if ev.when.After(s.now) {
			s.mu.Unlock()
			return
		}

		// Pop the first event
		s.events = s.events[1:]

		if ev.cancelled {
			s.mu.Unlock()
			continue
		}

		if s.index != nil {
			delete(s.index, ev.id)
		}

		callback := ev.f
		s.mu.Unlock()

		// Execute callback outside the lock.
		if callback != nil {
			callback()
		}
	}
}

// AdvanceTo sets the fake simulation time to the given time and executes all due events.
// Time is kept monotonic (does not go backwards).
func (s *FakeEventScheduler) AdvanceTo(t time.Time) {
	s.mu.Lock()
	if t.Before(s.now) {
		// Do not go backwards in time; keep it monotonic.
		s.mu.Unlock()
		return
	}
	s.now = t
	s.mu.Unlock()

	// After adjusting time, run all due events.
	s.RunDue()
}

