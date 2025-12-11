package core

import (
	"strings"
	"testing"
)

// TestDynamicLinkExplicitDeactivation verifies that dynamic links can be
// explicitly deactivated and will not auto-activate, and that this state
// is preserved across UpdateConnectivity calls.
func TestDynamicLinkExplicitDeactivation(t *testing.T) {
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

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	// Find the dynamic link that was created
	var dynamicLink *NetworkLink
	for _, link := range kb.GetAllNetworkLinks() {
		if strings.HasPrefix(link.ID, "dyn-") {
			dynamicLink = link
			break
		}
	}
	if dynamicLink == nil {
		t.Fatalf("expected a dynamic link to be created")
	}

	// Verify link is active
	if dynamicLink.Status != LinkStatusActive {
		t.Fatalf("expected dynamic link to be Active, got %v", dynamicLink.Status)
	}
	if !dynamicLink.IsUp {
		t.Fatalf("expected dynamic link to be up")
	}

	// Explicitly deactivate the dynamic link
	dynamicLink.Status = LinkStatusPotential
	dynamicLink.IsUp = false
	dynamicLink.WasExplicitlyDeactivated = true
	if err := kb.UpdateNetworkLink(dynamicLink); err != nil {
		t.Fatalf("UpdateNetworkLink: %v", err)
	}

	// Call UpdateConnectivity - link should NOT auto-activate
	cs.UpdateConnectivity()

	// Re-fetch the link (it may have been recreated)
	var updatedLink *NetworkLink
	for _, link := range kb.GetAllNetworkLinks() {
		if strings.HasPrefix(link.ID, "dyn-") {
			updatedLink = link
			break
		}
	}
	if updatedLink == nil {
		t.Fatalf("expected dynamic link to still exist")
	}

	// Link should remain Potential (not auto-activated)
	if updatedLink.Status == LinkStatusActive {
		t.Fatalf("expected dynamic link to remain Potential after explicit deactivation, got Status=%v", updatedLink.Status)
	}
	if updatedLink.IsUp {
		t.Fatalf("expected dynamic link to be down after explicit deactivation")
	}
}

