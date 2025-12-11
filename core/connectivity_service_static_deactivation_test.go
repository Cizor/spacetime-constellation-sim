package core

import (
	"testing"
)

// TestStaticLinkExplicitDeactivationDoesNotAutoActivate verifies that static links
// explicitly set to Potential via DeactivateLink do NOT auto-activate when geometry
// improves, even though they have IsStatic=true. This ensures that explicit control
// plane actions are respected.
func TestStaticLinkExplicitDeactivationDoesNotAutoActivate(t *testing.T) {
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

	// Create a static link (simulating NBI-created link)
	link := &NetworkLink{
		ID:         "static-link",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     MediumWireless,
		IsStatic:   true, // Static link (NBI-created)
		Status:     LinkStatusUnknown, // Start as Unknown
	}
	if err := kb.AddNetworkLink(link); err != nil {
		t.Fatalf("AddNetworkLink: %v", err)
	}

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	// Link should auto-activate from Unknown to Active
	link = kb.GetNetworkLink("static-link")
	if link.Status != LinkStatusActive {
		t.Fatalf("expected static link to auto-activate from Unknown, got Status=%v", link.Status)
	}

	// Now explicitly deactivate it (simulating DeactivateLink call)
	link.Status = LinkStatusPotential
	link.IsUp = false
	link.WasExplicitlyDeactivated = true // Mark as explicitly deactivated
	if err := kb.UpdateNetworkLink(link); err != nil {
		t.Fatalf("UpdateNetworkLink: %v", err)
	}

	// Verify it's now Potential
	link = kb.GetNetworkLink("static-link")
	if link.Status != LinkStatusPotential {
		t.Fatalf("expected link to be Potential after explicit deactivation, got Status=%v", link.Status)
	}

	// Call UpdateConnectivity - link should NOT auto-activate even though geometry is good
	// and IsStatic=true, because it was explicitly deactivated
	cs.UpdateConnectivity()

	link = kb.GetNetworkLink("static-link")
	if link.Status != LinkStatusPotential {
		t.Fatalf("expected static link explicitly deactivated to remain Potential, got Status=%v (should NOT auto-activate)", link.Status)
	}
	if link.IsUp {
		t.Fatalf("expected link with Potential status to be down, got IsUp=true")
	}
}

