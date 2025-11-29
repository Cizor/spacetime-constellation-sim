package core

import "testing"

func TestHasLineOfSight_NoObstruction(t *testing.T) {
	// Two satellites high and on the same side of Earth, separated in Y.
	// The segment between them stays at x ≈ 8000 km, well outside Earth.
	posA := Vec3{X: 8000, Y: 0, Z: 0}
	posB := Vec3{X: 8000, Y: 1000, Z: 0}

	if !hasLineOfSight(posA, posB) {
		t.Errorf("expected LoS between two high satellites on same side of Earth")
	}
}

func TestHasLineOfSight_Obstructed(t *testing.T) {
	// Two points on opposite sides: the chord passes through the Earth.
	posA := Vec3{X: 7000, Y: 0, Z: 0}
	posB := Vec3{X: -7000, Y: 0, Z: 0}

	if hasLineOfSight(posA, posB) {
		t.Errorf("expected LoS to be blocked by Earth")
	}
}
