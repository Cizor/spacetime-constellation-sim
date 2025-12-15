package controller

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/timectrl"
)

func TestRunReplanningLoop_TickerDriven(t *testing.T) {
	scheduler := newTestScheduler(t)

	var beams, routes, srSched, active int32
	scheduler.replanHooks = replanHooks{
		RecomputeContactWindows: func(ctx context.Context, now, horizon time.Time) {},
		ScheduleLinkBeams: func(ctx context.Context) error {
			atomic.AddInt32(&beams, 1)
			return nil
		},
		ScheduleLinkRoutes: func(ctx context.Context) error {
			atomic.AddInt32(&routes, 1)
			return nil
		},
		ScheduleServiceRequests: func(ctx context.Context) error {
			atomic.AddInt32(&srSched, 1)
			return nil
		},
		ReplanActivePaths: func(ctx context.Context, now time.Time) {
			atomic.AddInt32(&active, 1)
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	go scheduler.RunReplanningLoop(ctx, 10*time.Millisecond)

	time.Sleep(60 * time.Millisecond)
	cancel()

	if atomic.LoadInt32(&beams) == 0 {
		t.Fatalf("expected beams scheduled at least once")
	}
	if atomic.LoadInt32(&routes) == 0 {
		t.Fatalf("expected routes scheduled at least once")
	}
	if atomic.LoadInt32(&srSched) == 0 {
		t.Fatalf("expected service request scheduling at least once")
	}
	if atomic.LoadInt32(&active) == 0 {
		t.Fatalf("expected active paths to be evaluated at least once")
	}
}

func TestRunReplanningLoop_RequestReplan(t *testing.T) {
	scheduler := newTestScheduler(t)

	var triggered int32
	scheduler.replanHooks = replanHooks{
		RecomputeContactWindows: func(ctx context.Context, now, horizon time.Time) {},
		ScheduleLinkBeams: func(ctx context.Context) error {
			return nil
		},
		ScheduleLinkRoutes: func(ctx context.Context) error {
			return nil
		},
		ScheduleServiceRequests: func(ctx context.Context) error {
			atomic.AddInt32(&triggered, 1)
			return nil
		},
		ReplanActivePaths: func(ctx context.Context, now time.Time) {},
	}

	ctx, cancel := context.WithCancel(context.Background())
	go scheduler.RunReplanningLoop(ctx, time.Second)

	time.Sleep(10 * time.Millisecond)
	scheduler.RequestReplan()
	time.Sleep(30 * time.Millisecond)
	cancel()

	if atomic.LoadInt32(&triggered) == 0 {
		t.Fatalf("expected RequestReplan to trigger a scheduling run")
	}
}

func newTestScheduler(t *testing.T) *Scheduler {
	t.Helper()

	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	tc := timectrl.NewTimeController(time.Now(), time.Second, timectrl.Accelerated)
	clock := sbi.NewEventScheduler(tc)
	scenarioState := state.NewScenarioState(physKB, netKB, logging.Noop())

	return NewScheduler(scenarioState, clock, nil, logging.Noop(), state.NewTelemetryState())
}
