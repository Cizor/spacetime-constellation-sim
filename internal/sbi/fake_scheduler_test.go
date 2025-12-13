package sbi

import (
	"testing"
	"time"
)

func TestFakeEventScheduler_BasicScheduling(t *testing.T) {
	start := time.Unix(0, 0)
	sched := NewFakeEventScheduler(start)

	var executionOrder []string
	t1 := start.Add(10 * time.Second)

	// Schedule a callback at t1
	sched.Schedule(t1, func() {
		executionOrder = append(executionOrder, "e1")
	})

	// Before advancing time, RunDue should not execute
	sched.RunDue()
	if len(executionOrder) != 0 {
		t.Fatalf("expected no events executed before time advance, got %d", len(executionOrder))
	}

	// Advance to t1 - event should execute
	sched.AdvanceTo(t1)
	if len(executionOrder) != 1 {
		t.Fatalf("expected 1 event executed, got %d", len(executionOrder))
	}
	if executionOrder[0] != "e1" {
		t.Fatalf("expected execution order [e1], got %v", executionOrder)
	}
}

func TestFakeEventScheduler_MultipleEventsInOrder(t *testing.T) {
	start := time.Unix(0, 0)
	sched := NewFakeEventScheduler(start)

	var executionOrder []string
	t1 := start.Add(10 * time.Second)
	t2 := start.Add(20 * time.Second)
	t3 := start.Add(30 * time.Second)

	// Schedule three events
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
	sched.AdvanceTo(t2)
	if len(executionOrder) != 2 {
		t.Fatalf("expected 2 events executed, got %d", len(executionOrder))
	}
	if executionOrder[0] != "e1" || executionOrder[1] != "e2" {
		t.Fatalf("expected execution order [e1, e2], got %v", executionOrder)
	}

	// Advance to t3 - should run e3
	sched.AdvanceTo(t3)
	if len(executionOrder) != 3 {
		t.Fatalf("expected 3 events executed, got %d", len(executionOrder))
	}
	if executionOrder[2] != "e3" {
		t.Fatalf("expected execution order [e1, e2, e3], got %v", executionOrder)
	}
}

func TestFakeEventScheduler_PastDueEvent(t *testing.T) {
	start := time.Unix(0, 0)
	sched := NewFakeEventScheduler(start)

	var counter int
	tPast := start.Add(-5 * time.Second) // Event scheduled in the past

	// Schedule an event in the past
	sched.Schedule(tPast, func() {
		counter++
	})

	// Advance to start - past-due event should run
	sched.AdvanceTo(start)
	if counter != 1 {
		t.Fatalf("expected past-due event to run when time is first advanced, counter=%d", counter)
	}
}

func TestFakeEventScheduler_Cancellation(t *testing.T) {
	start := time.Unix(0, 0)
	sched := NewFakeEventScheduler(start)

	var counter int
	t1 := start.Add(10 * time.Second)

	// Schedule an event
	id := sched.Schedule(t1, func() {
		counter++
	})

	// Cancel it
	sched.Cancel(id)

	// Advance time - cancelled event should not run
	sched.AdvanceTo(t1)
	if counter != 0 {
		t.Fatalf("expected cancelled event to not run, counter=%d", counter)
	}
}

func TestFakeEventScheduler_MonotonicTime(t *testing.T) {
	start := time.Unix(0, 0)
	sched := NewFakeEventScheduler(start)

	t1 := start.Add(10 * time.Second)
	t0 := start.Add(-5 * time.Second) // Earlier than start

	// Set initial time to t1
	sched.AdvanceTo(t1)

	// Try to go backwards to t0 - should be a no-op
	sched.AdvanceTo(t0)

	// Time should still be t1
	now := sched.Now()
	if !now.Equal(t1) {
		t.Fatalf("expected time to remain t1 after backwards AdvanceTo, got %v", now)
	}
}

func TestFakeEventScheduler_Now(t *testing.T) {
	start := time.Unix(0, 0)
	sched := NewFakeEventScheduler(start)

	now := sched.Now()
	if !now.Equal(start) {
		t.Fatalf("Now() = %v, want %v", now, start)
	}

	// Advance time
	newTime := start.Add(1 * time.Hour)
	sched.AdvanceTo(newTime)

	now = sched.Now()
	if !now.Equal(newTime) {
		t.Fatalf("Now() after AdvanceTo = %v, want %v", now, newTime)
	}
}

func TestFakeEventScheduler_CancelUnknownID(t *testing.T) {
	start := time.Unix(0, 0)
	sched := NewFakeEventScheduler(start)

	// Cancel an unknown ID - should be a no-op
	sched.Cancel("unknown-id")

	// Should not panic or cause issues
	sched.AdvanceTo(start.Add(1 * time.Second))
}

func TestFakeEventScheduler_MultipleRunDueCalls(t *testing.T) {
	start := time.Unix(0, 0)
	sched := NewFakeEventScheduler(start)

	var counter int
	t1 := start.Add(10 * time.Second)

	sched.Schedule(t1, func() {
		counter++
	})

	// Advance time
	sched.AdvanceTo(t1)

	// Call RunDue multiple times - event should only run once
	sched.RunDue()
	sched.RunDue()
	sched.RunDue()

	if counter != 1 {
		t.Fatalf("expected counter=1 after multiple RunDue calls, got %d", counter)
	}
}

func TestFakeEventScheduler_Reentrancy(t *testing.T) {
	start := time.Unix(0, 0)
	sched := NewFakeEventScheduler(start)

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

	// Advance to t1 - first event should run
	sched.AdvanceTo(t1)
	if counter != 1 {
		t.Fatalf("expected counter=1 after first event, got %d", counter)
	}

	// Advance to t2 - nested event should run
	sched.AdvanceTo(t2)
	if counter != 2 {
		t.Fatalf("expected counter=2 after nested event, got %d", counter)
	}
}

