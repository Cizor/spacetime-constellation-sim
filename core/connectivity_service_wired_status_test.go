package core

import (
	"testing"
)

// TestWiredLinkRespectsPotentialStatus verifies that wired links respect
// explicitly-set Potential status and do not auto-activate, allowing
// administrative disabling of wired links.
func TestWiredLinkRespectsPotentialStatus(t *testing.T) {
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

	// Create a wired link with explicitly-set Potential status (explicitly deactivated)
	link := &NetworkLink{
		ID:                       "linkAB-wired",
		InterfaceA:               "ifA",
		InterfaceB:               "ifB",
		Medium:                   MediumWired,
		Status:                   LinkStatusPotential, // Explicitly set to Potential
		WasExplicitlyDeactivated: true,                // Explicitly deactivated
	}
	if err := kb.AddNetworkLink(link); err != nil {
		t.Fatalf("AddNetworkLink(linkAB-wired): %v", err)
	}

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	// Verify that Potential status is respected (not auto-activated)
	updatedLink := kb.GetNetworkLink("linkAB-wired")
	if updatedLink == nil {
		t.Fatalf("expected link to exist")
	}
	if updatedLink.Status != LinkStatusPotential {
		t.Fatalf("expected Status to remain Potential (not auto-activated), got %v", updatedLink.Status)
	}
	if updatedLink.IsUp {
		t.Fatalf("expected IsUp=false for Potential wired link, got IsUp=true")
	}
	if updatedLink.Quality != LinkQualityExcellent {
		t.Fatalf("expected Quality=Excellent for wired link, got %v", updatedLink.Quality)
	}
}

// TestWiredLinkAutoActivatesUnknownStatus verifies that wired links with
// Unknown status are auto-activated for backward compatibility.
func TestWiredLinkAutoActivatesUnknownStatus(t *testing.T) {
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

	// Create a wired link with Unknown status (should auto-activate)
	link := &NetworkLink{
		ID:         "linkAB-wired-unknown",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     MediumWired,
		Status:     LinkStatusUnknown, // Unknown - should auto-activate
	}
	if err := kb.AddNetworkLink(link); err != nil {
		t.Fatalf("AddNetworkLink(linkAB-wired-unknown): %v", err)
	}

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	// Verify that Unknown status is auto-activated
	updatedLink := kb.GetNetworkLink("linkAB-wired-unknown")
	if updatedLink == nil {
		t.Fatalf("expected link to exist")
	}
	if updatedLink.Status != LinkStatusActive {
		t.Fatalf("expected Status to be auto-activated to Active, got %v", updatedLink.Status)
	}
	if !updatedLink.IsUp {
		t.Fatalf("expected IsUp=true for Active wired link, got IsUp=false")
	}
	if updatedLink.Quality != LinkQualityExcellent {
		t.Fatalf("expected Quality=Excellent for wired link, got %v", updatedLink.Quality)
	}
}
