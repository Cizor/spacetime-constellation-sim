package timectrl

import (
	"time"
)

// SimClock is an interface for accessing simulation time. This allows
// Scope 4 components (scheduler, agents) to depend on a clock abstraction
// rather than a concrete time controller type, enabling testability.
type SimClock interface {
	// Now returns the current simulation time.
	Now() time.Time
	// After returns a channel that will receive the current simulation time
	// after the duration d has elapsed in simulation time. This will be
	// integrated with the event scheduler in later Scope 4 chunks.
	After(d time.Duration) <-chan time.Time
}

// Mode describes how the TimeController advances simulation time.
type Mode int

const (
	// RealTime advances according to wall-clock time.
	RealTime Mode = iota
	// Accelerated advances as quickly as the loop can run while still stepping by Tick.
	Accelerated
)

// TimeController drives simulation time and notifies registered listeners.
// It implements SimClock for use by Scope 4 components.
type TimeController struct {
	StartTime time.Time
	Tick      time.Duration
	Mode      Mode

	listeners []func(time.Time)
}

// NewTimeController constructs a controller.
func NewTimeController(start time.Time, tick time.Duration, mode Mode) *TimeController {
	return &TimeController{
		StartTime: start,
		Tick:      tick,
		Mode:      mode,
	}
}

// AddListener registers a callback invoked on every tick.
func (tc *TimeController) AddListener(fn func(time.Time)) {
	tc.listeners = append(tc.listeners, fn)
}

// Start runs the controller for the specified duration in a separate goroutine.
// It returns a channel that is closed when the controller finishes.
func (tc *TimeController) Start(duration time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)

		simTime := tc.StartTime
		elapsed := time.Duration(0)

		// In both modes we use a ticker for simplicity and determinism.
		ticker := time.NewTicker(tc.Tick)
		defer ticker.Stop()

		for {
			if duration > 0 && elapsed >= duration {
				return
			}

			<-ticker.C
			simTime = simTime.Add(tc.Tick)
			elapsed += tc.Tick

			for _, fn := range tc.listeners {
				fn(simTime)
			}
		}
	}()
	return done
}
