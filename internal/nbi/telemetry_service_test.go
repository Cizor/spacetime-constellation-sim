package nbi

import (
	"context"
	"testing"

	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
)

func TestTelemetryService_ListInterfaceMetrics_All(t *testing.T) {
	ts := sim.NewTelemetryState()
	ts.UpdateMetrics(&sim.InterfaceMetrics{
		NodeID:      "n1",
		InterfaceID: "if1",
		Up:          true,
		BytesTx:     100,
		BytesRx:     50,
	})
	ts.UpdateMetrics(&sim.InterfaceMetrics{
		NodeID:      "n1",
		InterfaceID: "if2",
		Up:          false,
		BytesTx:     200,
	})
	ts.UpdateMetrics(&sim.InterfaceMetrics{
		NodeID:      "n2",
		InterfaceID: "if1",
		Up:          true,
		BytesTx:     300,
	})

	service := NewTelemetryService(ts, logging.Noop())
	req := &v1alpha.ListInterfaceMetricsRequest{}

	resp, err := service.ListInterfaceMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("ListInterfaceMetrics failed: %v", err)
	}

	if len(resp.Metrics) != 3 {
		t.Fatalf("expected 3 metrics, got %d", len(resp.Metrics))
	}

	// Verify all metrics are present
	found := make(map[string]bool)
	for _, m := range resp.Metrics {
		key := m.GetNodeId() + "/" + m.GetInterfaceId()
		found[key] = true
	}
	if !found["n1/if1"] || !found["n1/if2"] || !found["n2/if1"] {
		t.Errorf("missing expected metrics, found keys: %v", found)
	}
}

func TestTelemetryService_ListInterfaceMetrics_FilterByNodeID(t *testing.T) {
	ts := sim.NewTelemetryState()
	ts.UpdateMetrics(&sim.InterfaceMetrics{
		NodeID:      "n1",
		InterfaceID: "if1",
		Up:          true,
		BytesTx:     100,
	})
	ts.UpdateMetrics(&sim.InterfaceMetrics{
		NodeID:      "n2",
		InterfaceID: "if1",
		Up:          true,
		BytesTx:     200,
	})

	service := NewTelemetryService(ts, logging.Noop())
	nodeID := "n1"
	req := &v1alpha.ListInterfaceMetricsRequest{
		NodeId: &nodeID,
	}

	resp, err := service.ListInterfaceMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("ListInterfaceMetrics failed: %v", err)
	}

	if len(resp.Metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(resp.Metrics))
	}

	if resp.Metrics[0].GetNodeId() != "n1" {
		t.Errorf("expected node_id=n1, got %q", resp.Metrics[0].GetNodeId())
	}
	if resp.Metrics[0].GetInterfaceId() != "if1" {
		t.Errorf("expected interface_id=if1, got %q", resp.Metrics[0].GetInterfaceId())
	}
}

func TestTelemetryService_ListInterfaceMetrics_FilterByInterfaceID(t *testing.T) {
	ts := sim.NewTelemetryState()
	ts.UpdateMetrics(&sim.InterfaceMetrics{
		NodeID:      "n1",
		InterfaceID: "if1",
		Up:          true,
		BytesTx:     100,
	})
	ts.UpdateMetrics(&sim.InterfaceMetrics{
		NodeID:      "n1",
		InterfaceID: "if2",
		Up:          false,
		BytesTx:     200,
	})

	service := NewTelemetryService(ts, logging.Noop())
	ifaceID := "if1"
	req := &v1alpha.ListInterfaceMetricsRequest{
		InterfaceId: &ifaceID,
	}

	resp, err := service.ListInterfaceMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("ListInterfaceMetrics failed: %v", err)
	}

	if len(resp.Metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(resp.Metrics))
	}

	if resp.Metrics[0].GetInterfaceId() != "if1" {
		t.Errorf("expected interface_id=if1, got %q", resp.Metrics[0].GetInterfaceId())
	}
}

func TestTelemetryService_ListInterfaceMetrics_Empty(t *testing.T) {
	ts := sim.NewTelemetryState()
	service := NewTelemetryService(ts, logging.Noop())
	req := &v1alpha.ListInterfaceMetricsRequest{}

	resp, err := service.ListInterfaceMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("ListInterfaceMetrics failed: %v", err)
	}

	if len(resp.Metrics) != 0 {
		t.Fatalf("expected 0 metrics, got %d", len(resp.Metrics))
	}
}

func TestTelemetryService_ListInterfaceMetrics_NilTelemetryState(t *testing.T) {
	service := NewTelemetryService(nil, logging.Noop())
	req := &v1alpha.ListInterfaceMetricsRequest{}

	_, err := service.ListInterfaceMetrics(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error when telemetry state is nil")
	}
}

