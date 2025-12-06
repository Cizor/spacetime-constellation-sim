// internal/sim/state/state_snapshot_clear_test.go
package state

import (
	"context"
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

type resettableMotion struct {
	resets int
}

func (m *resettableMotion) Reset() {
	m.resets++
}

type resettableConnectivity struct {
	resets int
}

func (c *resettableConnectivity) Reset() {
	c.resets++
}

func TestScenarioStateSnapshot(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	netKB := network.NewKnowledgeBase()
	s := NewScenarioState(phys, netKB, logging.Noop())

	// Platforms + nodes
	if err := s.CreatePlatform(&model.PlatformDefinition{
		ID:   "p1",
		Name: "Platform-1",
	}); err != nil {
		t.Fatalf("CreatePlatform error: %v", err)
	}
	if err := phys.AddNetworkNode(&model.NetworkNode{
		ID:         "n1",
		Name:       "Node-1",
		PlatformID: "p1",
	}); err != nil {
		t.Fatalf("AddNetworkNode error: %v", err)
	}

	// Interfaces + links
	if err := netKB.AddInterface(&network.NetworkInterface{
		ID:            "ifA",
		Name:          "If-A",
		Medium:        network.MediumWired,
		ParentNodeID:  "n1",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifA) failed: %v", err)
	}
	if err := netKB.AddInterface(&network.NetworkInterface{
		ID:            "ifB",
		Name:          "If-B",
		Medium:        network.MediumWired,
		ParentNodeID:  "n1",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifB) failed: %v", err)
	}

	if err := s.CreateLink(&network.NetworkLink{
		ID:         "link-1",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     network.MediumWired,
		IsUp:       true,
	}); err != nil {
		t.Fatalf("CreateLink error: %v", err)
	}

	// ServiceRequest
	if err := s.CreateServiceRequest(&model.ServiceRequest{
		ID:        "sr-1",
		SrcNodeID: "n1",
		DstNodeID: "n1",
		Priority:  1,
	}); err != nil {
		t.Fatalf("CreateServiceRequest error: %v", err)
	}

	snap := s.Snapshot()
	if snap == nil {
		t.Fatal("Snapshot returned nil")
	}

	if len(snap.Platforms) != 1 || snap.Platforms[0].ID != "p1" {
		t.Fatalf("Snapshot.Platforms = %+v, want [p1]", snap.Platforms)
	}
	if len(snap.Nodes) != 1 || snap.Nodes[0].ID != "n1" {
		t.Fatalf("Snapshot.Nodes = %+v, want [n1]", snap.Nodes)
	}
	if len(snap.Interfaces) != 2 {
		t.Fatalf("Snapshot.Interfaces len = %d, want 2", len(snap.Interfaces))
	}
	if got := snap.InterfacesByNode["n1"]; len(got) != 2 {
		t.Fatalf("Snapshot.InterfacesByNode[n1] len = %d, want 2; got %#v", len(got), snap.InterfacesByNode["n1"])
	}
	if len(snap.Links) != 1 || snap.Links[0].ID != "link-1" {
		t.Fatalf("Snapshot.Links = %+v, want [link-1]", snap.Links)
	}
	if len(snap.ServiceRequests) != 1 || snap.ServiceRequests[0].ID != "sr-1" {
		t.Fatalf("Snapshot.ServiceRequests = %+v, want [sr-1]", snap.ServiceRequests)
	}
}

func TestScenarioStateClearScenario(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	netKB := network.NewKnowledgeBase()
	motion := &resettableMotion{}
	connectivity := &resettableConnectivity{}
	s := NewScenarioState(phys, netKB, logging.Noop(), WithMotionModel(motion), WithConnectivityService(connectivity))

	// Populate a minimal scenario.
	if err := s.CreatePlatform(&model.PlatformDefinition{
		ID: "p1",
	}); err != nil {
		t.Fatalf("CreatePlatform error: %v", err)
	}
	if err := phys.AddNetworkNode(&model.NetworkNode{
		ID:         "n1",
		PlatformID: "p1",
	}); err != nil {
		t.Fatalf("AddNetworkNode error: %v", err)
	}
	if err := netKB.AddInterface(&network.NetworkInterface{
		ID:            "ifA",
		Medium:        network.MediumWired,
		ParentNodeID:  "n1",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifA) failed: %v", err)
	}
	if err := s.CreateLink(&network.NetworkLink{
		ID:         "link-1",
		InterfaceA: "ifA",
		InterfaceB: "ifA",
		Medium:     network.MediumWired,
		IsUp:       true,
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

	// Sanity checks before clearing.
	if n := len(s.ListPlatforms()); n == 0 {
		t.Fatalf("precondition: expected non-empty platforms")
	}
	if n := len(s.ListLinks()); n == 0 {
		t.Fatalf("precondition: expected non-empty links")
	}
	if n := len(s.ListServiceRequests()); n == 0 {
		t.Fatalf("precondition: expected non-empty service requests")
	}

	if err := s.ClearScenario(context.Background()); err != nil {
		t.Fatalf("ClearScenario error: %v", err)
	}

	if motion.resets != 1 {
		t.Fatalf("motion.Reset call count = %d, want 1", motion.resets)
	}
	if connectivity.resets != 1 {
		t.Fatalf("connectivity.Reset call count = %d, want 1", connectivity.resets)
	}

	// ScenarioState views should now be empty.
	if got := s.ListPlatforms(); len(got) != 0 {
		t.Fatalf("ListPlatforms after ClearScenario = %+v, want empty", got)
	}
	if got := s.ListLinks(); len(got) != 0 {
		t.Fatalf("ListLinks after ClearScenario = %+v, want empty", got)
	}
	if got := s.ListServiceRequests(); len(got) != 0 {
		t.Fatalf("ListServiceRequests after ClearScenario = %+v, want empty", got)
	}

	// Underlying KBs should also be cleared for platforms/nodes and interfaces/links.
	if plats := phys.ListPlatforms(); len(plats) != 0 {
		t.Fatalf("physKB platforms after ClearScenario = %+v, want empty", plats)
	}
	if nodes := phys.ListNetworkNodes(); len(nodes) != 0 {
		t.Fatalf("physKB nodes after ClearScenario = %+v, want empty", nodes)
	}
	if ifs := netKB.GetAllInterfaces(); len(ifs) != 0 {
		t.Fatalf("netKB interfaces after ClearScenario = %+v, want empty", ifs)
	}
	if links := netKB.GetAllNetworkLinks(); len(links) != 0 {
		t.Fatalf("netKB links after ClearScenario = %+v, want empty", links)
	}

	// Snapshot should also reflect the cleared scenario.
	snap := s.Snapshot()
	if len(snap.Platforms) != 0 ||
		len(snap.Nodes) != 0 ||
		len(snap.Interfaces) != 0 ||
		len(snap.InterfacesByNode) != 0 ||
		len(snap.Links) != 0 ||
		len(snap.ServiceRequests) != 0 {
		t.Fatalf("Snapshot after ClearScenario = %+v, want all empty collections", snap)
	}
}
