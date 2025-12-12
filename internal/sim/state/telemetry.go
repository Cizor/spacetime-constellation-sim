// Package state contains telemetry state management for SBI.
package state

import (
	"sync"
)

// InterfaceMetrics represents per-interface telemetry metrics.
type InterfaceMetrics struct {
	// NodeID is the ID of the node that owns this interface.
	NodeID string

	// InterfaceID is the ID of the interface.
	InterfaceID string

	// Up indicates whether the interface/link is considered "up".
	Up bool

	// BytesTx is the total transmitted bytes (monotonic).
	BytesTx uint64

	// BytesRx is the total received bytes (monotonic). Can be 0 if not used yet.
	BytesRx uint64

	// SNRdB is the last-known SNR in dB, if available.
	SNRdB float64

	// Modulation is the modulation scheme, e.g. "QPSK", "16QAM". Empty if unknown.
	Modulation string
}

// TelemetryState is a concurrency-safe store for interface metrics.
type TelemetryState struct {
	mu   sync.RWMutex
	byIf map[string]*InterfaceMetrics // key: "nodeID/interfaceID"
}

// NewTelemetryState creates a new TelemetryState instance.
func NewTelemetryState() *TelemetryState {
	return &TelemetryState{
		byIf: make(map[string]*InterfaceMetrics),
	}
}

// telemetryKey forms the map key from nodeID and interfaceID.
func telemetryKey(nodeID, ifaceID string) string {
	return nodeID + "/" + ifaceID
}

// UpdateMetrics stores or updates metrics for a given interface.
// It stores a copy of the provided metrics to prevent external mutation.
func (t *TelemetryState) UpdateMetrics(m *InterfaceMetrics) {
	if m == nil {
		return
	}

	key := telemetryKey(m.NodeID, m.InterfaceID)

	t.mu.Lock()
	defer t.mu.Unlock()

	// Store a copy so callers cannot mutate internal state.
	copy := *m
	t.byIf[key] = &copy
}

// GetMetrics retrieves metrics for a given node and interface.
// Returns nil if no metrics exist for that (nodeID, interfaceID) pair.
// Returns a copy of the stored metrics; modifications to the returned struct
// do not affect TelemetryState.
func (t *TelemetryState) GetMetrics(nodeID, ifaceID string) *InterfaceMetrics {
	key := telemetryKey(nodeID, ifaceID)

	t.mu.RLock()
	defer t.mu.RUnlock()

	m, ok := t.byIf[key]
	if !ok || m == nil {
		return nil
	}

	// Return a copy so callers can't mutate internal state.
	copy := *m
	return &copy
}

