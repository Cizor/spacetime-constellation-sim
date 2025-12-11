package state

import (
	"context"
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

// newScenarioStateForTest wires up fresh in-memory KBs for each test.
func newScenarioStateForTest() (*ScenarioState, *kb.KnowledgeBase, *network.KnowledgeBase) {
	phys := kb.NewKnowledgeBase()
	netKB := network.NewKnowledgeBase()
	return NewScenarioState(phys, netKB, logging.Noop()), phys, netKB
}

func TestScenarioStateNodeAndInterfaceLifecycle(t *testing.T) {
	s, phys, netKB := newScenarioStateForTest()

	// Create a platform so the node can reference it.
	if err := s.CreatePlatform(&model.PlatformDefinition{
		ID:   "p1",
		Name: "Platform-1",
	}); err != nil {
		t.Fatalf("CreatePlatform error: %v", err)
	}

	// Node in Scope-1 KB.
	if err := phys.AddNetworkNode(&model.NetworkNode{
		ID:         "node1",
		Name:       "Node-1",
		PlatformID: "p1",
	}); err != nil {
		t.Fatalf("AddNetworkNode error: %v", err)
	}

	// Two interfaces in Scope-2 KB for that node.
	if err := netKB.AddInterface(&network.NetworkInterface{
		ID:            "ifA",
		Name:          "If-A",
		Medium:        network.MediumWired,
		ParentNodeID:  "node1",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifA) error: %v", err)
	}
	if err := netKB.AddInterface(&network.NetworkInterface{
		ID:            "ifB",
		Name:          "If-B",
		Medium:        network.MediumWireless,
		ParentNodeID:  "node1",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifB) error: %v", err)
	}

	// Verify node is present in Scope-1.
	if got := phys.GetNetworkNode("node1"); got == nil || got.PlatformID != "p1" {
		t.Fatalf("GetNetworkNode = %+v, want PlatformID p1", got)
	}

	// Verify interfaces are in Scope-2 and associated with the node.
	if a := netKB.GetNetworkInterface("ifA"); a == nil || a.ParentNodeID != "node1" {
		t.Fatalf("GetNetworkInterface(ifA) = %+v, want ParentNodeID node1", a)
	}
	if b := netKB.GetNetworkInterface("ifB"); b == nil || b.ParentNodeID != "node1" {
		t.Fatalf("GetNetworkInterface(ifB) = %+v, want ParentNodeID node1", b)
	}

	// Sanity: they show up in GetAllInterfaces.
	ifs := netKB.GetAllInterfaces()
	if len(ifs) != 2 {
		t.Fatalf("GetAllInterfaces len = %d, want 2; got %#v", len(ifs), ifs)
	}

	// Optionally exercise ClearScenario here as well.
	if err := s.ClearScenario(context.Background()); err != nil {
		t.Fatalf("ClearScenario error: %v", err)
	}
	if got := phys.GetNetworkNode("node1"); got != nil {
		t.Fatalf("GetNetworkNode after ClearScenario = %+v, want nil", got)
	}
	if got := netKB.GetNetworkInterface("ifA"); got != nil {
		t.Fatalf("GetNetworkInterface(ifA) after ClearScenario = %+v, want nil", got)
	}
	if got := netKB.GetNetworkInterface("ifB"); got != nil {
		t.Fatalf("GetNetworkInterface(ifB) after ClearScenario = %+v, want nil", got)
	}
}

func TestScenarioStateSnapshotAndClearScenario(t *testing.T) {
	s, phys, netKB := newScenarioStateForTest()

	// Build a small scenario: 1 platform, 1 node, 2 interfaces, 1 link, 1 service request.
	if err := s.CreatePlatform(&model.PlatformDefinition{
		ID:   "p1",
		Name: "P1",
	}); err != nil {
		t.Fatalf("CreatePlatform error: %v", err)
	}
	if err := phys.AddNetworkNode(&model.NetworkNode{
		ID:         "n1",
		Name:       "N1",
		PlatformID: "p1",
	}); err != nil {
		t.Fatalf("AddNetworkNode error: %v", err)
	}
	if err := netKB.AddInterface(&network.NetworkInterface{
		ID:           "ifA",
		ParentNodeID: "n1",
		Medium:       network.MediumWired,
	}); err != nil {
		t.Fatalf("AddInterface(ifA) error: %v", err)
	}
	if err := netKB.AddInterface(&network.NetworkInterface{
		ID:           "ifB",
		ParentNodeID: "n1",
		Medium:       network.MediumWireless,
	}); err != nil {
		t.Fatalf("AddInterface(ifB) error: %v", err)
	}
	if err := s.CreateLink(&network.NetworkLink{
		ID:         "link-1",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     network.MediumWireless,
	}); err != nil {
		t.Fatalf("CreateLink error: %v", err)
	}
	if err := s.CreateServiceRequest(&model.ServiceRequest{
		ID:        "sr-1",
		SrcNodeID: "n1",
		DstNodeID: "n1",
		Priority:  1,
	}); err != nil {
		t.Fatalf("CreateServiceRequest error: %v", err)
	}

	// Take a snapshot and verify contents.
	snap := s.Snapshot()
	if len(snap.Platforms) != 1 || snap.Platforms[0].ID != "p1" {
		t.Fatalf("Snapshot.Platforms = %+v, want [p1]", snap.Platforms)
	}
	if len(snap.Nodes) != 1 || snap.Nodes[0].ID != "n1" {
		t.Fatalf("Snapshot.Nodes = %+v, want [n1]", snap.Nodes)
	}
	if len(snap.Interfaces) != 2 {
		t.Fatalf("Snapshot.Interfaces len = %d, want 2", len(snap.Interfaces))
	}
	if len(snap.Links) != 1 || snap.Links[0].ID != "link-1" {
		t.Fatalf("Snapshot.Links = %+v, want [link-1]", snap.Links)
	}
	if len(snap.ServiceRequests) != 1 || snap.ServiceRequests[0].ID != "sr-1" {
		t.Fatalf("Snapshot.ServiceRequests = %+v, want [sr-1]", snap.ServiceRequests)
	}

	// Clear the scenario and ensure everything is empty in the next snapshot.
	if err := s.ClearScenario(context.Background()); err != nil {
		t.Fatalf("ClearScenario error: %v", err)
	}
	snap = s.Snapshot()
	if len(snap.Platforms) != 0 ||
		len(snap.Nodes) != 0 ||
		len(snap.Interfaces) != 0 ||
		len(snap.Links) != 0 ||
		len(snap.ServiceRequests) != 0 {
		t.Fatalf("Snapshot after ClearScenario = %+v, want all empty slices", snap)
	}
}

func TestScenarioStateRoutingOperations(t *testing.T) {
	s, phys, _ := newScenarioStateForTest()

	// Create a platform and node
	if err := s.CreatePlatform(&model.PlatformDefinition{
		ID:   "p1",
		Name: "Platform-1",
	}); err != nil {
		t.Fatalf("CreatePlatform error: %v", err)
	}

	if err := phys.AddNetworkNode(&model.NetworkNode{
		ID:         "node1",
		Name:       "Node-1",
		PlatformID: "p1",
	}); err != nil {
		t.Fatalf("AddNetworkNode error: %v", err)
	}

	// Test InstallRoute - new node with no routes
	route1 := model.RouteEntry{
		DestinationCIDR: "10.0.0.0/24",
		NextHopNodeID:   "node2",
		OutInterfaceID:  "if1",
	}
	if err := s.InstallRoute("node1", route1); err != nil {
		t.Fatalf("InstallRoute error: %v", err)
	}

	// Verify route was installed
	routes, err := s.GetRoutes("node1")
	if err != nil {
		t.Fatalf("GetRoutes error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("Expected 1 route, got %d", len(routes))
	}
	if routes[0].DestinationCIDR != "10.0.0.0/24" {
		t.Fatalf("Expected DestinationCIDR=10.0.0.0/24, got %s", routes[0].DestinationCIDR)
	}

	// Test InstallRoute - add second route with different destination
	route2 := model.RouteEntry{
		DestinationCIDR: "192.168.1.0/24",
		NextHopNodeID:   "node3",
		OutInterfaceID:  "if2",
	}
	if err := s.InstallRoute("node1", route2); err != nil {
		t.Fatalf("InstallRoute second route error: %v", err)
	}

	routes, err = s.GetRoutes("node1")
	if err != nil {
		t.Fatalf("GetRoutes error: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("Expected 2 routes, got %d", len(routes))
	}

	// Test InstallRoute - replace existing route with same DestinationCIDR
	route1Updated := model.RouteEntry{
		DestinationCIDR: "10.0.0.0/24",
		NextHopNodeID:   "node4", // Changed next hop
		OutInterfaceID:  "if3",   // Changed interface
	}
	if err := s.InstallRoute("node1", route1Updated); err != nil {
		t.Fatalf("InstallRoute replace error: %v", err)
	}

	routes, err = s.GetRoutes("node1")
	if err != nil {
		t.Fatalf("GetRoutes error: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("Expected 2 routes after replace, got %d", len(routes))
	}
	// Find the updated route
	found := false
	for _, r := range routes {
		if r.DestinationCIDR == "10.0.0.0/24" {
			if r.NextHopNodeID != "node4" || r.OutInterfaceID != "if3" {
				t.Fatalf("Route not updated correctly, got NextHop=%s OutInterface=%s", r.NextHopNodeID, r.OutInterfaceID)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Updated route not found")
	}

	// Test RemoveRoute
	if err := s.RemoveRoute("node1", "10.0.0.0/24"); err != nil {
		t.Fatalf("RemoveRoute error: %v", err)
	}

	routes, err = s.GetRoutes("node1")
	if err != nil {
		t.Fatalf("GetRoutes error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("Expected 1 route after remove, got %d", len(routes))
	}
	if routes[0].DestinationCIDR != "192.168.1.0/24" {
		t.Fatalf("Expected remaining route to be 192.168.1.0/24, got %s", routes[0].DestinationCIDR)
	}

	// Test RemoveRoute - non-existent route
	if err := s.RemoveRoute("node1", "10.0.0.0/24"); err == nil {
		t.Fatalf("RemoveRoute non-existent route expected error, got nil")
	}

	// Test GetRoutes - unknown node
	_, err = s.GetRoutes("unknown-node")
	if err == nil {
		t.Fatalf("GetRoutes unknown node expected error, got nil")
	}

	// Test InstallRoute - unknown node
	if err := s.InstallRoute("unknown-node", route1); err == nil {
		t.Fatalf("InstallRoute unknown node expected error, got nil")
	}

	// Test RemoveRoute - unknown node
	if err := s.RemoveRoute("unknown-node", "10.0.0.0/24"); err == nil {
		t.Fatalf("RemoveRoute unknown node expected error, got nil")
	}

	// Test GetRoutes returns a copy (modifying returned slice doesn't affect internal state)
	routes, err = s.GetRoutes("node1")
	if err != nil {
		t.Fatalf("GetRoutes error: %v", err)
	}
	originalLen := len(routes)
	routes = append(routes, model.RouteEntry{DestinationCIDR: "test"}) // Modify the copy

	// Verify internal state wasn't affected
	routes2, err := s.GetRoutes("node1")
	if err != nil {
		t.Fatalf("GetRoutes error: %v", err)
	}
	if len(routes2) != originalLen {
		t.Fatalf("Modifying returned slice affected internal state, expected %d routes, got %d", originalLen, len(routes2))
	}
}