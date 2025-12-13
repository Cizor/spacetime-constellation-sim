package timectrl

import (
	"sync"
	"testing"
	"time"
)

func TestTimeControllerTicks(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	tick := 10 * time.Millisecond
	duration := 50 * time.Millisecond

	tc := NewTimeController(start, tick, RealTime)

	var count int
	var last time.Time
	tc.AddListener(func(ts time.Time) {
		count++
		last = ts
	})

	done := tc.Start(duration)
	<-done

	if count == 0 {
		t.Fatalf("expected at least one tick, got 0")
	}
	// Rough check: we expect about duration/tick ticks.
	expected := int(duration / tick)
	if count < expected-1 || count > expected+1 {
		t.Fatalf("unexpected tick count: got %d, want approx %d", count, expected)
	}
	if last.Before(start) {
		t.Fatalf("last tick time %v before start %v", last, start)
	}
}

func TestTimeControllerImplementsSimClock(t *testing.T) {
	start := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	tc := NewTimeController(start, 1*time.Second, Accelerated)

	// Verify it implements SimClock interface
	var clock SimClock = tc

	// Test Now() returns initial time
	now := clock.Now()
	if !now.Equal(start) {
		t.Fatalf("Now() = %v, want %v", now, start)
	}

	// Test After() returns a channel (stub for now)
	ch := clock.After(5 * time.Second)
	if ch == nil {
		t.Fatalf("After() returned nil channel")
	}
}

func TestTimeControllerNowUpdatesWithTime(t *testing.T) {
	start := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	tick := 100 * time.Millisecond
	tc := NewTimeController(start, tick, Accelerated)

	// Initially should return start time
	if !tc.Now().Equal(start) {
		t.Fatalf("Initial Now() = %v, want %v", tc.Now(), start)
	}

	// Start the controller and let it advance
	done := tc.Start(250 * time.Millisecond)
	<-done

	// Now() should reflect advanced time
	now := tc.Now()
	if now.Before(start) || now.Equal(start) {
		t.Fatalf("Now() after advance = %v, should be after start %v", now, start)
	}
	// Should have advanced by approximately 250ms (allowing for timing variance)
	expectedMin := start.Add(200 * time.Millisecond)
	if now.Before(expectedMin) {
		t.Fatalf("Now() = %v, expected at least %v", now, expectedMin)
	}
}

// FakeSimClock is a test implementation of SimClock that allows deterministic
// control over simulation time.
type FakeSimClock struct {
	mu  sync.RWMutex
	now time.Time
}

// NewFakeSimClock creates a new fake clock starting at the given time.
func NewFakeSimClock(start time.Time) *FakeSimClock {
	return &FakeSimClock{now: start}
}

// Now returns the current simulated time. Implements SimClock.
func (f *FakeSimClock) Now() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.now
}

// After returns a channel that will receive the current simulation time
// after the duration d has elapsed. Implements SimClock.
//
// TODO: This will be enhanced in later Scope 4 chunks to fire timers
// when AdvanceTo/AdvanceBy hits the appropriate simulated time.
func (f *FakeSimClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	// Later, we can trigger this channel when AdvanceTo/AdvanceBy hits the appropriate simulated time.
	// For now, tests may not depend on After() firing.
	return ch
}

// AdvanceTo sets the simulated time to the given time.
func (f *FakeSimClock) AdvanceTo(t time.Time) {
	f.mu.Lock()
	f.now = t
	f.mu.Unlock()
	// A later Scope 4 chunk can add logic to fire any registered timers here.
}

// AdvanceBy advances the simulated time by the given duration.
func (f *FakeSimClock) AdvanceBy(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	f.mu.Unlock()
	// A later Scope 4 chunk can add logic to fire any registered timers here.
}

func TestFakeSimClock(t *testing.T) {
	start := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := NewFakeSimClock(start)

	// Verify it implements SimClock interface
	var _ SimClock = clock

	// Test Now() returns start time
	now := clock.Now()
	if !now.Equal(start) {
		t.Fatalf("Now() = %v, want %v", now, start)
	}

	// Test AdvanceTo
	newTime := start.Add(1 * time.Hour)
	clock.AdvanceTo(newTime)
	now = clock.Now()
	if !now.Equal(newTime) {
		t.Fatalf("Now() after AdvanceTo = %v, want %v", now, newTime)
	}

	// Test AdvanceBy
	clock.AdvanceBy(30 * time.Minute)
	now = clock.Now()
	expected := newTime.Add(30 * time.Minute)
	if !now.Equal(expected) {
		t.Fatalf("Now() after AdvanceBy = %v, want %v", now, expected)
	}

	// Test After() returns a channel (stub for now)
	ch := clock.After(5 * time.Minute)
	if ch == nil {
		t.Fatalf("After() returned nil channel")
	}
}
