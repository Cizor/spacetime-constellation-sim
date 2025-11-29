package core

import "testing"

// Min-elevation: configure a very high MinElevationDeg so a sat-ground
// link that is geometrically visible is still rejected by the elevation
// constraint.
func TestMinElevationConstraintBlocksLink(t *testing.T) {
	kb := NewKnowledgeBase()

	trx := &TransceiverModel{
		ID: "trx-ku",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
		MaxRangeKm: 50000.0, // far larger than our test geometry
	}
	if err := kb.AddTransceiverModel(trx); err != nil {
		t.Fatalf("AddTransceiverModel failed: %v", err)
	}

	// Ground interface.
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "if-ground",
		Name:          "GS-Ku",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku",
		ParentNodeID:  "node-ground",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(if-ground) failed: %v", err)
	}

	// Satellite interface.
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "if-sat",
		Name:          "Sat-Ku",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku",
		ParentNodeID:  "node-sat",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(if-sat) failed: %v", err)
	}

	// Ground on Earth's surface, satellite directly "above" it. Geometrically
	// this is clear LoS with elevation ~90 degrees.
	kb.SetNodeECEFPosition("node-ground", Vec3{
		X: EarthRadiusKm,
		Y: 0,
		Z: 0,
	})
	kb.SetNodeECEFPosition("node-sat", Vec3{
		X: EarthRadiusKm + 500,
		Y: 0,
		Z: 0,
	})

	cs := NewConnectivityService(kb)
	// Unrealistically strict, but guarantees the elevation check kicks in.
	cs.MinElevationDeg = 91.0

	cs.UpdateConnectivity()

	var found bool
	for _, l := range kb.GetAllNetworkLinks() {
		if (l.InterfaceA == "if-ground" && l.InterfaceB == "if-sat") ||
			(l.InterfaceA == "if-sat" && l.InterfaceB == "if-ground") {
			found = true
			if l.IsUp {
				t.Fatalf("expected link to be down due to min elevation constraint, got up (quality=%v, SNR=%v)", l.Quality, l.SNRdB)
			}
			if l.Quality != LinkQualityDown {
				t.Fatalf("expected link quality down, got %v", l.Quality)
			}
		}
	}
	if !found {
		t.Fatalf("expected a dynamic wireless link between if-ground and if-sat to be created")
	}
}

// Range cut-off: positions are in clear LoS, but beyond MaxRangeKm
// so the link is forced down.
func TestMaxRangeConstraintBlocksTooDistantLink(t *testing.T) {
	kb := NewKnowledgeBase()

	trx := &TransceiverModel{
		ID: "trx-short",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
		MaxRangeKm: 500.0, // 500 km max
	}
	if err := kb.AddTransceiverModel(trx); err != nil {
		t.Fatalf("AddTransceiverModel failed: %v", err)
	}

	// Two satellite-like nodes at the same altitude but separated in Y.
	// This gives clear LoS, but distance ~1000 km > 500 km range.
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifA",
		Name:          "If-A",
		Medium:        MediumWireless,
		TransceiverID: "trx-short",
		ParentNodeID:  "nodeA",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifA) failed: %v", err)
	}
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifB",
		Name:          "If-B",
		Medium:        MediumWireless,
		TransceiverID: "trx-short",
		ParentNodeID:  "nodeB",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifB) failed: %v", err)
	}

	kb.SetNodeECEFPosition("nodeA", Vec3{
		X: EarthRadiusKm + 700,
		Y: 0,
		Z: 0,
	})
	kb.SetNodeECEFPosition("nodeB", Vec3{
		X: EarthRadiusKm + 700,
		Y: 1000,
		Z: 0,
	})

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	var found bool
	for _, l := range kb.GetAllNetworkLinks() {
		if (l.InterfaceA == "ifA" && l.InterfaceB == "ifB") ||
			(l.InterfaceA == "ifB" && l.InterfaceB == "ifA") {
			found = true
			if l.IsUp {
				t.Fatalf("expected link to be down due to range constraint, got up (quality=%v, SNR=%v)", l.Quality, l.SNRdB)
			}
			if l.Quality != LinkQualityDown {
				t.Fatalf("expected link quality down, got %v", l.Quality)
			}
			if l.MaxDataRateMbps != 0 {
				t.Fatalf("expected zero capacity for out-of-range link, got %v Mbps", l.MaxDataRateMbps)
			}
		}
	}
	if !found {
		t.Fatalf("expected a dynamic wireless link between ifA and ifB to be created")
	}
}
