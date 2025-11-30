package types

import (
	"testing"

	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestPlatformMappingRoundTrip_Domain(t *testing.T) {
	orig := &model.PlatformDefinition{
		ID:           "ISS",
		Name:         "ISS",
		Type:         "SATELLITE",
		CategoryTag:  "demo-leo",
		NoradID:      25544,
		MotionSource: model.MotionSourceSpacetrack,
		Coordinates: model.Motion{
			X: 1234.5,
			Y: -9876.5,
			Z: 42.0,
		},
	}

	p := PlatformToProto(orig)
	if p == nil {
		t.Fatalf("PlatformToProto returned nil")
	}

	back, err := PlatformFromProto(p)
	if err != nil {
		t.Fatalf("PlatformFromProto returned error: %v", err)
	}

	if back.ID != orig.ID {
		t.Errorf("ID mismatch: got %q, want %q", back.ID, orig.ID)
	}
	if back.Name != orig.Name {
		t.Errorf("Name mismatch: got %q, want %q", back.Name, orig.Name)
	}
	if back.Type != orig.Type {
		t.Errorf("Type mismatch: got %q, want %q", back.Type, orig.Type)
	}
	if back.CategoryTag != orig.CategoryTag {
		t.Errorf("CategoryTag mismatch: got %q, want %q", back.CategoryTag, orig.CategoryTag)
	}
	if back.NoradID != orig.NoradID {
		t.Errorf("NoradID mismatch: got %d, want %d", back.NoradID, orig.NoradID)
	}
	if back.MotionSource != orig.MotionSource {
		t.Errorf("MotionSource mismatch: got %v, want %v", back.MotionSource, orig.MotionSource)
	}

	if back.Coordinates.X != orig.Coordinates.X ||
		back.Coordinates.Y != orig.Coordinates.Y ||
		back.Coordinates.Z != orig.Coordinates.Z {
		t.Errorf("Coordinates mismatch: got (%f,%f,%f), want (%f,%f,%f)",
			back.Coordinates.X, back.Coordinates.Y, back.Coordinates.Z,
			orig.Coordinates.X, orig.Coordinates.Y, orig.Coordinates.Z)
	}
}

func TestNetworkNodeMappingRoundTrip_Domain(t *testing.T) {
	orig := &model.NetworkNode{
		ID:         "node-1",
		Name:       "gw-1",
		Type:       "GROUND",
		PlatformID: "platform-1",
	}

	p := NodeToProto(orig)
	if p == nil {
		t.Fatalf("NodeToProto returned nil")
	}

	back, err := NodeFromProto(p)
	if err != nil {
		t.Fatalf("NodeFromProto returned error: %v", err)
	}

	if back.ID != orig.ID {
		t.Errorf("ID mismatch: got %q, want %q", back.ID, orig.ID)
	}
	if back.Name != orig.Name {
		t.Errorf("Name mismatch: got %q, want %q", back.Name, orig.Name)
	}
	if back.Type != orig.Type {
		t.Errorf("Type mismatch: got %q, want %q", back.Type, orig.Type)
	}

	// PlatformID is not encoded in the proto, so we expect it to be empty
	// when we come back from NodeFromProto.
	if back.PlatformID != "" {
		t.Errorf("expected PlatformID to be empty after roundtrip, got %q", back.PlatformID)
	}
}
