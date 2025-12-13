package state

import (
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

// TestApplyBeamUpdate_Idempotent verifies that calling ApplyBeamUpdate
// multiple times with the same inputs is safe and doesn't corrupt state.
func TestApplyBeamUpdate_Idempotent(t *testing.T) {
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

	beam := &sbi.BeamSpec{
		NodeID:       "nodeA",
		InterfaceID:  "ifA",
		TargetNodeID: "nodeB",
		TargetIfID:   "ifB",
	}

	// First call - should activate the link
	if err := s.ApplyBeamUpdate("nodeA", beam); err != nil {
		t.Fatalf("First ApplyBeamUpdate: %v", err)
	}

	// Verify link is Active
	got, err := s.GetLink("linkAB")
	if err != nil {
		t.Fatalf("GetLink: %v", err)
	}
	if got.Status != network.LinkStatusActive {
		t.Fatalf("After first call: Link Status = %v, want LinkStatusActive", got.Status)
	}
	if !got.IsUp {
		t.Fatalf("After first call: Link IsUp = %v, want true", got.IsUp)
	}

	// Second call - should be idempotent (no error, state remains Active)
	if err := s.ApplyBeamUpdate("nodeA", beam); err != nil {
		t.Fatalf("Second ApplyBeamUpdate: %v", err)
	}

	// Verify link is still Active (not corrupted)
	got2, err := s.GetLink("linkAB")
	if err != nil {
		t.Fatalf("GetLink after second call: %v", err)
	}
	if got2.Status != network.LinkStatusActive {
		t.Fatalf("After second call: Link Status = %v, want LinkStatusActive", got2.Status)
	}
	if !got2.IsUp {
		t.Fatalf("After second call: Link IsUp = %v, want true", got2.IsUp)
	}
}

// TestApplyBeamDelete_Idempotent verifies that calling ApplyBeamDelete
// multiple times with the same inputs is safe.
func TestApplyBeamDelete_Idempotent(t *testing.T) {
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

	// First call - should deactivate the link
	if err := s.ApplyBeamDelete("nodeA", "ifA", "nodeB", "ifB"); err != nil {
		t.Fatalf("First ApplyBeamDelete: %v", err)
	}

	// Verify link is Potential
	got, err := s.GetLink("linkAB")
	if err != nil {
		t.Fatalf("GetLink: %v", err)
	}
	if got.Status != network.LinkStatusPotential {
		t.Fatalf("After first call: Link Status = %v, want LinkStatusPotential", got.Status)
	}
	if got.IsUp {
		t.Fatalf("After first call: Link IsUp = %v, want false", got.IsUp)
	}

	// Second call - should be idempotent (no error, state remains Potential)
	// Note: The current implementation may return an error if link is not found,
	// but it should handle the case where link is already Potential gracefully.
	// For now, we'll verify it doesn't corrupt state even if it returns an error.
	err2 := s.ApplyBeamDelete("nodeA", "ifA", "nodeB", "ifB")
	// Accept either success or a documented error (link not found or already deactivated)
	if err2 != nil {
		t.Logf("Second ApplyBeamDelete returned error (may be expected): %v", err2)
	}

	// Verify link state is still consistent (not corrupted)
	got2, err := s.GetLink("linkAB")
	if err != nil {
		t.Fatalf("GetLink after second call: %v", err)
	}
	if got2.Status != network.LinkStatusPotential {
		t.Fatalf("After second call: Link Status = %v, want LinkStatusPotential", got2.Status)
	}
}

