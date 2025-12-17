// Package state contains telemetry state management for SBI.
package state

import (
	"errors"
	"math"
	"sync"
	"time"
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

// ModemMetrics represents low-level modem stats per interface.
type ModemMetrics struct {
	// NodeID is optional but recommended to correlate with interface metrics.
	NodeID string
	// InterfaceID identifies the interface this modem data belongs to.
	InterfaceID string
	// SNRdB is the estimated signal-to-noise ratio in dB.
	SNRdB float64
	// Modulation describes the modulation scheme (e.g. "QPSK").
	Modulation string
	// CodingRate holds the forward error correction rate string.
	CodingRate string
	// BER is the bit error rate.
	BER float64
	// ThroughputBps is the current throughput estimate in bits per second.
	ThroughputBps uint64
	// Timestamp indicates when these metrics were captured.
	Timestamp time.Time
}

// IntentMetrics captures per-ServiceRequest telemetry information.
type IntentMetrics struct {
	ServiceRequestID    string
	IsProvisioned       bool
	ProvisionedDuration time.Duration
	TotalDuration       time.Duration
	FulfillmentRate     float64
	AverageLatency      time.Duration
	BytesTransferred    uint64
}

// TelemetryState is a concurrency-safe store for interface metrics.
type TelemetryState struct {
	mu     sync.RWMutex
	byIf   map[string]*InterfaceMetrics // key: "nodeID/interfaceID"
	modem  map[string]*ModemMetrics     // key: "nodeID/interfaceID"
	intent map[string]*IntentMetrics
}

// NewTelemetryState creates a new TelemetryState instance.
func NewTelemetryState() *TelemetryState {
	return &TelemetryState{
		byIf:   make(map[string]*InterfaceMetrics),
		modem:  make(map[string]*ModemMetrics),
		intent: make(map[string]*IntentMetrics),
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

// ListAll returns all stored interface metrics.
// Returns a slice of copies; modifications to the returned structs
// do not affect TelemetryState.
func (t *TelemetryState) ListAll() []*InterfaceMetrics {
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := make([]*InterfaceMetrics, 0, len(t.byIf))
	for _, v := range t.byIf {
		if v == nil {
			continue
		}
		// Make a copy to avoid mutation outside.
		cp := *v
		out = append(out, &cp)
	}
	return out
}

// UpdateModemMetrics stores modem metrics for an interface.
func (t *TelemetryState) UpdateModemMetrics(m *ModemMetrics) error {
	if m == nil {
		return errors.New("modem metrics is nil")
	}
	if m.InterfaceID == "" {
		return errors.New("interface ID is required")
	}
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now()
	}
	key := telemetryKey(m.NodeID, m.InterfaceID)

	t.mu.Lock()
	defer t.mu.Unlock()

	copy := *m
	t.modem[key] = &copy
	return nil
}

// UpdateIntentMetrics stores intent metrics for a service request.
func (t *TelemetryState) UpdateIntentMetrics(metrics *IntentMetrics) error {
	if metrics == nil {
		return errors.New("intent metrics is nil")
	}
	if metrics.ServiceRequestID == "" {
		return errors.New("service request ID is required")
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	cp := *metrics
	t.intent[metrics.ServiceRequestID] = &cp
	return nil
}

// GetIntentMetrics retrieves intent metrics for a given service request.
func (t *TelemetryState) GetIntentMetrics(srID string) *IntentMetrics {
	if srID == "" {
		return nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	m, ok := t.intent[srID]
	if !ok || m == nil {
		return nil
	}
	cp := *m
	return &cp
}

// GetModemMetrics retrieves modem metrics for an interface.
func (t *TelemetryState) GetModemMetrics(nodeID, ifaceID string) (*ModemMetrics, error) {
	key := telemetryKey(nodeID, ifaceID)

	t.mu.RLock()
	defer t.mu.RUnlock()

	m, ok := t.modem[key]
	if !ok || m == nil {
		return nil, nil
	}

	copy := *m
	return &copy, nil
}

// NodeInterfaceSummary captures aggregated interface metrics for a node.
type NodeInterfaceSummary struct {
	NodeID         string  `json:"node_id"`
	InterfaceCount int     `json:"interface_count"`
	InterfacesUp   int     `json:"interfaces_up"`
	InterfacesDown int     `json:"interfaces_down"`
	TotalBytesTx   uint64  `json:"total_bytes_tx"`
	TotalBytesRx   uint64  `json:"total_bytes_rx"`
	AverageSNRdB   float64 `json:"average_snr_db"`
}

// NodeModemSummary captures aggregated modem metrics for a node.
type NodeModemSummary struct {
	NodeID             string    `json:"node_id"`
	ModemCount         int       `json:"modem_count"`
	TotalThroughputBps uint64    `json:"total_throughput_bps"`
	AverageSNRdB       float64   `json:"average_snr_db"`
	LatestTimestamp    time.Time `json:"latest_timestamp"`
}

// AggregateInterfaceMetricsByNode groups interface metrics per node.
func (t *TelemetryState) AggregateInterfaceMetricsByNode() map[string]NodeInterfaceSummary {
	if t == nil {
		return nil
	}

	type acc struct {
		summary NodeInterfaceSummary
		snrSum  float64
		snrCnt  int
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]NodeInterfaceSummary)
	temp := make(map[string]*acc)
	for _, m := range t.byIf {
		if m == nil {
			continue
		}
		nodeID := m.NodeID
		if nodeID == "" {
			nodeID = "unknown"
		}
		entry, ok := temp[nodeID]
		if !ok {
			entry = &acc{}
			entry.summary.NodeID = nodeID
			temp[nodeID] = entry
		}
		entry.summary.InterfaceCount++
		if m.Up {
			entry.summary.InterfacesUp++
		} else {
			entry.summary.InterfacesDown++
		}
		entry.summary.TotalBytesTx += m.BytesTx
		entry.summary.TotalBytesRx += m.BytesRx
		if !math.IsNaN(m.SNRdB) {
			entry.snrSum += m.SNRdB
			entry.snrCnt++
		}
	}

	for nodeID, aggregate := range temp {
		if aggregate.snrCnt > 0 {
			aggregate.summary.AverageSNRdB = aggregate.snrSum / float64(aggregate.snrCnt)
		}
		result[nodeID] = aggregate.summary
	}
	return result
}

// AggregateModemMetricsByNode groups modem metrics per node.
func (t *TelemetryState) AggregateModemMetricsByNode() map[string]NodeModemSummary {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()

	type acc struct {
		summary NodeModemSummary
		snrSum  float64
		snrCnt  int
	}

	temp := make(map[string]*acc)
	for _, m := range t.modem {
		if m == nil {
			continue
		}
		nodeID := m.NodeID
		if nodeID == "" {
			nodeID = "unknown"
		}
		entry, ok := temp[nodeID]
		if !ok {
			entry = &acc{}
			entry.summary.NodeID = nodeID
			temp[nodeID] = entry
		}
		entry.summary.ModemCount++
		entry.summary.TotalThroughputBps += m.ThroughputBps
		if m.Timestamp.After(entry.summary.LatestTimestamp) {
			entry.summary.LatestTimestamp = m.Timestamp
		}
		if !math.IsNaN(m.SNRdB) {
			entry.snrSum += m.SNRdB
			entry.snrCnt++
		}
	}

	result := make(map[string]NodeModemSummary)
	for nodeID, aggregate := range temp {
		if aggregate.snrCnt > 0 {
			aggregate.summary.AverageSNRdB = aggregate.snrSum / float64(aggregate.snrCnt)
		}
		result[nodeID] = aggregate.summary
	}
	return result
}
