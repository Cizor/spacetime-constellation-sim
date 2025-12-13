package state

import (
	"testing"
)

func TestTelemetryState_UpdateMetrics_NewEntry(t *testing.T) {
	ts := NewTelemetryState()

	metrics := &InterfaceMetrics{
		NodeID:      "n1",
		InterfaceID: "if1",
		Up:          true,
		BytesTx:     100,
		BytesRx:     50,
		SNRdB:       25.5,
		Modulation:   "QPSK",
	}

	ts.UpdateMetrics(metrics)

	out := ts.GetMetrics("n1", "if1")
	if out == nil {
		t.Fatalf("expected metrics to exist")
	}
	if out.NodeID != "n1" {
		t.Fatalf("NodeID mismatch: got %q, want %q", out.NodeID, "n1")
	}
	if out.InterfaceID != "if1" {
		t.Fatalf("InterfaceID mismatch: got %q, want %q", out.InterfaceID, "if1")
	}
	if !out.Up {
		t.Fatalf("expected Up=true, got Up=false")
	}
	if out.BytesTx != 100 {
		t.Fatalf("BytesTx mismatch: got %d, want 100", out.BytesTx)
	}
	if out.BytesRx != 50 {
		t.Fatalf("BytesRx mismatch: got %d, want 50", out.BytesRx)
	}
	if out.SNRdB != 25.5 {
		t.Fatalf("SNRdB mismatch: got %f, want 25.5", out.SNRdB)
	}
	if out.Modulation != "QPSK" {
		t.Fatalf("Modulation mismatch: got %q, want %q", out.Modulation, "QPSK")
	}
}

func TestTelemetryState_UpdateMetrics_Overwrite(t *testing.T) {
	ts := NewTelemetryState()

	// Insert initial metrics
	ts.UpdateMetrics(&InterfaceMetrics{
		NodeID:      "n1",
		InterfaceID: "if1",
		Up:          true,
		BytesTx:     100,
	})

	// Overwrite with new values
	ts.UpdateMetrics(&InterfaceMetrics{
		NodeID:      "n1",
		InterfaceID: "if1",
		Up:          false,
		BytesTx:     500,
	})

	out := ts.GetMetrics("n1", "if1")
	if out == nil {
		t.Fatalf("expected metrics to exist")
	}
	if out.BytesTx != 500 {
		t.Fatalf("BytesTx mismatch after overwrite: got %d, want 500", out.BytesTx)
	}
	if out.Up {
		t.Fatalf("expected Up=false after overwrite, got Up=true")
	}
}

func TestTelemetryState_GetMetrics_UnknownInterface(t *testing.T) {
	ts := NewTelemetryState()

	out := ts.GetMetrics("n-missing", "if-missing")
	if out != nil {
		t.Fatalf("expected nil for unknown interface, got %+v", out)
	}
}

func TestTelemetryState_GetMetrics_ReturnsCopy(t *testing.T) {
	ts := NewTelemetryState()

	// Insert metrics
	ts.UpdateMetrics(&InterfaceMetrics{
		NodeID:      "n1",
		InterfaceID: "if1",
		Up:          true,
		BytesTx:     100,
	})

	// Get metrics and mutate the returned value
	out := ts.GetMetrics("n1", "if1")
	if out == nil {
		t.Fatalf("expected metrics to exist")
	}

	originalBytesTx := out.BytesTx
	out.BytesTx = 999
	out.Up = false

	// Get again and verify the stored value hasn't changed
	out2 := ts.GetMetrics("n1", "if1")
	if out2 == nil {
		t.Fatalf("expected metrics to still exist")
	}
	if out2.BytesTx != originalBytesTx {
		t.Fatalf("stored BytesTx was mutated: got %d, want %d", out2.BytesTx, originalBytesTx)
	}
	if !out2.Up {
		t.Fatalf("stored Up was mutated: got %v, want true", out2.Up)
	}
}

func TestTelemetryState_UpdateMetrics_NilInput(t *testing.T) {
	ts := NewTelemetryState()

	// Should not panic
	ts.UpdateMetrics(nil)

	out := ts.GetMetrics("n1", "if1")
	if out != nil {
		t.Fatalf("expected nil after nil UpdateMetrics, got %+v", out)
	}
}

func TestTelemetryState_ListAll(t *testing.T) {
	ts := NewTelemetryState()

	// Empty state should return empty slice
	all := ts.ListAll()
	if len(all) != 0 {
		t.Errorf("ListAll on empty state returned %d items, want 0", len(all))
	}

	// Add some metrics
	ts.UpdateMetrics(&InterfaceMetrics{
		NodeID:      "n1",
		InterfaceID: "if1",
		Up:          true,
		BytesTx:     100,
	})
	ts.UpdateMetrics(&InterfaceMetrics{
		NodeID:      "n1",
		InterfaceID: "if2",
		Up:          false,
		BytesTx:     200,
	})
	ts.UpdateMetrics(&InterfaceMetrics{
		NodeID:      "n2",
		InterfaceID: "if1",
		Up:          true,
		BytesTx:     300,
	})

	all = ts.ListAll()
	if len(all) != 3 {
		t.Fatalf("ListAll returned %d items, want 3", len(all))
	}

	// Verify all metrics are present
	found := make(map[string]bool)
	for _, m := range all {
		key := m.NodeID + "/" + m.InterfaceID
		found[key] = true
	}
	if !found["n1/if1"] || !found["n1/if2"] || !found["n2/if1"] {
		t.Errorf("ListAll missing expected metrics, found keys: %v", found)
	}
}

func TestTelemetryState_ListAll_ReturnsCopies(t *testing.T) {
	ts := NewTelemetryState()
	ts.UpdateMetrics(&InterfaceMetrics{
		NodeID:      "n1",
		InterfaceID: "if1",
		Up:          true,
		BytesTx:     100,
	})

	all := ts.ListAll()
	if len(all) != 1 {
		t.Fatalf("ListAll returned %d items, want 1", len(all))
	}

	// Mutate the returned copy
	all[0].BytesTx = 999
	all[0].Up = false

	// Fetch again and ensure original is unchanged
	original := ts.GetMetrics("n1", "if1")
	if original == nil {
		t.Fatalf("GetMetrics returned nil")
	}
	if original.BytesTx != 100 || original.Up != true {
		t.Errorf("Original metrics mutated: got BytesTx %d, Up %t; want BytesTx %d, Up %t",
			original.BytesTx, original.Up, 100, true)
	}
}

