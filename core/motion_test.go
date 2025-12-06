package core

import (
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/model"
)

type capturingUpdater struct {
	positions map[string]model.Motion
	calls     map[string]int
}

func (c *capturingUpdater) UpdatePlatformPosition(id string, pos model.Motion) error {
	if c.positions == nil {
		c.positions = make(map[string]model.Motion)
	}
	if c.calls == nil {
		c.calls = make(map[string]int)
	}
	c.positions[id] = pos
	c.calls[id]++
	return nil
}

func (c *capturingUpdater) snapshot(id string) (model.Motion, int) {
	return c.positions[id], c.calls[id]
}

func TestStaticMotionModel_NoChange(t *testing.T) {
	m := &StaticMotionModel{}
	p := &model.PlatformDefinition{
		Coordinates: model.Motion{X: 1, Y: 2, Z: 3},
	}

	t1 := time.Now().UTC()
	m.UpdatePosition(t1, p)
	if p.Coordinates != (model.Motion{X: 1, Y: 2, Z: 3}) {
		t.Fatalf("static motion should not change coordinates, got %#v", p.Coordinates)
	}

	t2 := t1.Add(time.Hour)
	m.UpdatePosition(t2, p)
	if p.Coordinates != (model.Motion{X: 1, Y: 2, Z: 3}) {
		t.Fatalf("static motion should not change coordinates after second update, got %#v", p.Coordinates)
	}
}

// We don't assert exact orbital values (those belong to go-satellite);
// we just ensure that positions differ at distinct times.
func TestOrbitalSGP4MotionModel_ChangesOverTime(t *testing.T) {
	// ISS sample TLE (also used in testdata/iss.tle)
	tle1 := "1 25544U 98067A   21275.59097222  .00000204  00000-0  10270-4 0  9990"
	tle2 := "2 25544  51.6459 115.9059 0001817  61.3028  35.9198 15.49370953257760"

	m := NewOrbitalModelFromTLE(tle1, tle2)
	p := &model.PlatformDefinition{}

	t1 := time.Date(2021, 10, 2, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(5 * time.Minute)

	m.UpdatePosition(t1, p)
	first := p.Coordinates

	m.UpdatePosition(t2, p)
	second := p.Coordinates

	if first == second {
		t.Fatalf("expected orbital position to change over time, got %+v at both times", first)
	}
}

func TestMotionModel_AddUpdateAndRemove(t *testing.T) {
	tle1 := "1 25544U 98067A   21275.59097222  .00000204  00000-0  10270-4 0  9990"
	tle2 := "2 25544  51.6459 115.9059 0001817  61.3028  35.9198 15.49370953257760"

	sat := &model.PlatformDefinition{
		ID:           "sat1",
		MotionSource: model.MotionSourceSpacetrack,
	}
	ground := &model.PlatformDefinition{
		ID:           "ground1",
		MotionSource: model.MotionSourceUnknown,
		Coordinates:  model.Motion{X: 1, Y: 2, Z: 3},
	}

	updater := &capturingUpdater{}
	mm := NewMotionModel(WithTLEFetcher(func(pd *model.PlatformDefinition) (string, string) {
		if pd.ID == sat.ID {
			return tle1, tle2
		}
		return "", ""
	}), WithPositionUpdater(updater))

	if err := mm.AddPlatform(sat); err != nil {
		t.Fatalf("AddPlatform sat: %v", err)
	}
	if err := mm.AddPlatform(ground); err != nil {
		t.Fatalf("AddPlatform ground: %v", err)
	}
	if err := mm.AddPlatform(sat); err == nil {
		t.Fatalf("expected duplicate AddPlatform error")
	}

	t1 := time.Date(2021, 10, 2, 0, 0, 0, 0, time.UTC)
	if err := mm.UpdatePositions(t1); err != nil {
		t.Fatalf("UpdatePositions first tick: %v", err)
	}
	firstSat, satCalls := updater.snapshot(sat.ID)
	firstGround, groundCalls := updater.snapshot(ground.ID)

	t2 := t1.Add(5 * time.Minute)
	if err := mm.UpdatePositions(t2); err != nil {
		t.Fatalf("UpdatePositions second tick: %v", err)
	}
	secondSat, satCalls2 := updater.snapshot(sat.ID)
	secondGround, groundCalls2 := updater.snapshot(ground.ID)
	if satCalls2 <= satCalls {
		t.Fatalf("expected satellite position to be updated at least once, got calls %d -> %d", satCalls, satCalls2)
	}
	if firstSat == secondSat {
		t.Fatalf("expected satellite position to change after UpdatePositions, got %+v", secondSat)
	}
	if firstGround != secondGround {
		t.Fatalf("static platform coordinates should stay constant, got %+v", secondGround)
	}
	if groundCalls2 <= groundCalls {
		t.Fatalf("expected ground platform to be propagated again, got calls %d -> %d", groundCalls, groundCalls2)
	}

	if err := mm.RemovePlatform(ground.ID); err != nil {
		t.Fatalf("RemovePlatform: %v", err)
	}

	if err := mm.UpdatePositions(t2.Add(time.Minute)); err != nil {
		t.Fatalf("UpdatePositions after removal: %v", err)
	}
	_, groundCalls3 := updater.snapshot(ground.ID)
	if groundCalls3 != groundCalls2 {
		t.Fatalf("removed platform should not be updated, got calls %d -> %d", groundCalls2, groundCalls3)
	}
}
