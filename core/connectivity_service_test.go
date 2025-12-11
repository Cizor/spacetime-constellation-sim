package core

import "testing"

// findLinkBetween returns the first link that connects ifA and ifB (in either
// direction), or nil if no such link exists.
func findLinkBetween(kb *KnowledgeBase, ifA, ifB string) *NetworkLink {
	for _, l := range kb.GetAllNetworkLinks() {
		if (l.InterfaceA == ifA && l.InterfaceB == ifB) ||
			(l.InterfaceA == ifB && l.InterfaceB == ifA) {
			return l
		}
	}
	return nil
}

// TestWirelessLoSBlocked verifies that two wireless interfaces on opposite
// sides of the Earth never get an "up" link due to the Earth occluding LoS.
func TestWirelessLoSBlocked(t *testing.T) {
	kb := NewKnowledgeBase()

	trx := &TransceiverModel{
		ID: "trx-ku",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
		MaxRangeKm: 80000.0,
	}
	if err := kb.AddTransceiverModel(trx); err != nil {
		t.Fatalf("AddTransceiverModel: %v", err)
	}

	// Two wireless interfaces on different nodes.
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifA",
		Name:          "If-A",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku",
		ParentNodeID:  "nodeA",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifA): %v", err)
	}
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifB",
		Name:          "If-B",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku",
		ParentNodeID:  "nodeB",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifB): %v", err)
	}

	// Place them on opposite sides of the Earth so LoS is blocked.
	kb.SetNodeECEFPosition("nodeA", Vec3{X: EarthRadiusKm + 500, Y: 0, Z: 0})
	kb.SetNodeECEFPosition("nodeB", Vec3{X: -(EarthRadiusKm + 500), Y: 0, Z: 0})

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	link := findLinkBetween(kb, "ifA", "ifB")
	if link == nil {
		t.Fatalf("expected a dynamic wireless link between ifA and ifB")
	}
	if link.IsUp {
		t.Fatalf("expected link to be down due to Earth occlusion, got IsUp=true")
	}
	if link.Quality != LinkQualityDown {
		t.Fatalf("expected link quality=DOWN, got %v", link.Quality)
	}
}

// TestWirelessLoSClear verifies that when two wireless interfaces have line of
// sight and reasonable range, the link can be up.
func TestWirelessLoSClear(t *testing.T) {
	kb := NewKnowledgeBase()

	trx := &TransceiverModel{
		ID: "trx-ku",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
		MaxRangeKm: 80000.0,
	}
	if err := kb.AddTransceiverModel(trx); err != nil {
		t.Fatalf("AddTransceiverModel: %v", err)
	}

	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifA",
		Name:          "If-A",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku",
		ParentNodeID:  "nodeA",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifA): %v", err)
	}
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifB",
		Name:          "If-B",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku",
		ParentNodeID:  "nodeB",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifB): %v", err)
	}

	// Put both nodes in "co-orbit" above the Earth with small separation so
	// LoS is clear and distance is well within MaxRangeKm.
	kb.SetNodeECEFPosition("nodeA", Vec3{X: EarthRadiusKm + 700, Y: 0, Z: 0})
	kb.SetNodeECEFPosition("nodeB", Vec3{X: EarthRadiusKm + 700, Y: 1000, Z: 0})

	// Create a static link (not dynamic) so we can test Status persistence
	// Start with Unknown status to test auto-activation (backward compatibility)
	if err := kb.AddNetworkLink(&NetworkLink{
		ID:         "linkAB",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     MediumWireless,
		Status:     LinkStatusUnknown, // Unknown links auto-activate when geometry allows
	}); err != nil {
		t.Fatalf("AddNetworkLink failed: %v", err)
	}

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	link := kb.GetNetworkLink("linkAB")
	if link == nil {
		t.Fatalf("expected static link linkAB to exist")
	}
	// Links with Unknown status auto-activate when geometry allows (backward compatibility)
	// So Status should be Active and link should be up
	if link.Status != LinkStatusActive {
		t.Fatalf("expected link Status = LinkStatusActive (auto-activated), got %v", link.Status)
	}
	if !link.IsUp {
		t.Fatalf("expected link to be up with clear LoS, within range, and Active status (Status=%v, Quality=%v, IsUp=%v)", link.Status, link.Quality, link.IsUp)
	}
	if link.Quality == LinkQualityDown {
		t.Fatalf("expected non-DOWN quality for a usable link, got %v", link.Quality)
	}
}

// TestFrequencyBandMismatchBlocksLink ensures that even with perfect geometry,
// a Ku–Ka band mismatch forces a wireless link down.
func TestFrequencyBandMismatchBlocksLink(t *testing.T) {
	kb := NewKnowledgeBase()

	// Non-overlapping bands: Ku vs Ka.
	trxKu := &TransceiverModel{
		ID: "trx-ku",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
		MaxRangeKm: 80000.0,
	}
	trxKa := &TransceiverModel{
		ID: "trx-ka",
		Band: FrequencyBand{
			MinGHz: 27.0,
			MaxGHz: 30.0,
		},
		MaxRangeKm: 80000.0,
	}

	if err := kb.AddTransceiverModel(trxKu); err != nil {
		t.Fatalf("AddTransceiverModel(trxKu): %v", err)
	}
	if err := kb.AddTransceiverModel(trxKa); err != nil {
		t.Fatalf("AddTransceiverModel(trxKa): %v", err)
	}

	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifKu",
		Name:          "If-Ku",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku",
		ParentNodeID:  "nodeA",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifKu): %v", err)
	}
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifKa",
		Name:          "If-Ka",
		Medium:        MediumWireless,
		TransceiverID: "trx-ka",
		ParentNodeID:  "nodeB",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifKa): %v", err)
	}

	// Geometry: co-orbiting nodes with clear LoS, within range.
	kb.SetNodeECEFPosition("nodeA", Vec3{X: EarthRadiusKm + 700, Y: 0, Z: 0})
	kb.SetNodeECEFPosition("nodeB", Vec3{X: EarthRadiusKm + 700, Y: 1000, Z: 0})

	// We use a static wireless link here so we can directly assert on its state.
	if err := kb.AddNetworkLink(&NetworkLink{
		ID:         "link-ku-ka",
		InterfaceA: "ifKu",
		InterfaceB: "ifKa",
		Medium:     MediumWireless,
		Status:     LinkStatusActive, // Set to Active to test RF compatibility check
	}); err != nil {
		t.Fatalf("AddNetworkLink(link-ku-ka): %v", err)
	}

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	link := kb.GetNetworkLink("link-ku-ka")
	if link == nil {
		t.Fatalf("expected static link link-ku-ka to exist")
	}
	if link.IsUp {
		t.Fatalf("expected Ku–Ka band mismatch to keep link down, got IsUp=true")
	}
	if link.Quality != LinkQualityDown {
		t.Fatalf("expected link quality=DOWN due to band mismatch, got %v", link.Quality)
	}
}

// TestMultiBeam_AllowsMultipleConcurrentLinks verifies the "Scope 2"
// behaviour: MaxBeams is informational only and we allow multiple concurrent
// RF links from a single interface.
func TestMultiBeam_AllowsMultipleConcurrentLinks(t *testing.T) {
	kb := NewKnowledgeBase()

	// Conceptual multi-beam Ku transceiver for the ground station.
	trxGS := &TransceiverModel{
		ID: "trx-ku-multi",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
		MaxRangeKm: 80000.0,
		// MaxBeams is deliberately *not* enforced in Scope 2.
		MaxBeams: 2,
	}

	// Simple Ku transceiver for satellites.
	trxSat := &TransceiverModel{
		ID: "trx-ku-sat",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
		MaxRangeKm: 80000.0,
	}

	if err := kb.AddTransceiverModel(trxGS); err != nil {
		t.Fatalf("AddTransceiverModel(trxGS): %v", err)
	}
	if err := kb.AddTransceiverModel(trxSat); err != nil {
		t.Fatalf("AddTransceiverModel(trxSat): %v", err)
	}

	// One ground interface, two satellite interfaces.
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "if-gs",
		Name:          "GS-Ku-Multi",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku-multi",
		ParentNodeID:  "node-gs",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(if-gs): %v", err)
	}
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "if-sat1",
		Name:          "Sat1-Ku",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku-sat",
		ParentNodeID:  "node-sat1",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(if-sat1): %v", err)
	}
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "if-sat2",
		Name:          "Sat2-Ku",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku-sat",
		ParentNodeID:  "node-sat2",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(if-sat2): %v", err)
	}

	// Place all three nodes in a co-orbiting configuration above the Earth,
	// with small angular separation so all links have LoS and are within range.
	kb.SetNodeECEFPosition("node-gs", Vec3{X: EarthRadiusKm + 700, Y: 0, Z: 0})
	kb.SetNodeECEFPosition("node-sat1", Vec3{X: EarthRadiusKm + 700, Y: 300, Z: 0})
	kb.SetNodeECEFPosition("node-sat2", Vec3{X: EarthRadiusKm + 700, Y: -300, Z: 0})

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	linkGS1 := findLinkBetween(kb, "if-gs", "if-sat1")
	linkGS2 := findLinkBetween(kb, "if-gs", "if-sat2")

	if linkGS1 == nil || linkGS2 == nil {
		t.Fatalf("expected dynamic links from GS to both satellites (linkGS1=%v, linkGS2=%v)", linkGS1, linkGS2)
	}

	if !linkGS1.IsUp || !linkGS2.IsUp {
		t.Fatalf("expected both GS–satellite links to be up (up1=%v, up2=%v)",
			linkGS1.IsUp, linkGS2.IsUp)
	}

	// Important: this test documents that in Scope 2 we do *not* enforce
	// MaxBeams; concurrency is allowed and MaxBeams is metadata only.
}

// TestManualLinkImpairmentOverridesGeometry verifies that an administratively
// impaired link is forced DOWN even when geometry and radios would otherwise
// allow it to be up.
func TestManualLinkImpairmentOverridesGeometry(t *testing.T) {
	kb := NewKnowledgeBase()

	trx := &TransceiverModel{
		ID: "trx-ku",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
		MaxRangeKm: 80000.0,
	}
	if err := kb.AddTransceiverModel(trx); err != nil {
		t.Fatalf("AddTransceiverModel: %v", err)
	}

	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifA",
		Name:          "If-A",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku",
		ParentNodeID:  "nodeA",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifA): %v", err)
	}
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifB",
		Name:          "If-B",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku",
		ParentNodeID:  "nodeB",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifB): %v", err)
	}

	// Clear LoS, within range geometry.
	kb.SetNodeECEFPosition("nodeA", Vec3{X: EarthRadiusKm + 700, Y: 0, Z: 0})
	kb.SetNodeECEFPosition("nodeB", Vec3{X: EarthRadiusKm + 700, Y: 1000, Z: 0})

	// Use a *static* wireless link so that impairment flags persist across
	// ClearDynamicWirelessLinks() calls.
	if err := kb.AddNetworkLink(&NetworkLink{
		ID:         "linkAB",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     MediumWireless,
		Status:     LinkStatusActive, // Set to Active to test impairment behavior
	}); err != nil {
		t.Fatalf("AddNetworkLink(linkAB): %v", err)
	}

	cs := NewConnectivityService(kb)

	// Baseline: link should be up with good geometry and compatible radios.
	cs.UpdateConnectivity()
	link := kb.GetNetworkLink("linkAB")
	if link == nil {
		t.Fatalf("expected static link linkAB to exist")
	}
	if !link.IsUp {
		t.Fatalf("expected baseline linkAB to be up before impairment")
	}

	// Now administratively impair the link and re-run connectivity.
	if err := kb.SetLinkImpaired("linkAB", true); err != nil {
		t.Fatalf("SetLinkImpaired(linkAB): %v", err)
	}

	cs.UpdateConnectivity()
	link = kb.GetNetworkLink("linkAB")
	if link == nil {
		t.Fatalf("expected static link linkAB to still exist after impairment")
	}
	if link.IsUp {
		t.Fatalf("expected impaired link to be down, but IsUp==true")
	}
	if link.Quality != LinkQualityDown {
		t.Fatalf("expected impaired link quality=DOWN, got %v", link.Quality)
	}
}
