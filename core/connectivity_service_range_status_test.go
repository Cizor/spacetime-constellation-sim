package core

import (
	"testing"
)

// TestRangeFailureDowngradesActiveStatus verifies that when a link exceeds
// maximum range, a link with Active status is downgraded to Potential to
// maintain consistency (Status should not remain Active when IsUp is false).
func TestRangeFailureDowngradesActiveStatus(t *testing.T) {
	kb := NewKnowledgeBase()

	// Create transceiver model with limited range
	trx := &TransceiverModel{
		ID: "trx-limited-range",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
		MaxRangeKm: 100.0, // Limited range - will fail range check
	}
	if err := kb.AddTransceiverModel(trx); err != nil {
		t.Fatalf("AddTransceiverModel: %v", err)
	}

	// Create interfaces
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifA",
		Name:          "If-A",
		Medium:        MediumWireless,
		TransceiverID: "trx-limited-range",
		ParentNodeID:  "nodeA",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifA): %v", err)
	}
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifB",
		Name:          "If-B",
		Medium:        MediumWireless,
		TransceiverID: "trx-limited-range",
		ParentNodeID:  "nodeB",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifB): %v", err)
	}

	// Set node positions far apart (exceeding max range)
	// Distance ~1000km, but max range is only 100km
	kb.SetNodeECEFPosition("nodeA", Vec3{X: EarthRadiusKm + 500, Y: 0, Z: 0})
	kb.SetNodeECEFPosition("nodeB", Vec3{X: EarthRadiusKm + 500, Y: 1000, Z: 0}) // ~1000km apart

	// Create a link with Active status (simulating a link that was previously activated)
	link := &NetworkLink{
		ID:         "linkAB-active",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     MediumWireless,
		Status:     LinkStatusActive, // Start as Active
	}
	if err := kb.AddNetworkLink(link); err != nil {
		t.Fatalf("AddNetworkLink(linkAB-active): %v", err)
	}

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	// Verify that range failure downgrades Active to Potential
	updatedLink := kb.GetNetworkLink("linkAB-active")
	if updatedLink == nil {
		t.Fatalf("expected link to exist")
	}
	if updatedLink.Status != LinkStatusPotential {
		t.Fatalf("expected Status to be downgraded from Active to Potential due to range failure, got %v", updatedLink.Status)
	}
	if updatedLink.IsUp {
		t.Fatalf("expected IsUp=false due to range failure, got IsUp=true")
	}
	if updatedLink.Quality != LinkQualityDown {
		t.Fatalf("expected Quality=Down due to range failure, got %v", updatedLink.Quality)
	}
}

