package core

import (
	"testing"
)

// TestStatusRestorationOnUnimpair verifies that when a link is un-impaired,
// its original status (before impairment) is restored, not reset to Unknown.
// This fixes Bug 1: links that were Active before impairment should restore
// to Active, not go through the Unknown -> auto-activate path.
func TestStatusRestorationOnUnimpair(t *testing.T) {
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

	// Create a link with Active status (simulating a link that was activated by control plane)
	link := &NetworkLink{
		ID:         "linkAB",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     MediumWireless,
		Status:     LinkStatusActive, // Link was Active before impairment
	}
	if err := kb.AddNetworkLink(link); err != nil {
		t.Fatalf("AddNetworkLink: %v", err)
	}

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	// Verify link is Active and up
	link = kb.GetNetworkLink("linkAB")
	if link.Status != LinkStatusActive {
		t.Fatalf("expected link to be Active before impairment, got %v", link.Status)
	}
	if !link.IsUp {
		t.Fatalf("expected link to be up before impairment")
	}

	// Impair the link
	if err := kb.SetLinkImpaired("linkAB", true); err != nil {
		t.Fatalf("SetLinkImpaired(true): %v", err)
	}

	cs.UpdateConnectivity()

	// Verify link is impaired and StatusBeforeImpairment was saved
	link = kb.GetNetworkLink("linkAB")
	if link.Status != LinkStatusImpaired {
		t.Fatalf("expected Status=LinkStatusImpaired, got %v", link.Status)
	}
	if link.StatusBeforeImpairment != LinkStatusActive {
		t.Fatalf("expected StatusBeforeImpairment=LinkStatusActive, got %v", link.StatusBeforeImpairment)
	}

	// Un-impair the link
	if err := kb.SetLinkImpaired("linkAB", false); err != nil {
		t.Fatalf("SetLinkImpaired(false): %v", err)
	}

	cs.UpdateConnectivity()

	// Verify link status is restored to Active (not reset to Unknown)
	link = kb.GetNetworkLink("linkAB")
	if link.Status != LinkStatusActive {
		t.Fatalf("expected Status to be restored to LinkStatusActive after un-impairing, got %v", link.Status)
	}
	if link.StatusBeforeImpairment != LinkStatusUnknown {
		t.Fatalf("expected StatusBeforeImpairment to be cleared after restoration, got %v", link.StatusBeforeImpairment)
	}
	if !link.IsUp {
		t.Fatalf("expected link to be up after status restoration")
	}
}

