package state

import (
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestGetRoute_RouteExists(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	// Create platform and node with routes
	platform := &model.PlatformDefinition{ID: "platformA", Name: "Platform A"}
	if err := s.CreatePlatform(platform); err != nil {
		t.Fatalf("CreatePlatform: %v", err)
	}
	node := &model.NetworkNode{
		ID:         "nodeA",
		PlatformID: "platformA",
		Routes: []model.RouteEntry{
			{DestinationCIDR: "10.0.0.0/24", NextHopNodeID: "nodeB", OutInterfaceID: "if1"},
			{DestinationCIDR: "10.1.0.0/24", NextHopNodeID: "nodeC", OutInterfaceID: "if2"},
		},
	}
	if err := s.CreateNode(node, nil); err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	// Get route
	r, ok := s.GetRoute("nodeA", "10.0.0.0/24")
	if !ok {
		t.Fatalf("GetRoute returned ok=false, want true")
	}
	if r == nil {
		t.Fatalf("GetRoute returned nil route, want non-nil")
	}
	if r.DestinationCIDR != "10.0.0.0/24" {
		t.Fatalf("Route DestinationCIDR = %q, want %q", r.DestinationCIDR, "10.0.0.0/24")
	}
	if r.NextHopNodeID != "nodeB" {
		t.Fatalf("Route NextHopNodeID = %q, want %q", r.NextHopNodeID, "nodeB")
	}
	if r.OutInterfaceID != "if1" {
		t.Fatalf("Route OutInterfaceID = %q, want %q", r.OutInterfaceID, "if1")
	}
}

func TestGetRoute_NoRouteForDestination(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	// Create platform and node with routes
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

	// Try to get non-existent route
	r, ok := s.GetRoute("nodeA", "10.1.0.0/16")
	if ok {
		t.Fatalf("GetRoute returned ok=true, want false")
	}
	if r != nil {
		t.Fatalf("GetRoute returned non-nil route, want nil")
	}
}

func TestGetRoute_UnknownNode(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	// Try to get route for unknown node
	r, ok := s.GetRoute("unknown-node", "10.0.0.0/24")
	if ok {
		t.Fatalf("GetRoute returned ok=true for unknown node, want false")
	}
	if r != nil {
		t.Fatalf("GetRoute returned non-nil route for unknown node, want nil")
	}
}

func TestGetRoute_ReturnsCopy(t *testing.T) {
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

	// Get route and modify it
	r1, ok := s.GetRoute("nodeA", "10.0.0.0/24")
	if !ok {
		t.Fatalf("GetRoute returned ok=false")
	}
	r1.NextHopNodeID = "modified-node"

	// Get route again - should be unchanged
	r2, ok := s.GetRoute("nodeA", "10.0.0.0/24")
	if !ok {
		t.Fatalf("GetRoute returned ok=false on second call")
	}
	if r2.NextHopNodeID != "nodeB" {
		t.Fatalf("Route NextHopNodeID = %q after modification, want %q (should be unchanged)", r2.NextHopNodeID, "nodeB")
	}
}

func TestGetRoute_ExactMatch(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	// Create platform and node with routes
	platform := &model.PlatformDefinition{ID: "platformA", Name: "Platform A"}
	if err := s.CreatePlatform(platform); err != nil {
		t.Fatalf("CreatePlatform: %v", err)
	}
	node := &model.NetworkNode{
		ID:         "nodeA",
		PlatformID: "platformA",
		Routes: []model.RouteEntry{
			{DestinationCIDR: "10.0.0.0/24", NextHopNodeID: "nodeB", OutInterfaceID: "if1"},
			{DestinationCIDR: "10.0.0.0/16", NextHopNodeID: "nodeC", OutInterfaceID: "if2"},
		},
	}
	if err := s.CreateNode(node, nil); err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	// Get route with /24 prefix - should match exactly
	r, ok := s.GetRoute("nodeA", "10.0.0.0/24")
	if !ok {
		t.Fatalf("GetRoute returned ok=false for exact match")
	}
	if r.DestinationCIDR != "10.0.0.0/24" {
		t.Fatalf("Route DestinationCIDR = %q, want %q (exact match)", r.DestinationCIDR, "10.0.0.0/24")
	}
	if r.NextHopNodeID != "nodeB" {
		t.Fatalf("Route NextHopNodeID = %q, want %q", r.NextHopNodeID, "nodeB")
	}

	// Get route with /16 prefix - should match exactly (different route)
	r2, ok := s.GetRoute("nodeA", "10.0.0.0/16")
	if !ok {
		t.Fatalf("GetRoute returned ok=false for /16 exact match")
	}
	if r2.DestinationCIDR != "10.0.0.0/16" {
		t.Fatalf("Route DestinationCIDR = %q, want %q (exact match)", r2.DestinationCIDR, "10.0.0.0/16")
	}
	if r2.NextHopNodeID != "nodeC" {
		t.Fatalf("Route NextHopNodeID = %q, want %q", r2.NextHopNodeID, "nodeC")
	}
}

