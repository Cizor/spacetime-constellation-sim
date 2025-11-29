package core

import "testing"

func TestNetworkInterfaceAttributes(t *testing.T) {
	intf := &NetworkInterface{
		ID:            "intf-1",
		Name:          "SAT-Ku",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku",
		ParentNodeID:  "sat1",
		IsOperational: true,
	}

	// Assert all the fields we set so the writes are meaningful and
	// we verify the struct wiring.
	if intf.ID != "intf-1" {
		t.Errorf("expected ID=intf-1, got %s", intf.ID)
	}
	if intf.Name != "SAT-Ku" {
		t.Errorf("expected Name=SAT-Ku, got %s", intf.Name)
	}
	if intf.TransceiverID != "trx-ku" {
		t.Errorf("expected TransceiverID=trx-ku, got %s", intf.TransceiverID)
	}
	if intf.ParentNodeID != "sat1" {
		t.Errorf("expected ParentNodeID=sat1, got %s", intf.ParentNodeID)
	}
	if intf.Medium != MediumWireless {
		t.Errorf("expected MediumWireless, got %v", intf.Medium)
	}
	if !intf.IsOperational {
		t.Errorf("expected interface to be operational")
	}
	// With a freshly created interface, we expect no links yet.
	if len(intf.LinkIDs) != 0 {
		t.Errorf("expected no LinkIDs initially, got %d", len(intf.LinkIDs))
	}
}
