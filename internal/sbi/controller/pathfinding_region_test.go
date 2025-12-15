package controller

import (
	"context"
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/model"
)

func setupRegionPathfindingScheduler(t *testing.T) (*Scheduler, string, time.Time) {
	t.Helper()

	scheduler, linkID, now := setupPathfindingScheduler(t)
	state := scheduler.State

	if err := state.CreatePlatform(&model.PlatformDefinition{
		ID:          "plat-A",
		Coordinates: model.Motion{X: 0, Y: 0, Z: 0},
	}); err != nil {
		t.Fatalf("CreatePlatform plat-A: %v", err)
	}
	if err := state.CreatePlatform(&model.PlatformDefinition{
		ID:          "plat-B",
		Coordinates: model.Motion{X: 1_000, Y: 0, Z: 0},
	}); err != nil {
		t.Fatalf("CreatePlatform plat-B: %v", err)
	}

	nodeA, _, err := state.GetNode("node-A")
	if err != nil {
		t.Fatalf("GetNode node-A: %v", err)
	}
	nodeA.PlatformID = "plat-A"

	nodeB, _, err := state.GetNode("node-B")
	if err != nil {
		t.Fatalf("GetNode node-B: %v", err)
	}
	nodeB.PlatformID = "plat-B"

	return scheduler, linkID, now
}

func TestFindRegionToNodePath(t *testing.T) {
	scheduler, linkID, now := setupRegionPathfindingScheduler(t)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkID: {
			{StartTime: now.Add(1 * time.Minute), EndTime: now.Add(4 * time.Minute), Quality: 0},
		},
	}
	region := &model.Region{
		ID:       "region-a",
		Type:     model.RegionTypeCircle,
		Center:   model.Coordinates{X: 0, Y: 0, Z: 0},
		RadiusKm: 0.5,
	}
	if err := scheduler.State.CreateRegion(region); err != nil {
		t.Fatalf("CreateRegion: %v", err)
	}

	path, err := scheduler.FindRegionToNodePath(context.Background(), region.ID, "node-B", now, 10*time.Minute)
	if err != nil {
		t.Fatalf("FindRegionToNodePath failed: %v", err)
	}
	if path == nil || len(path.Hops) != 1 {
		t.Fatalf("expected path, got %+v", path)
	}
	if path.Hops[0].FromNodeID != "node-A" {
		t.Fatalf("expected path from node-A, got %s", path.Hops[0].FromNodeID)
	}
}

func TestFindNodeToRegionPath(t *testing.T) {
	scheduler, linkID, now := setupRegionPathfindingScheduler(t)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkID: {
			{StartTime: now.Add(1 * time.Minute), EndTime: now.Add(4 * time.Minute), Quality: 0},
		},
	}
	region := &model.Region{
		ID:       "region-b",
		Type:     model.RegionTypeCircle,
		Center:   model.Coordinates{X: 1_000, Y: 0, Z: 0},
		RadiusKm: 0.5,
	}
	if err := scheduler.State.CreateRegion(region); err != nil {
		t.Fatalf("CreateRegion: %v", err)
	}

	path, err := scheduler.FindNodeToRegionPath(context.Background(), "node-A", region.ID, now, 10*time.Minute)
	if err != nil {
		t.Fatalf("FindNodeToRegionPath failed: %v", err)
	}
	if path == nil || len(path.Hops) != 1 {
		t.Fatalf("expected path, got %+v", path)
	}
	if path.Hops[0].ToNodeID != "node-B" {
		t.Fatalf("expected path to node-B, got %s", path.Hops[0].ToNodeID)
	}
}

func TestFindRegionPath(t *testing.T) {
	scheduler, linkID, now := setupRegionPathfindingScheduler(t)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkID: {
			{StartTime: now.Add(1 * time.Minute), EndTime: now.Add(4 * time.Minute), Quality: 0},
		},
	}
	regionA := &model.Region{
		ID:       "region-a",
		Type:     model.RegionTypeCircle,
		Center:   model.Coordinates{X: 0, Y: 0, Z: 0},
		RadiusKm: 0.5,
	}
	if err := scheduler.State.CreateRegion(regionA); err != nil {
		t.Fatalf("CreateRegion A: %v", err)
	}
	regionB := &model.Region{
		ID:       "region-b",
		Type:     model.RegionTypeCircle,
		Center:   model.Coordinates{X: 1_000, Y: 0, Z: 0},
		RadiusKm: 10,
	}
	if err := scheduler.State.CreateRegion(regionB); err != nil {
		t.Fatalf("CreateRegion B: %v", err)
	}

	path, err := scheduler.FindRegionPath(context.Background(), regionA.ID, regionB.ID, now, 10*time.Minute)
	if err != nil {
		t.Fatalf("FindRegionPath failed: %v", err)
	}
	if path == nil || len(path.Hops) != 1 {
		t.Fatalf("expected path, got %+v", path)
	}
	if path.Hops[0].FromNodeID != "node-A" || path.Hops[0].ToNodeID != "node-B" {
		t.Fatalf("expected path from node-A to node-B, got %+v", path.Hops[0])
	}
}
