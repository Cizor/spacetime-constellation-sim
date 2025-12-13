package sbi

import (
	"fmt"
	"sync"
)

// SBIMetrics tracks in-memory counters for SBI activity.
// All counters are concurrency-safe and can be incremented from multiple goroutines.
type SBIMetrics struct {
	mu sync.Mutex

	// CDPI → Agent commands
	NumCreateEntrySent uint64
	NumDeleteEntrySent uint64
	NumFinalizeSent    uint64

	// Agent-side execution
	NumActionsExecuted uint64

	// Agent → CDPI responses
	NumResponsesOK    uint64
	NumResponsesError uint64

	// Telemetry
	NumTelemetryReports uint64
}

// NewSBIMetrics creates a new SBIMetrics instance with all counters initialized to zero.
func NewSBIMetrics() *SBIMetrics {
	return &SBIMetrics{}
}

// IncCreateEntrySent increments the CreateEntrySent counter.
func (m *SBIMetrics) IncCreateEntrySent() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NumCreateEntrySent++
}

// IncDeleteEntrySent increments the DeleteEntrySent counter.
func (m *SBIMetrics) IncDeleteEntrySent() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NumDeleteEntrySent++
}

// IncFinalizeSent increments the FinalizeSent counter.
func (m *SBIMetrics) IncFinalizeSent() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NumFinalizeSent++
}

// IncActionsExecuted increments the ActionsExecuted counter.
func (m *SBIMetrics) IncActionsExecuted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NumActionsExecuted++
}

// IncResponsesOK increments the ResponsesOK counter.
func (m *SBIMetrics) IncResponsesOK() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NumResponsesOK++
}

// IncResponsesError increments the ResponsesError counter.
func (m *SBIMetrics) IncResponsesError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NumResponsesError++
}

// IncTelemetryReports increments the TelemetryReports counter.
func (m *SBIMetrics) IncTelemetryReports() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NumTelemetryReports++
}

// SBIMetricsSnapshot is a snapshot of current metrics values.
// It's safe to read without holding the mutex.
type SBIMetricsSnapshot struct {
	NumCreateEntrySent  uint64
	NumDeleteEntrySent  uint64
	NumFinalizeSent     uint64
	NumActionsExecuted  uint64
	NumResponsesOK      uint64
	NumResponsesError   uint64
	NumTelemetryReports uint64
}

// Snapshot returns a snapshot of the current metrics values.
func (m *SBIMetrics) Snapshot() SBIMetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return SBIMetricsSnapshot{
		NumCreateEntrySent:  m.NumCreateEntrySent,
		NumDeleteEntrySent:  m.NumDeleteEntrySent,
		NumFinalizeSent:     m.NumFinalizeSent,
		NumActionsExecuted:  m.NumActionsExecuted,
		NumResponsesOK:      m.NumResponsesOK,
		NumResponsesError:   m.NumResponsesError,
		NumTelemetryReports: m.NumTelemetryReports,
	}
}

// String returns a human-readable string representation of the metrics.
func (m *SBIMetrics) String() string {
	snap := m.Snapshot()
	return fmt.Sprintf("SBI metrics: create=%d delete=%d finalize=%d executed=%d resp_ok=%d resp_err=%d telemetry_reports=%d",
		snap.NumCreateEntrySent,
		snap.NumDeleteEntrySent,
		snap.NumFinalizeSent,
		snap.NumActionsExecuted,
		snap.NumResponsesOK,
		snap.NumResponsesError,
		snap.NumTelemetryReports,
	)
}

