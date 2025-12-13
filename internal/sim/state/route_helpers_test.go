package state

import (
	"strings"
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestApplySetRoute_InstallsNewRoute(t *testing.T) {
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

	// Apply route
	route := model.RouteEntry{
		DestinationCIDR: "10.0.0.0/24",
		NextHopNodeID:   "nodeB",
		OutInterfaceID:  "if1",
	}
	if err := s.ApplySetRoute("nodeA", route); err != nil {
		t.Fatalf("ApplySetRoute: %v", err)
	}

	// Verify route was installed
	got, err := s.GetRoutes("nodeA")
	if err != nil {
		t.Fatalf("GetRoutes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetRoutes length = %d, want 1", len(got))
	}
	if got[0].DestinationCIDR != "10.0.0.0/24" {
		t.Fatalf("Route DestinationCIDR = %q, want %q", got[0].DestinationCIDR, "10.0.0.0/24")
	}
	if got[0].NextHopNodeID != "nodeB" {
		t.Fatalf("Route NextHopNodeID = %q, want %q", got[0].NextHopNodeID, "nodeB")
	}
	if got[0].OutInterfaceID != "if1" {
		t.Fatalf("Route OutInterfaceID = %q, want %q", got[0].OutInterfaceID, "if1")
	}
}

func TestApplySetRoute_ReplacesExistingRoute(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	// Create platform and node
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

	// Apply route with same DestinationCIDR but different NextHopNodeID
	route := model.RouteEntry{
		DestinationCIDR: "10.0.0.0/24",
		NextHopNodeID:   "nodeC", // Different next hop
		OutInterfaceID:  "if2",   // Different interface
	}
	if err := s.ApplySetRoute("nodeA", route); err != nil {
		t.Fatalf("ApplySetRoute: %v", err)
	}

	// Verify route was replaced (not added)
	got, err := s.GetRoutes("nodeA")
	if err != nil {
		t.Fatalf("GetRoutes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetRoutes length = %d, want 1 (replaced, not added)", len(got))
	}
	if got[0].NextHopNodeID != "nodeC" {
		t.Fatalf("Route NextHopNodeID = %q, want %q (replaced)", got[0].NextHopNodeID, "nodeC")
	}
	if got[0].OutInterfaceID != "if2" {
		t.Fatalf("Route OutInterfaceID = %q, want %q (replaced)", got[0].OutInterfaceID, "if2")
	}
}

func TestApplyDeleteRoute_RemovesExistingRoute(t *testing.T) {
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

	// Delete one route
	if err := s.ApplyDeleteRoute("nodeA", "10.0.0.0/24"); err != nil {
		t.Fatalf("ApplyDeleteRoute: %v", err)
	}

	// Verify route was removed
	got, err := s.GetRoutes("nodeA")
	if err != nil {
		t.Fatalf("GetRoutes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetRoutes length = %d, want 1", len(got))
	}
	if got[0].DestinationCIDR != "10.1.0.0/24" {
		t.Fatalf("Remaining route DestinationCIDR = %q, want %q", got[0].DestinationCIDR, "10.1.0.0/24")
	}
}

func TestApplyDeleteRoute_NonExistentRoute(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	// Create platform and node with one route
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

	// Try to delete non-existent route - should return error (strict mode)
	err := s.ApplyDeleteRoute("nodeA", "10.1.0.0/16")
	if err == nil {
		t.Fatalf("ApplyDeleteRoute should return error for non-existent route")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("ApplyDeleteRoute error = %q, want error containing 'not found'", err.Error())
	}

	// Verify original route still exists
	got, err := s.GetRoutes("nodeA")
	if err != nil {
		t.Fatalf("GetRoutes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetRoutes length = %d, want 1 (route should remain)", len(got))
	}
}

func TestApplySetRoute_UnknownNode(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	route := model.RouteEntry{
		DestinationCIDR: "10.0.0.0/24",
		NextHopNodeID:   "nodeB",
		OutInterfaceID:  "if1",
	}
	err := s.ApplySetRoute("unknown-node", route)
	if err == nil {
		t.Fatalf("ApplySetRoute should return error for unknown node")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("ApplySetRoute error = %q, want error containing 'not found'", err.Error())
	}
}

func TestApplyDeleteRoute_UnknownNode(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	err := s.ApplyDeleteRoute("unknown-node", "10.0.0.0/24")
	if err == nil {
		t.Fatalf("ApplyDeleteRoute should return error for unknown node")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("ApplyDeleteRoute error = %q, want error containing 'not found'", err.Error())
	}
}

func TestApplySetRoute_EmptyNodeID(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	route := model.RouteEntry{
		DestinationCIDR: "10.0.0.0/24",
		NextHopNodeID:   "nodeB",
		OutInterfaceID:  "if1",
	}
	err := s.ApplySetRoute("", route)
	if err == nil {
		t.Fatalf("ApplySetRoute should return error for empty nodeID")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("ApplySetRoute error = %q, want error containing 'must not be empty'", err.Error())
	}
}

func TestApplySetRoute_EmptyDestinationCIDR(t *testing.T) {
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
		DestinationCIDR: "", // Empty
		NextHopNodeID:   "nodeB",
		OutInterfaceID:  "if1",
	}
	err := s.ApplySetRoute("nodeA", route)
	if err == nil {
		t.Fatalf("ApplySetRoute should return error for empty DestinationCIDR")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("ApplySetRoute error = %q, want error containing 'must not be empty'", err.Error())
	}
}

func TestApplyDeleteRoute_EmptyNodeID(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	err := s.ApplyDeleteRoute("", "10.0.0.0/24")
	if err == nil {
		t.Fatalf("ApplyDeleteRoute should return error for empty nodeID")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("ApplyDeleteRoute error = %q, want error containing 'must not be empty'", err.Error())
	}
}

func TestApplyDeleteRoute_EmptyDestCIDR(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	err := s.ApplyDeleteRoute("nodeA", "")
	if err == nil {
		t.Fatalf("ApplyDeleteRoute should return error for empty destCIDR")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("ApplyDeleteRoute error = %q, want error containing 'must not be empty'", err.Error())
	}
}

