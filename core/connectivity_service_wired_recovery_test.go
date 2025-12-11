package core

import (
	"testing"
)

// TestWiredLinkRecoveryFromPotential verifies that wired links with
// LinkStatusPotential (that were not explicitly deactivated) can recover
// to Active status, matching the recovery behavior of wireless links.
func TestWiredLinkRecoveryFromPotential(t *testing.T) {
	kb := NewKnowledgeBase()

	// Create wired interfaces
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifA",
		Name:          "If-A",
		Medium:        MediumWired,
		ParentNodeID:  "nodeA",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifA): %v", err)
	}
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifB",
		Name:          "If-B",
		Medium:        MediumWired,
		ParentNodeID:  "nodeB",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifB): %v", err)
	}

	// Create a wired link with Potential status (not explicitly deactivated)
	// This simulates a link that was auto-downgraded to Potential
	link := &NetworkLink{
		ID:         "linkAB-wired",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     MediumWired,
		Status:     LinkStatusPotential, // Auto-downgraded, not explicitly deactivated
		// WasExplicitlyDeactivated is false (default)
	}
	if err := kb.AddNetworkLink(link); err != nil {
		t.Fatalf("AddNetworkLink(linkAB-wired): %v", err)
	}

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	// Verify that Potential wired links (not explicitly deactivated) auto-activate
	// This matches the recovery behavior of wireless links
	updatedLink := kb.GetNetworkLink("linkAB-wired")
	if updatedLink == nil {
		t.Fatalf("expected link to exist")
	}
	if updatedLink.Status != LinkStatusActive {
		t.Fatalf("expected Status to auto-activate from Potential to Active (was not explicitly deactivated), got %v", updatedLink.Status)
	}
	if !updatedLink.IsUp {
		t.Fatalf("expected IsUp=true for Active wired link, got IsUp=false")
	}
	if updatedLink.Quality != LinkQualityExcellent {
		t.Fatalf("expected Quality=Excellent for wired link, got %v", updatedLink.Quality)
	}
}

// TestWiredLinkExplicitDeactivationDoesNotRecover verifies that wired links
// that were explicitly deactivated (WasExplicitlyDeactivated=true) do NOT
// auto-activate, even if they have Potential status.
func TestWiredLinkExplicitDeactivationDoesNotRecover(t *testing.T) {
	kb := NewKnowledgeBase()

	// Create wired interfaces
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifA",
		Name:          "If-A",
		Medium:        MediumWired,
		ParentNodeID:  "nodeA",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifA): %v", err)
	}
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifB",
		Name:          "If-B",
		Medium:        MediumWired,
		ParentNodeID:  "nodeB",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifB): %v", err)
	}

	// Create a wired link with Potential status that was explicitly deactivated
	link := &NetworkLink{
		ID:                       "linkAB-wired",
		InterfaceA:               "ifA",
		InterfaceB:               "ifB",
		Medium:                   MediumWired,
		Status:                   LinkStatusPotential,
		WasExplicitlyDeactivated: true, // Explicitly deactivated
	}
	if err := kb.AddNetworkLink(link); err != nil {
		t.Fatalf("AddNetworkLink(linkAB-wired): %v", err)
	}

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	// Verify that explicitly deactivated wired links do NOT auto-activate
	updatedLink := kb.GetNetworkLink("linkAB-wired")
	if updatedLink == nil {
		t.Fatalf("expected link to exist")
	}
	if updatedLink.Status != LinkStatusPotential {
		t.Fatalf("expected Status to remain Potential (was explicitly deactivated), got %v", updatedLink.Status)
	}
	if updatedLink.IsUp {
		t.Fatalf("expected IsUp=false for Potential wired link (was explicitly deactivated), got IsUp=true")
	}
}
