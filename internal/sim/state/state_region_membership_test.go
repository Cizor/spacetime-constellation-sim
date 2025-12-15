package state

import (
	"testing"

	"github.com/signalsfoundry/constellation-simulator/model"
)

type membershipEvent struct {
	regionID string
	left     []string
	entered  []string
}

func TestRegionMembershipCaching(t *testing.T) {
	state := newTestStateWithPlatform(t)
	ensureNode(t, state, "node-a")

	region := &model.Region{
		ID:       "reg-1",
		Type:     model.RegionTypeCircle,
		Center:   model.Coordinates{},
		RadiusKm: 1,
	}
	if err := state.CreateRegion(region); err != nil {
		t.Fatalf("CreateRegion() error = %v", err)
	}

	if err := state.UpdateRegionMembership("reg-1"); err != nil {
		t.Fatalf("UpdateRegionMembership() error = %v", err)
	}
	if !state.CheckRegionMembership("node-a", "reg-1") {
		t.Fatalf("CheckRegionMembership() expected true for node inside region")
	}

	platform, err := state.GetPlatform("plat-1")
	if err != nil {
		t.Fatalf("GetPlatform() error = %v", err)
	}
	platform.Coordinates = model.Motion{X: 3_000_000}
	if err := state.UpdatePlatform(platform); err != nil {
		t.Fatalf("UpdatePlatform() error = %v", err)
	}

	state.SetRegionMembershipTTL(0)
	if state.CheckRegionMembership("node-a", "reg-1") {
		t.Fatalf("expected membership to refresh and become false after node moved")
	}
}

func TestRegionMembershipHook(t *testing.T) {
	state := newTestStateWithPlatform(t)
	ensureNode(t, state, "node-a")

	region := &model.Region{
		ID:       "reg-hook",
		Type:     model.RegionTypeCircle,
		Center:   model.Coordinates{},
		RadiusKm: 1,
	}
	if err := state.CreateRegion(region); err != nil {
		t.Fatalf("CreateRegion() error = %v", err)
	}

	var events []membershipEvent
	state.SetRegionMembershipHook(func(regionID string, left, entered []string) {
		events = append(events, membershipEvent{
			regionID: regionID,
			left:     append([]string(nil), left...),
			entered:  append([]string(nil), entered...),
		})
	})

	if err := state.UpdateRegionMembership("reg-hook"); err != nil {
		t.Fatalf("UpdateRegionMembership() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 hook event, got %d", len(events))
	}
	if events[0].regionID != "reg-hook" || len(events[0].entered) != 1 {
		t.Fatalf("unexpected hook event: %+v", events[0])
	}

	events = events[:0]
	if err := state.UpdateRegionMembership("reg-hook"); err != nil {
		t.Fatalf("UpdateRegionMembership() error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no hook when membership unchanged, got %d", len(events))
	}

	platform, err := state.GetPlatform("plat-1")
	if err != nil {
		t.Fatalf("GetPlatform() error = %v", err)
	}
	platform.Coordinates = model.Motion{X: 3_000_000}
	if err := state.UpdatePlatform(platform); err != nil {
		t.Fatalf("UpdatePlatform() error = %v", err)
	}

	state.SetRegionMembershipTTL(0)
	if err := state.UpdateRegionMembership("reg-hook"); err != nil {
		t.Fatalf("UpdateRegionMembership() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected hook event for node exit, got %d", len(events))
	}
	if len(events[0].left) != 1 || events[0].left[0] != "node-a" {
		t.Fatalf("unexpected left nodes: %+v", events[0].left)
	}
}
