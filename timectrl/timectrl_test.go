package timectrl

import (
	"testing"
	"time"
)

func TestTimeControllerSetTime(t *testing.T) {
	start := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	tc := NewTimeController(start, time.Second, RealTime)

	newNow := start.Add(42 * time.Second)
	tc.SetTime(newNow)

	if got := tc.Now(); !got.Equal(newNow) {
		t.Fatalf("Now() = %v, want %v", got, newNow)
	}
}

func TestTimeControllerStartUpdatesNow(t *testing.T) {
	start := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	tc := NewTimeController(start, 5*time.Millisecond, Accelerated)

	done := tc.Start(15 * time.Millisecond)
	<-done

	expected := start.Add(15 * time.Millisecond)
	if got := tc.Now(); !got.Equal(expected) {
		t.Fatalf("Now() = %v, want %v", got, expected)
	}
}
