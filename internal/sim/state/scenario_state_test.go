package state

import (
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

// newScenarioStateForTest wires up fresh in-memory KBs for each test.
func newScenarioStateForTest() (*ScenarioState, *kb.KnowledgeBase, *network.KnowledgeBase) {
	phys := kb.NewKnowledgeBase()
	netKB := network.NewKnowledgeBase()
	return NewScenarioState(phys, netKB), phys, netKB
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
	if err := s.ClearScenario(); err != nil {
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
		Type:      "video",
		SrcNodeID: "n1",
		DstNodeID: "n1",
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
	if err := s.ClearScenario(); err != nil {
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
