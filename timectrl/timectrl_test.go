package timectrl

import (
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
