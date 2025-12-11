package core

import (
	"strings"
	"testing"
)

// TestDynamicLinkDeactivationStatePersistence verifies that when a dynamic link
// is explicitly deactivated and then UpdateConnectivity is called, the deactivation
// state is preserved across the rebuild cycle.
func TestDynamicLinkDeactivationStatePersistence(t *testing.T) {
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

	// Find the dynamic link
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

	// Explicitly deactivate the dynamic link
	dynamicLink.Status = LinkStatusPotential
	dynamicLink.IsUp = false
	dynamicLink.WasExplicitlyDeactivated = true
	if err := kb.UpdateNetworkLink(dynamicLink); err != nil {
		t.Fatalf("UpdateNetworkLink: %v", err)
	}

	// Verify deactivation state is set
	linkID := dynamicLink.ID
	dynamicLink = kb.GetNetworkLink(linkID)
	if !dynamicLink.WasExplicitlyDeactivated {
		t.Fatalf("expected WasExplicitlyDeactivated=true after explicit deactivation")
	}

	// Call UpdateConnectivity - this will rebuild dynamic links
	cs.UpdateConnectivity()

	// Re-fetch the link after rebuild
	updatedLink := kb.GetNetworkLink(linkID)
	if updatedLink == nil {
		t.Fatalf("expected dynamic link to still exist after rebuild")
	}

	// Verify deactivation state was preserved
	if !updatedLink.WasExplicitlyDeactivated {
		t.Fatalf("expected WasExplicitlyDeactivated=true to be preserved after rebuild, got false")
	}
	if updatedLink.Status == LinkStatusActive {
		t.Fatalf("expected link to remain Potential after rebuild (was explicitly deactivated), got Status=%v", updatedLink.Status)
	}
	if updatedLink.IsUp {
		t.Fatalf("expected link to remain down after rebuild (was explicitly deactivated)")
	}
}

