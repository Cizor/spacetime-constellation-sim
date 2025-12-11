package sbi

import (
	"sync"
	"testing"
	"time"
)

// fakeClock is a minimal test-only implementation of SimClock for scheduler tests.
type fakeClock struct {
	mu  sync.RWMutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	// Not used in these tests; simple stub is fine.
	ch := make(chan time.Time, 1)
	return ch
}

func (c *fakeClock) AdvanceTo(t time.Time) {
	c.mu.Lock()
	c.now = t
	c.mu.Unlock()
}

func TestEventScheduler_SingleEvent(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	sched := NewEventScheduler(clock)

	var counter int
	t1 := start.Add(10 * time.Second)

	// Schedule an event at t1
	id := sched.Schedule(t1, func() {
		counter++
	})

	if id == "" {
		t.Fatalf("Schedule returned empty ID")
	}

	// Call RunDue at t0 - event should not run yet
	sched.RunDue()
	if counter != 0 {
		t.Fatalf("expected counter=0 before time advance, got %d", counter)
	}

	// Advance clock to t1 and run due events
	clock.AdvanceTo(t1)
	sched.RunDue()

	if counter != 1 {
		t.Fatalf("expected counter=1 after time advance, got %d", counter)
	}

	// RunDue again - event should not run twice
	sched.RunDue()
	if counter != 1 {
		t.Fatalf("expected counter=1 after second RunDue (event should not run twice), got %d", counter)
	}
}

func TestEventScheduler_MultipleEventsInOrder(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	sched := NewEventScheduler(clock)

	var executionOrder []string
	t1 := start.Add(10 * time.Second)
	t2 := start.Add(20 * time.Second)
	t3 := start.Add(30 * time.Second)

	// Schedule events in reverse order to test ordering
	sched.Schedule(t3, func() {
		executionOrder = append(executionOrder, "e3")
	})
	sched.Schedule(t1, func() {
		executionOrder = append(executionOrder, "e1")
	})
	sched.Schedule(t2, func() {
		executionOrder = append(executionOrder, "e2")
	})

	// Advance to t2 - should run e1 and e2
	clock.AdvanceTo(t2)
	sched.RunDue()

	if len(executionOrder) != 2 {
		t.Fatalf("expected 2 events executed, got %d", len(executionOrder))
	}
	if executionOrder[0] != "e1" || executionOrder[1] != "e2" {
		t.Fatalf("expected execution order [e1, e2], got %v", executionOrder)
	}

	// Advance to t3 - should run e3
	clock.AdvanceTo(t3)
	sched.RunDue()

	if len(executionOrder) != 3 {
		t.Fatalf("expected 3 events executed, got %d", len(executionOrder))
	}
	if executionOrder[2] != "e3" {
		t.Fatalf("expected execution order [e1, e2, e3], got %v", executionOrder)
	}
}

func TestEventScheduler_PastDueEvent(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	sched := NewEventScheduler(clock)

	var counter int
	t0 := start.Add(-5 * time.Second) // Event scheduled in the past

	// Schedule an event in the past
	sched.Schedule(t0, func() {
		counter++
	})

	// RunDue should execute the past-due event immediately
	sched.RunDue()

	if counter != 1 {
		t.Fatalf("expected past-due event to run immediately, counter=%d", counter)
	}
}

func TestEventScheduler_Cancellation(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	sched := NewEventScheduler(clock)

	var counter int
	t1 := start.Add(10 * time.Second)

	// Schedule an event
	id := sched.Schedule(t1, func() {
		counter++
	})

	// Cancel it before time advances
	sched.Cancel(id)

	// Advance clock and run due events
	clock.AdvanceTo(t1)
	sched.RunDue()

	if counter != 0 {
		t.Fatalf("expected cancelled event to not run, counter=%d", counter)
	}
}

func TestEventScheduler_CancelUnknownID(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	sched := NewEventScheduler(clock)

	// Cancel an unknown ID - should be a no-op
	sched.Cancel("unknown-id")

	// Should not panic or cause issues
	clock.AdvanceTo(start.Add(1 * time.Second))
	sched.RunDue()
}

func TestEventScheduler_Reentrancy(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	sched := NewEventScheduler(clock)

	var counter int
	t1 := start.Add(10 * time.Second)
	t2 := start.Add(20 * time.Second)

	// Schedule an event that schedules another event
	sched.Schedule(t1, func() {
		counter++
		// Schedule another event from within the callback
		sched.Schedule(t2, func() {
			counter++
		})
	})

	// Advance to t1 and run due events
	clock.AdvanceTo(t1)
	sched.RunDue()

	if counter != 1 {
		t.Fatalf("expected counter=1 after first event, got %d", counter)
	}

	// Advance to t2 and run due events - the nested event should run
	clock.AdvanceTo(t2)
	sched.RunDue()

	if counter != 2 {
		t.Fatalf("expected counter=2 after nested event, got %d", counter)
	}
}

func TestEventScheduler_Now(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	sched := NewEventScheduler(clock)

	now := sched.Now()
	if !now.Equal(start) {
		t.Fatalf("Now() = %v, want %v", now, start)
	}

	// Advance clock
	newTime := start.Add(1 * time.Hour)
	clock.AdvanceTo(newTime)

	now = sched.Now()
	if !now.Equal(newTime) {
		t.Fatalf("Now() after advance = %v, want %v", now, newTime)
	}
}

func TestEventScheduler_MultipleRunDueCalls(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	sched := NewEventScheduler(clock)

	var counter int
	t1 := start.Add(10 * time.Second)

	sched.Schedule(t1, func() {
		counter++
	})

	// Advance clock
	clock.AdvanceTo(t1)

	// Call RunDue multiple times - event should only run once
	sched.RunDue()
	sched.RunDue()
	sched.RunDue()

	if counter != 1 {
		t.Fatalf("expected counter=1 after multiple RunDue calls, got %d", counter)
	}
}

