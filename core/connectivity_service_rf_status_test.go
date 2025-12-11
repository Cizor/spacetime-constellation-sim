package core

import (
	"testing"
)

// TestRFCompatibilityFailureDowngradesActiveStatus verifies that when RF compatibility
// fails, a link with Active status is downgraded to Potential to maintain consistency
// (Status should not remain Active when IsUp is false).
func TestRFCompatibilityFailureDowngradesActiveStatus(t *testing.T) {
	kb := NewKnowledgeBase()

	// Create incompatible transceivers (Ku vs Ka)
	trxKu := &TransceiverModel{
		ID: "trx-ku",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
		MaxRangeKm: 80000.0,
	}
	trxKa := &TransceiverModel{
		ID: "trx-ka",
		Band: FrequencyBand{
			MinGHz: 26.0,
			MaxGHz: 40.0,
		},
		MaxRangeKm: 80000.0,
	}
	if err := kb.AddTransceiverModel(trxKu); err != nil {
		t.Fatalf("AddTransceiverModel(trxKu): %v", err)
	}
	if err := kb.AddTransceiverModel(trxKa); err != nil {
		t.Fatalf("AddTransceiverModel(trxKa): %v", err)
	}

	// Create interfaces with incompatible transceivers
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifKu",
		Name:          "If-Ku",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku",
		ParentNodeID:  "nodeA",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifKu): %v", err)
	}
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifKa",
		Name:          "If-Ka",
		Medium:        MediumWireless,
		TransceiverID: "trx-ka",
		ParentNodeID:  "nodeB",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifKa): %v", err)
	}

	// Set node positions for clear LoS
	kb.SetNodeECEFPosition("nodeA", Vec3{X: EarthRadiusKm + 500, Y: 0, Z: 0})
	kb.SetNodeECEFPosition("nodeB", Vec3{X: EarthRadiusKm + 500, Y: 100, Z: 0})

	// Create a link with Active status (simulating a link that was previously activated)
	link := &NetworkLink{
		ID:         "link-ku-ka-active",
		InterfaceA: "ifKu",
		InterfaceB: "ifKa",
		Medium:     MediumWireless,
		Status:     LinkStatusActive, // Start as Active
	}
	if err := kb.AddNetworkLink(link); err != nil {
		t.Fatalf("AddNetworkLink(link-ku-ka-active): %v", err)
	}

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	// Verify that RF compatibility failure downgrades Active to Potential
	updatedLink := kb.GetNetworkLink("link-ku-ka-active")
	if updatedLink == nil {
		t.Fatalf("expected link to exist")
	}
	if updatedLink.Status != LinkStatusPotential {
		t.Fatalf("expected Status to be downgraded from Active to Potential due to RF incompatibility, got %v", updatedLink.Status)
	}
	if updatedLink.IsUp {
		t.Fatalf("expected IsUp=false due to RF incompatibility, got IsUp=true")
	}
	if updatedLink.Quality != LinkQualityDown {
		t.Fatalf("expected Quality=Down due to RF incompatibility, got %v", updatedLink.Quality)
	}
}

