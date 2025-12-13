package state

import (
	"fmt"
	"sync"
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

// TestInstallRoute_Idempotent verifies that calling InstallRoute
// multiple times with the same inputs is safe and replaces the route.
func TestInstallRoute_Idempotent(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	// Create platform and node
	platform := &model.PlatformDefinition{ID: "platformA", Name: "Platform A"}
	if err := s.CreatePlatform(platform); err != nil {
		t.Fatalf("CreatePlatform: %v", err)
	}
	node := &model.NetworkNode{ID: "nodeA", PlatformID: "platformA"}
	if err := s.CreateNode(node, nil); err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	route := model.RouteEntry{
		DestinationCIDR: "10.0.0.0/24",
		NextHopNodeID:   "nodeB",
		OutInterfaceID:  "if1",
	}

	// First call - should install the route
	if err := s.InstallRoute("nodeA", route); err != nil {
		t.Fatalf("First InstallRoute: %v", err)
	}

	// Verify route was installed
	got, err := s.GetRoutes("nodeA")
	if err != nil {
		t.Fatalf("GetRoutes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("After first call: GetRoutes length = %d, want 1", len(got))
	}

	// Second call with same route - should replace (idempotent)
	if err := s.InstallRoute("nodeA", route); err != nil {
		t.Fatalf("Second InstallRoute: %v", err)
	}

	// Verify route is still there (replaced, not duplicated)
	got2, err := s.GetRoutes("nodeA")
	if err != nil {
		t.Fatalf("GetRoutes after second call: %v", err)
	}
	if len(got2) != 1 {
		t.Fatalf("After second call: GetRoutes length = %d, want 1 (replaced, not duplicated)", len(got2))
	}
	if got2[0].DestinationCIDR != route.DestinationCIDR {
		t.Fatalf("After second call: Route DestinationCIDR = %q, want %q", got2[0].DestinationCIDR, route.DestinationCIDR)
	}
}

// TestRemoveRoute_Idempotent verifies that calling RemoveRoute
// multiple times is safe (second call should return error, but not corrupt state).
func TestRemoveRoute_Idempotent(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	// Create platform and node with route
	platform := &model.PlatformDefinition{ID: "platformA", Name: "Platform A"}
	if err := s.CreatePlatform(platform); err != nil {
		t.Fatalf("CreatePlatform: %v", err)
	}
	node := &model.NetworkNode{
		ID:         "nodeA",
		PlatformID: "platformA",
		Routes: []model.RouteEntry{
			{DestinationCIDR: "10.0.0.0/24", NextHopNodeID: "nodeB", OutInterfaceID: "if1"},
		},
	}
	if err := s.CreateNode(node, nil); err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	// First call - should remove the route
	if err := s.RemoveRoute("nodeA", "10.0.0.0/24"); err != nil {
		t.Fatalf("First RemoveRoute: %v", err)
	}

	// Verify route was removed
	got, err := s.GetRoutes("nodeA")
	if err != nil {
		t.Fatalf("GetRoutes: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("After first call: GetRoutes length = %d, want 0", len(got))
	}

	// Second call - should return error (route not found), but not corrupt state
	err2 := s.RemoveRoute("nodeA", "10.0.0.0/24")
	if err2 == nil {
		t.Fatalf("Second RemoveRoute should return error for non-existent route")
	}

	// Verify state is still consistent (no routes, node still exists)
	got2, err := s.GetRoutes("nodeA")
	if err != nil {
		t.Fatalf("GetRoutes after second call: %v", err)
	}
	if len(got2) != 0 {
		t.Fatalf("After second call: GetRoutes length = %d, want 0 (state should remain consistent)", len(got2))
	}
}

// TestRouteHelpers_ConcurrentAccess verifies that route operations
// are safe under concurrent access (no data races).
func TestRouteHelpers_ConcurrentAccess(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	// Create platform and node
	platform := &model.PlatformDefinition{ID: "platformA", Name: "Platform A"}
	if err := s.CreatePlatform(platform); err != nil {
		t.Fatalf("CreatePlatform: %v", err)
	}
	node := &model.NetworkNode{ID: "nodeA", PlatformID: "platformA"}
	if err := s.CreateNode(node, nil); err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	// Concurrent route installs with different destinations
	var wg sync.WaitGroup
	numGoroutines := 10
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			route := model.RouteEntry{
				DestinationCIDR: fmt.Sprintf("10.%d.0.0/24", id),
				NextHopNodeID:   "nodeB",
				OutInterfaceID:  "if1",
			}
			if err := s.InstallRoute("nodeA", route); err != nil {
				t.Errorf("Goroutine %d: InstallRoute failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all routes were installed
	got, err := s.GetRoutes("nodeA")
	if err != nil {
		t.Fatalf("GetRoutes: %v", err)
	}
	if len(got) != numGoroutines {
		t.Fatalf("GetRoutes length = %d, want %d", len(got), numGoroutines)
	}
}

// TestBeamHelpers_ConcurrentAccess verifies that beam operations
// are safe under concurrent access (no data races).
func TestBeamHelpers_ConcurrentAccess(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	// Create platforms
	platformA := &model.PlatformDefinition{ID: "platformA", Name: "Platform A"}
	platformB := &model.PlatformDefinition{ID: "platformB", Name: "Platform B"}
	if err := s.CreatePlatform(platformA); err != nil {
		t.Fatalf("CreatePlatform(platformA): %v", err)
	}
	if err := s.CreatePlatform(platformB); err != nil {
		t.Fatalf("CreatePlatform(platformB): %v", err)
	}

	// Create nodes
	nodeA := &model.NetworkNode{ID: "nodeA", PlatformID: "platformA"}
	nodeB := &model.NetworkNode{ID: "nodeB", PlatformID: "platformB"}
	if err := s.CreateNode(nodeA, nil); err != nil {
		t.Fatalf("CreateNode(nodeA): %v", err)
	}
	if err := s.CreateNode(nodeB, nil); err != nil {
		t.Fatalf("CreateNode(nodeB): %v", err)
	}

	// Create interfaces
	ifA := &network.NetworkInterface{
		ID:            "nodeA/ifA",
		ParentNodeID:  "nodeA",
		Medium:        network.MediumWireless,
		IsOperational: true,
	}
	ifB := &network.NetworkInterface{
		ID:            "nodeB/ifB",
		ParentNodeID:  "nodeB",
		Medium:        network.MediumWireless,
		IsOperational: true,
	}
	if err := net.AddInterface(ifA); err != nil {
		t.Fatalf("AddInterface(ifA): %v", err)
	}
	if err := net.AddInterface(ifB); err != nil {
		t.Fatalf("AddInterface(ifB): %v", err)
	}

	// Create a link with Potential status
	link := &network.NetworkLink{
		ID:         "linkAB",
		InterfaceA: "nodeA/ifA",
		InterfaceB: "nodeB/ifB",
		Medium:     network.MediumWireless,
		Status:     network.LinkStatusPotential,
		IsUp:       false,
	}
	if err := s.CreateLink(link); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	// Concurrent beam updates (should be idempotent)
	var wg sync.WaitGroup
	numGoroutines := 5
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			beam := &sbi.BeamSpec{
				NodeID:       "nodeA",
				InterfaceID:  "ifA",
				TargetNodeID: "nodeB",
				TargetIfID:   "ifB",
			}
			if err := s.ApplyBeamUpdate("nodeA", beam); err != nil {
				t.Errorf("ApplyBeamUpdate failed: %v", err)
			}
		}()
	}

	wg.Wait()

	// Verify link is Active (final state should be consistent)
	got, err := s.GetLink("linkAB")
	if err != nil {
		t.Fatalf("GetLink: %v", err)
	}
	if got.Status != network.LinkStatusActive {
		t.Fatalf("Link Status = %v, want LinkStatusActive", got.Status)
	}
	if !got.IsUp {
		t.Fatalf("Link IsUp = %v, want true", got.IsUp)
	}
}

