package main

import (
	"context"
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/timectrl"
)

type countingEventScheduler struct {
	sbi.EventScheduler
	runDueCalls int
}

func (c *countingEventScheduler) RunDue() {
	c.runDueCalls++
	c.EventScheduler.RunDue()
}

type fakeReplanScheduler struct {
	recomputeCount int
	beamCount      int
	routeCount     int
	scheduleCount  int
}

func (f *fakeReplanScheduler) RecomputeContactWindows(context.Context, time.Time, time.Time) {
	f.recomputeCount++
}

func (f *fakeReplanScheduler) ScheduleLinkBeams(context.Context) error {
	f.beamCount++
	return nil
}

func (f *fakeReplanScheduler) ScheduleLinkRoutes(context.Context) error {
	f.routeCount++
	return nil
}

func (f *fakeReplanScheduler) ScheduleServiceRequests(context.Context) error {
	f.scheduleCount++
	return nil
}

func (f *fakeReplanScheduler) RunReplanningLoop(ctx context.Context, interval time.Duration) {
	if ctx == nil {
		return
	}
	if interval <= 0 {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			f.RecomputeContactWindows(ctx, now, now.Add(time.Second))
			_ = f.ScheduleLinkBeams(ctx)
			_ = f.ScheduleLinkRoutes(ctx)
			_ = f.ScheduleServiceRequests(ctx)
		}
	}
}

func TestRunSimLoop_EventSchedulerAndReplan(t *testing.T) {
	originalInterval := replanInterval
	replanInterval = 40 * time.Millisecond
	defer func() { replanInterval = originalInterval }()

	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	state := state.NewScenarioState(physKB, netKB, logging.Noop())
	motionModel := core.NewMotionModel()
	connectivity := core.NewConnectivityService(netKB)

	start := time.Now()
	tc := timectrl.NewTimeController(start, 10*time.Millisecond, timectrl.Accelerated)
	eventScheduler := &countingEventScheduler{
		EventScheduler: sbi.NewEventScheduler(tc),
	}

	fakeScheduler := &fakeReplanScheduler{}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runSimLoop(ctx, tc, state, motionModel, connectivity, eventScheduler, fakeScheduler, logging.Noop())
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	if eventScheduler.runDueCalls == 0 {
		t.Fatalf("expected RunDue to be called at least once")
	}
	if fakeScheduler.recomputeCount == 0 {
		t.Fatalf("expected RecomputeContactWindows to be called at least once")
	}
	if fakeScheduler.scheduleCount == 0 {
		t.Fatalf("expected ScheduleServiceRequests to be called at least once")
	}
}
