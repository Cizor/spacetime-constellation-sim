package sbi

import (
	"sync"
	"testing"
)

func TestSBIMetrics_NewSBIMetrics(t *testing.T) {
	m := NewSBIMetrics()
	if m == nil {
		t.Fatalf("NewSBIMetrics returned nil")
	}

	snap := m.Snapshot()
	if snap.NumCreateEntrySent != 0 {
		t.Errorf("expected NumCreateEntrySent=0, got %d", snap.NumCreateEntrySent)
	}
}

func TestSBIMetrics_IncrementHelpers(t *testing.T) {
	m := NewSBIMetrics()

	// Test all increment helpers
	m.IncCreateEntrySent()
	m.IncDeleteEntrySent()
	m.IncFinalizeSent()
	m.IncActionsExecuted()
	m.IncResponsesOK()
	m.IncResponsesError()
	m.IncTelemetryReports()

	snap := m.Snapshot()
	if snap.NumCreateEntrySent != 1 {
		t.Errorf("expected NumCreateEntrySent=1, got %d", snap.NumCreateEntrySent)
	}
	if snap.NumDeleteEntrySent != 1 {
		t.Errorf("expected NumDeleteEntrySent=1, got %d", snap.NumDeleteEntrySent)
	}
	if snap.NumFinalizeSent != 1 {
		t.Errorf("expected NumFinalizeSent=1, got %d", snap.NumFinalizeSent)
	}
	if snap.NumActionsExecuted != 1 {
		t.Errorf("expected NumActionsExecuted=1, got %d", snap.NumActionsExecuted)
	}
	if snap.NumResponsesOK != 1 {
		t.Errorf("expected NumResponsesOK=1, got %d", snap.NumResponsesOK)
	}
	if snap.NumResponsesError != 1 {
		t.Errorf("expected NumResponsesError=1, got %d", snap.NumResponsesError)
	}
	if snap.NumTelemetryReports != 1 {
		t.Errorf("expected NumTelemetryReports=1, got %d", snap.NumTelemetryReports)
	}
}

func TestSBIMetrics_MultipleIncrements(t *testing.T) {
	m := NewSBIMetrics()

	// Increment multiple times
	for i := 0; i < 5; i++ {
		m.IncCreateEntrySent()
		m.IncActionsExecuted()
	}

	snap := m.Snapshot()
	if snap.NumCreateEntrySent != 5 {
		t.Errorf("expected NumCreateEntrySent=5, got %d", snap.NumCreateEntrySent)
	}
	if snap.NumActionsExecuted != 5 {
		t.Errorf("expected NumActionsExecuted=5, got %d", snap.NumActionsExecuted)
	}
}

func TestSBIMetrics_ConcurrentAccess(t *testing.T) {
	m := NewSBIMetrics()

	// Concurrent increments from multiple goroutines
	const numGoroutines = 10
	const incrementsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				m.IncCreateEntrySent()
				m.IncActionsExecuted()
				m.IncResponsesOK()
			}
		}()
	}

	wg.Wait()

	snap := m.Snapshot()
	expected := uint64(numGoroutines * incrementsPerGoroutine)
	if snap.NumCreateEntrySent != expected {
		t.Errorf("expected NumCreateEntrySent=%d, got %d", expected, snap.NumCreateEntrySent)
	}
	if snap.NumActionsExecuted != expected {
		t.Errorf("expected NumActionsExecuted=%d, got %d", expected, snap.NumActionsExecuted)
	}
	if snap.NumResponsesOK != expected {
		t.Errorf("expected NumResponsesOK=%d, got %d", expected, snap.NumResponsesOK)
	}
}

func TestSBIMetrics_Snapshot(t *testing.T) {
	m := NewSBIMetrics()

	m.IncCreateEntrySent()
	snap1 := m.Snapshot()

	m.IncCreateEntrySent()
	snap2 := m.Snapshot()

	if snap1.NumCreateEntrySent != 1 {
		t.Errorf("snap1: expected NumCreateEntrySent=1, got %d", snap1.NumCreateEntrySent)
	}
	if snap2.NumCreateEntrySent != 2 {
		t.Errorf("snap2: expected NumCreateEntrySent=2, got %d", snap2.NumCreateEntrySent)
	}
}

func TestSBIMetrics_String(t *testing.T) {
	m := NewSBIMetrics()

	m.IncCreateEntrySent()
	m.IncDeleteEntrySent()
	m.IncFinalizeSent()
	m.IncActionsExecuted()
	m.IncResponsesOK()
	m.IncResponsesError()
	m.IncTelemetryReports()

	str := m.String()
	if str == "" {
		t.Fatalf("String() returned empty string")
	}

	// Verify it contains expected values
	if len(str) < 50 {
		t.Errorf("String() seems too short: %q", str)
	}
}

