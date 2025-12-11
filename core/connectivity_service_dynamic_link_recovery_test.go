package core

import (
	"testing"
)

// TestDynamicLinkRecoveryAfterGeometryFailure verifies that dynamic wireless links
// that were auto-downgraded to Potential due to geometry failure will auto-activate
// when geometry improves, ensuring symmetric behavior with static links.
func TestDynamicLinkRecoveryAfterGeometryFailure(t *testing.T) {
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

	cs := NewConnectivityService(kb)

	// Step 1: Set positions for clear LoS - dynamic link should be created and auto-activated
	kb.SetNodeECEFPosition("nodeA", Vec3{X: EarthRadiusKm + 500, Y: 0, Z: 0})
	kb.SetNodeECEFPosition("nodeB", Vec3{X: EarthRadiusKm + 500, Y: 100, Z: 0})

	cs.UpdateConnectivity()

	// Find the dynamic link
	var dynamicLink *NetworkLink
	for _, link := range kb.GetAllNetworkLinks() {
		if link.InterfaceA == "ifA" && link.InterfaceB == "ifB" ||
			link.InterfaceA == "ifB" && link.InterfaceB == "ifA" {
			if !link.IsStatic {
				dynamicLink = link
				break
			}
		}
	}

	if dynamicLink == nil {
		t.Fatalf("expected dynamic link to be created")
	}
	if dynamicLink.IsStatic {
		t.Fatalf("expected dynamic link to have IsStatic=false")
	}
	if dynamicLink.Status != LinkStatusActive {
		t.Fatalf("expected dynamic link to be Active with clear LoS, got Status=%v", dynamicLink.Status)
	}
	if !dynamicLink.IsUp {
		t.Fatalf("expected dynamic link to be up with clear LoS, got IsUp=false")
	}

	// Step 2: Move nodes to block LoS - link should be downgraded to Potential
	kb.SetNodeECEFPosition("nodeA", Vec3{X: EarthRadiusKm + 100, Y: 0, Z: 0})
	kb.SetNodeECEFPosition("nodeB", Vec3{X: -EarthRadiusKm - 100, Y: 0, Z: 0}) // Behind Earth

	cs.UpdateConnectivity()

	// Re-fetch the dynamic link after UpdateConnectivity
	dynamicLink = nil
	for _, link := range kb.GetAllNetworkLinks() {
		if link.InterfaceA == "ifA" && link.InterfaceB == "ifB" ||
			link.InterfaceA == "ifB" && link.InterfaceB == "ifA" {
			if !link.IsStatic {
				dynamicLink = link
				break
			}
		}
	}
	if dynamicLink == nil {
		t.Fatalf("expected dynamic link to still exist after LoS blocked")
	}

	// Verify link was downgraded to Potential
	if dynamicLink.Status != LinkStatusPotential {
		t.Fatalf("expected dynamic link to be downgraded to Potential when LoS is blocked, got Status=%v", dynamicLink.Status)
	}
	if dynamicLink.IsUp {
		t.Fatalf("expected dynamic link to be down when LoS is blocked, got IsUp=true")
	}

	// Step 3: Restore clear LoS - link should auto-activate (this is the bug fix)
	kb.SetNodeECEFPosition("nodeA", Vec3{X: EarthRadiusKm + 500, Y: 0, Z: 0})
	kb.SetNodeECEFPosition("nodeB", Vec3{X: EarthRadiusKm + 500, Y: 100, Z: 0})

	cs.UpdateConnectivity()

	// Re-fetch the dynamic link after UpdateConnectivity
	dynamicLink = nil
	for _, link := range kb.GetAllNetworkLinks() {
		if link.InterfaceA == "ifA" && link.InterfaceB == "ifB" ||
			link.InterfaceA == "ifB" && link.InterfaceB == "ifA" {
			if !link.IsStatic {
				dynamicLink = link
				break
			}
		}
	}
	if dynamicLink == nil {
		t.Fatalf("expected dynamic link to still exist after LoS restored")
	}

	// Verify link auto-activated
	if dynamicLink.Status != LinkStatusActive {
		t.Fatalf("expected dynamic link to auto-activate when geometry improves, got Status=%v", dynamicLink.Status)
	}
	if !dynamicLink.IsUp {
		t.Fatalf("expected dynamic link to be up when geometry improves, got IsUp=false")
	}
}

