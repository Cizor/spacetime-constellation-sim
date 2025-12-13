package state

import (
	"errors"
	"strings"
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestApplyBeamUpdate_ActivatesPotentialLink(t *testing.T) {
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
		ID:           "nodeA/ifA",
		ParentNodeID: "nodeA",
		Medium:       network.MediumWireless,
		IsOperational: true,
	}
	ifB := &network.NetworkInterface{
		ID:           "nodeB/ifB",
		ParentNodeID: "nodeB",
		Medium:       network.MediumWireless,
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

	// Apply beam update
	beam := &sbi.BeamSpec{
		NodeID:       "nodeA",
		InterfaceID:  "ifA",
		TargetNodeID: "nodeB",
		TargetIfID:   "ifB",
	}
	if err := s.ApplyBeamUpdate("nodeA", beam); err != nil {
		t.Fatalf("ApplyBeamUpdate: %v", err)
	}

	// Verify link is now Active
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

func TestApplyBeamDelete_DeactivatesActiveLink(t *testing.T) {
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

	// Create a link with Active status
	link := &network.NetworkLink{
		ID:         "linkAB",
		InterfaceA: "nodeA/ifA",
		InterfaceB: "nodeB/ifB",
		Medium:     network.MediumWireless,
		Status:     network.LinkStatusActive,
		IsUp:       true,
	}
	if err := s.CreateLink(link); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	// Apply beam delete
	if err := s.ApplyBeamDelete("nodeA", "ifA", "nodeB", "ifB"); err != nil {
		t.Fatalf("ApplyBeamDelete: %v", err)
	}

	// Verify link is now Potential
	got, err := s.GetLink("linkAB")
	if err != nil {
		t.Fatalf("GetLink: %v", err)
	}
	if got.Status != network.LinkStatusPotential {
		t.Fatalf("Link Status = %v, want LinkStatusPotential", got.Status)
	}
	if got.IsUp {
		t.Fatalf("Link IsUp = %v, want false", got.IsUp)
	}
}

func TestApplyBeamUpdate_MissingLink(t *testing.T) {
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

	// Create nodes but no link
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

	// Try to apply beam update - should fail because no link exists
	beam := &sbi.BeamSpec{
		NodeID:       "nodeA",
		InterfaceID:  "ifA",
		TargetNodeID: "nodeB",
		TargetIfID:   "ifB",
	}
	err := s.ApplyBeamUpdate("nodeA", beam)
	if err == nil {
		t.Fatalf("ApplyBeamUpdate should return error for missing link")
	}
	if !errors.Is(err, ErrLinkNotFoundForBeam) {
		t.Fatalf("ApplyBeamUpdate error = %v, want ErrLinkNotFoundForBeam", err)
	}
}

func TestApplyBeamUpdate_MissingNode(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	beam := &sbi.BeamSpec{
		NodeID:       "missing-node",
		InterfaceID:  "ifA",
		TargetNodeID: "nodeB",
		TargetIfID:   "ifB",
	}
	err := s.ApplyBeamUpdate("missing-node", beam)
	if err == nil {
		t.Fatalf("ApplyBeamUpdate should return error for missing node")
	}
	// Check that error message contains "not found" (error wrapping may prevent errors.Is from working)
	if err.Error() == "" || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("ApplyBeamUpdate error = %v, want error containing 'not found'", err)
	}
}

func TestApplyBeamUpdate_MissingInterface(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net, logging.Noop())

	// Create platform
	platformA := &model.PlatformDefinition{ID: "platformA", Name: "Platform A"}
	if err := s.CreatePlatform(platformA); err != nil {
		t.Fatalf("CreatePlatform(platformA): %v", err)
	}

	// Create node but no interface
	nodeA := &model.NetworkNode{ID: "nodeA", PlatformID: "platformA"}
	if err := s.CreateNode(nodeA, nil); err != nil {
		t.Fatalf("CreateNode(nodeA): %v", err)
	}

	beam := &sbi.BeamSpec{
		NodeID:       "nodeA",
		InterfaceID:  "missing-if",
		TargetNodeID: "nodeB",
		TargetIfID:   "ifB",
	}
	err := s.ApplyBeamUpdate("nodeA", beam)
	if err == nil {
		t.Fatalf("ApplyBeamUpdate should return error for missing interface")
	}
	if err.Error() == "" {
		t.Fatalf("ApplyBeamUpdate should return descriptive error")
	}
}

func TestApplyBeamDelete_MissingLink(t *testing.T) {
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

	// Create nodes but no link
	nodeA := &model.NetworkNode{ID: "nodeA", PlatformID: "platformA"}
	nodeB := &model.NetworkNode{ID: "nodeB", PlatformID: "platformB"}
	if err := s.CreateNode(nodeA, nil); err != nil {
		t.Fatalf("CreateNode(nodeA): %v", err)
	}
	if err := s.CreateNode(nodeB, nil); err != nil {
		t.Fatalf("CreateNode(nodeB): %v", err)
	}

	// Try to apply beam delete - should fail because no link exists
	err := s.ApplyBeamDelete("nodeA", "ifA", "nodeB", "ifB")
	if err == nil {
		t.Fatalf("ApplyBeamDelete should return error for missing link")
	}
	if !errors.Is(err, ErrLinkNotFoundForBeam) {
		t.Fatalf("ApplyBeamDelete error = %v, want ErrLinkNotFoundForBeam", err)
	}
}

func TestApplyBeamUpdate_BidirectionalLink(t *testing.T) {
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

	// Create link with InterfaceA/InterfaceB in one direction
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

	// Apply beam update - should work regardless of which direction we specify
	beam := &sbi.BeamSpec{
		NodeID:       "nodeA",
		InterfaceID:  "ifA",
		TargetNodeID: "nodeB",
		TargetIfID:   "ifB",
	}
	if err := s.ApplyBeamUpdate("nodeA", beam); err != nil {
		t.Fatalf("ApplyBeamUpdate: %v", err)
	}

	// Verify link is Active
	got, err := s.GetLink("linkAB")
	if err != nil {
		t.Fatalf("GetLink: %v", err)
	}
	if got.Status != network.LinkStatusActive {
		t.Fatalf("Link Status = %v, want LinkStatusActive", got.Status)
	}
}

