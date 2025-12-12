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

