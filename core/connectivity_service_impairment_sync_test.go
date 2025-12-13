package core

import (
	"testing"
)

// TestIsImpairedSyncsToStatus verifies that when IsImpaired is set to true
// (via SetLinkImpaired), the Status field is synced to LinkStatusImpaired
// during evaluateLink, maintaining bidirectional consistency.
func TestIsImpairedSyncsToStatus(t *testing.T) {
	kb := NewKnowledgeBase()

	// Create interfaces
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifA",
		Name:          "If-A",
		Medium:        MediumWireless,
		ParentNodeID:  "nodeA",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifA): %v", err)
	}
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifB",
		Name:          "If-B",
		Medium:        MediumWireless,
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
		Status:     LinkStatusActive, // Start as Active
	}
	if err := kb.AddNetworkLink(link); err != nil {
		t.Fatalf("AddNetworkLink(linkAB): %v", err)
	}

	// Set link as impaired using SetLinkImpaired (sets IsImpaired = true)
	if err := kb.SetLinkImpaired("linkAB", true); err != nil {
		t.Fatalf("SetLinkImpaired(linkAB, true): %v", err)
	}

	// Verify IsImpaired is set but Status hasn't been synced yet
	link = kb.GetNetworkLink("linkAB")
	if link == nil {
		t.Fatalf("expected link to exist")
	}
	if !link.IsImpaired {
		t.Fatalf("expected IsImpaired=true after SetLinkImpaired")
	}
	if link.Status == LinkStatusImpaired {
		t.Fatalf("Status should not be Impaired yet (before evaluateLink)")
	}

	// Run evaluateLink (via UpdateConnectivity)
	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	// Verify that Status was synced to Impaired
	updatedLink := kb.GetNetworkLink("linkAB")
	if updatedLink == nil {
		t.Fatalf("expected link to exist")
	}
	if !updatedLink.IsImpaired {
		t.Fatalf("expected IsImpaired=true after evaluateLink")
	}
	if updatedLink.Status != LinkStatusImpaired {
		t.Fatalf("expected Status to be synced to LinkStatusImpaired when IsImpaired=true, got %v", updatedLink.Status)
	}
	if updatedLink.IsUp {
		t.Fatalf("expected IsUp=false for impaired link, got IsUp=true")
	}
}

