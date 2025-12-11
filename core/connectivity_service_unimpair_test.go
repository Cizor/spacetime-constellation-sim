package core

import (
	"testing"
)

// TestUnimpairLink verifies that SetLinkImpaired(linkID, false) correctly
// un-impairs a link, allowing it to be re-evaluated normally.
func TestUnimpairLink(t *testing.T) {
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

	// Impair the link
	if err := kb.SetLinkImpaired("linkAB", true); err != nil {
		t.Fatalf("SetLinkImpaired(true): %v", err)
	}

	cs.UpdateConnectivity()

	// Verify link is impaired
	link = kb.GetNetworkLink("linkAB")
	if link.IsUp {
		t.Fatalf("expected link to be down when impaired")
	}
	if !link.IsImpaired {
		t.Fatalf("expected IsImpaired=true")
	}
	if link.Status != LinkStatusImpaired {
		t.Fatalf("expected Status=LinkStatusImpaired, got %v", link.Status)
	}

	// Un-impair the link
	if err := kb.SetLinkImpaired("linkAB", false); err != nil {
		t.Fatalf("SetLinkImpaired(false): %v", err)
	}

	cs.UpdateConnectivity()

	// Verify link is no longer impaired and can be re-evaluated
	link = kb.GetNetworkLink("linkAB")
	if link.IsImpaired {
		t.Fatalf("expected IsImpaired=false after un-impairing")
	}
	if link.Status == LinkStatusImpaired {
		t.Fatalf("expected Status to be cleared from Impaired after un-impairing, got %v", link.Status)
	}
	// Link should be re-evaluated and come up if geometry allows
	if !link.IsUp {
		t.Fatalf("expected link to come up after un-impairing (geometry is good)")
	}
	if link.Status != LinkStatusActive {
		t.Fatalf("expected Status=LinkStatusActive after un-impairing, got %v", link.Status)
	}
}

