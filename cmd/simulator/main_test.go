package main

import (
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
	"github.com/signalsfoundry/constellation-simulator/timectrl"
)

// TestIntegration_SingleSatAndGround runs a tiny end-to-end-style simulation.
func TestIntegration_SingleSatAndGround(t *testing.T) {
	store := kb.NewKnowledgeBase()

	sat := &model.PlatformDefinition{
		ID:           "sat1",
		Name:         "LEO-Sat-1",
		Type:         "SATELLITE",
		MotionSource: model.MotionSourceSpacetrack,
	}
	ground := &model.PlatformDefinition{
		ID:           "ground1",
		Name:         "Equator-GS",
		Type:         "GROUND_STATION",
		MotionSource: model.MotionSourceUnknown,
		Coordinates:  model.Motion{X: 6371000, Y: 0, Z: 0},
	}
	if err := store.AddPlatform(sat); err != nil {
		t.Fatalf("AddPlatform sat error: %v", err)
	}
	if err := store.AddPlatform(ground); err != nil {
		t.Fatalf("AddPlatform ground error: %v", err)
	}

	tle1 := "1 25544U 98067A   21275.59097222  .00000204  00000-0  10270-4 0  9990"
	tle2 := "2 25544  51.6459 115.9059 0001817  61.3028  35.9198 15.49370953257760"

	satModel := core.NewMotionModel(sat, tle1, tle2)
	groundModel := core.NewMotionModel(ground, "", "")

	// Run a short accelerated simulation.
	start := time.Date(2021, 10, 2, 0, 0, 0, 0, time.UTC)
	tc := timectrl.NewTimeController(start, 1*time.Second, timectrl.Accelerated)

	var satFirst, satLast model.Motion
	ticks := 0

	tc.AddListener(func(simTime time.Time) {
		satModel.UpdatePosition(simTime, sat)
		_ = store.UpdatePlatformPosition(sat.ID, sat.Coordinates)
		groundModel.UpdatePosition(simTime, ground)
		_ = store.UpdatePlatformPosition(ground.ID, ground.Coordinates)

		if ticks == 0 {
			satFirst = sat.Coordinates
		}
		satLast = sat.Coordinates
		ticks++
	})

	done := tc.Start(5 * time.Second)
	<-done

	if ticks == 0 {
		t.Fatalf("expected at least one tick, got 0")
	}
	if satFirst == satLast {
		t.Fatalf("expected satellite position to change over time, got %+v first == last", satFirst)
	}
	if got := store.GetPlatform("ground1"); got == nil || got.Coordinates != ground.Coordinates {
		t.Fatalf("ground platform not found or coords mismatch")
	}
}
