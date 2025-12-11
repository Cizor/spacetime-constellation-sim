package core

import (
	"testing"
)

// TestDeactivatedLinkUnimpairDoesNotAutoActivate verifies that when a link
// is explicitly deactivated, then impaired, then un-impaired, it does NOT
// auto-activate. The WasExplicitlyDeactivated flag should be preserved and
// respected even after impairment/un-impairment cycles.
func TestDeactivatedLinkUnimpairDoesNotAutoActivate(t *testing.T) {
	kb := NewKnowledgeBase()

	// Create transceiver model
	trx := &TransceiverModel{
		ID: "trx-test",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
		MaxRangeKm: 100000.0,
	}
	if err := kb.AddTransceiverModel(trx); err != nil {
		t.Fatalf("AddTransceiverModel: %v", err)
	}

	// Create interfaces
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifA",
		Name:          "If-A",
		Medium:        MediumWireless,
		TransceiverID: "trx-test",
		ParentNodeID:  "nodeA",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifA): %v", err)
	}
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifB",
		Name:          "If-B",
		Medium:        MediumWireless,
		TransceiverID: "trx-test",
		ParentNodeID:  "nodeB",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifB): %v", err)
	}

	// Set node positions for clear LoS
	kb.SetNodeECEFPosition("nodeA", Vec3{X: EarthRadiusKm + 500, Y: 0, Z: 0})
	kb.SetNodeECEFPosition("nodeB", Vec3{X: EarthRadiusKm + 500, Y: 100, Z: 0})

	// Create a link with Active status
	link := &NetworkLink{
		ID:         "linkAB",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     MediumWireless,
		Status:     LinkStatusActive,
	}
	if err := kb.AddNetworkLink(link); err != nil {
		t.Fatalf("AddNetworkLink: %v", err)
	}

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	// Verify link is up
	link = kb.GetNetworkLink("linkAB")
	if !link.IsUp {
		t.Fatalf("expected link to be up with good geometry")
	}

	// Explicitly deactivate the link
	link.Status = LinkStatusPotential
	link.IsUp = false
	link.WasExplicitlyDeactivated = true
	if err := kb.UpdateNetworkLink(link); err != nil {
		t.Fatalf("UpdateNetworkLink: %v", err)
	}

	cs.UpdateConnectivity()

	// Verify link is deactivated
	link = kb.GetNetworkLink("linkAB")
	if link.Status != LinkStatusPotential {
		t.Fatalf("expected link to be Potential after explicit deactivation, got %v", link.Status)
	}
	if !link.WasExplicitlyDeactivated {
		t.Fatalf("expected WasExplicitlyDeactivated=true after explicit deactivation")
	}

	// Now impair the link
	if err := kb.SetLinkImpaired("linkAB", true); err != nil {
		t.Fatalf("SetLinkImpaired(true): %v", err)
	}

	cs.UpdateConnectivity()

	// Verify link is impaired
	link = kb.GetNetworkLink("linkAB")
	if link.Status != LinkStatusImpaired {
		t.Fatalf("expected Status=LinkStatusImpaired, got %v", link.Status)
	}
	if !link.IsImpaired {
		t.Fatalf("expected IsImpaired=true")
	}
	// WasExplicitlyDeactivated should still be preserved
	if !link.WasExplicitlyDeactivated {
		t.Fatalf("expected WasExplicitlyDeactivated=true to be preserved after impairment")
	}

	// Now un-impair the link
	if err := kb.SetLinkImpaired("linkAB", false); err != nil {
		t.Fatalf("SetLinkImpaired(false): %v", err)
	}

	cs.UpdateConnectivity()

	// Verify link does NOT auto-activate (should remain Potential)
	link = kb.GetNetworkLink("linkAB")
	if link.IsImpaired {
		t.Fatalf("expected IsImpaired=false after un-impairing")
	}
	if link.Status == LinkStatusActive {
		t.Fatalf("expected link to remain Potential after un-impairing (was explicitly deactivated), got Status=%v", link.Status)
	}
	if link.Status != LinkStatusPotential {
		t.Fatalf("expected Status=LinkStatusPotential after un-impairing (was explicitly deactivated), got %v", link.Status)
	}
	if !link.WasExplicitlyDeactivated {
		t.Fatalf("expected WasExplicitlyDeactivated=true to be preserved")
	}
	if link.IsUp {
		t.Fatalf("expected link to remain down after un-impairing (was explicitly deactivated)")
	}
}

