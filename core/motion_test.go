package core

import (
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/model"
)

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

	mm := NewMotionModel(WithTLEFetcher(func(pd *model.PlatformDefinition) (string, string) {
		if pd.ID == sat.ID {
			return tle1, tle2
		}
		return "", ""
	}))

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
	firstSat := sat.Coordinates
	firstGround := ground.Coordinates

	t2 := t1.Add(5 * time.Minute)
	if err := mm.UpdatePositions(t2); err != nil {
		t.Fatalf("UpdatePositions second tick: %v", err)
	}
	if firstSat == sat.Coordinates {
		t.Fatalf("expected satellite position to change after UpdatePositions, got %+v", sat.Coordinates)
	}
	if firstGround != ground.Coordinates {
		t.Fatalf("static platform coordinates should stay constant, got %+v", ground.Coordinates)
	}

	if err := mm.RemovePlatform(ground.ID); err != nil {
		t.Fatalf("RemovePlatform: %v", err)
	}
	ground.Coordinates = model.Motion{X: 9, Y: 9, Z: 9}
	if err := mm.UpdatePositions(t2.Add(time.Minute)); err != nil {
		t.Fatalf("UpdatePositions after removal: %v", err)
	}
	if ground.Coordinates != (model.Motion{X: 9, Y: 9, Z: 9}) {
		t.Fatalf("removed platform should not be updated, got %+v", ground.Coordinates)
	}
}
