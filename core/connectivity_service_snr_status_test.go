package core

import (
	"testing"
)

// TestSNRFailureDowngradesActiveStatus verifies that when SNR evaluation fails
// (LinkQualityDown), a link with Active status is downgraded to Potential to
// maintain consistency (Status should not remain Active when IsUp is false).
func TestSNRFailureDowngradesActiveStatus(t *testing.T) {
	kb := NewKnowledgeBase()

	// Create transceiver model with very low power to force negative SNR
	// but with long range so range check passes
	trx := &TransceiverModel{
		ID: "trx-low-power",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
		MaxRangeKm:  100000.0, // Long range - will pass range check
		TxPowerDBw:  -20,      // Very low power (will cause negative SNR at distance)
		GainTxDBi:   0,        // No gain
		GainRxDBi:   0,        // No gain
	}
	if err := kb.AddTransceiverModel(trx); err != nil {
		t.Fatalf("AddTransceiverModel: %v", err)
	}

	// Create interfaces
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifA",
		Name:          "If-A",
		Medium:        MediumWireless,
		TransceiverID: "trx-low-power",
		ParentNodeID:  "nodeA",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifA): %v", err)
	}
	if err := kb.AddInterface(&NetworkInterface{
		ID:            "ifB",
		Name:          "If-B",
		Medium:        MediumWireless,
		TransceiverID: "trx-low-power",
		ParentNodeID:  "nodeB",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifB): %v", err)
	}

	// Set node positions far apart (within range but will cause negative SNR)
	// Distance ~1000km: FSPL ~92.45 + 20*log10(1000) + 20*log10(11) ≈ 92.45 + 60 + 20.8 ≈ 173 dB
	// With -20 dBW TX power and 0 dBi gains: PR = -20 + 0 + 0 - 173 = -193 dBW
	// Noise floor ~-120 dBW: SNR = -193 - (-120) = -73 dB (negative, so LinkQualityDown)
	kb.SetNodeECEFPosition("nodeA", Vec3{X: EarthRadiusKm + 500, Y: 0, Z: 0})
	kb.SetNodeECEFPosition("nodeB", Vec3{X: EarthRadiusKm + 500, Y: 1000, Z: 0}) // ~1000km apart

	// Create a link with Active status (simulating a link that was previously activated)
	link := &NetworkLink{
		ID:         "linkAB-active",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     MediumWireless,
		Status:     LinkStatusActive, // Start as Active
	}
	if err := kb.AddNetworkLink(link); err != nil {
		t.Fatalf("AddNetworkLink(linkAB-active): %v", err)
	}

	cs := NewConnectivityService(kb)
	cs.UpdateConnectivity()

	// Verify that SNR/range failure downgrades Active to Potential
	updatedLink := kb.GetNetworkLink("linkAB-active")
	if updatedLink == nil {
		t.Fatalf("expected link to exist")
	}
	if updatedLink.Status != LinkStatusPotential {
		t.Fatalf("expected Status to be downgraded from Active to Potential due to SNR/range failure, got %v", updatedLink.Status)
	}
	if updatedLink.IsUp {
		t.Fatalf("expected IsUp=false due to SNR/range failure, got IsUp=true")
	}
	if updatedLink.Quality != LinkQualityDown {
		t.Fatalf("expected Quality=Down due to SNR/range failure, got %v", updatedLink.Quality)
	}
}

