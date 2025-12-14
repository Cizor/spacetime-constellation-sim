package controller

import (
	"context"
	"testing"

	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestTelemetryServer_ExportMetrics(t *testing.T) {
	telemetryState := state.NewTelemetryState()
	log := logging.Noop()
	server := NewTelemetryServer(telemetryState, log)

	// Create a request with interface metrics
	now := timestamppb.Now()
	statusUp := telemetrypb.IfOperStatus_IF_OPER_STATUS_UP
	txBytes := int64(1000)
	rxBytes := int64(500)

	req := &telemetrypb.ExportMetricsRequest{
		InterfaceMetrics: []*telemetrypb.InterfaceMetrics{
			{
				InterfaceId: stringPtr("if1"),
				OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{
					{
						Time:  now,
						Value: &statusUp,
					},
				},
				StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{
					{
						Time:    now,
						TxBytes: &txBytes,
						RxBytes: &rxBytes,
					},
				},
			},
		},
	}

	// Create context with node_id metadata
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-node-id", "node1"))

	// Call ExportMetrics
	resp, err := server.ExportMetrics(ctx, req)
	if err != nil {
		t.Fatalf("ExportMetrics failed: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected non-nil response")
	}

	// Verify metrics were stored
	metrics := telemetryState.GetMetrics("node1", "if1")
	if metrics == nil {
		t.Fatalf("expected metrics to be stored")
	}
	if metrics.NodeID != "node1" {
		t.Fatalf("NodeID mismatch: got %q, want %q", metrics.NodeID, "node1")
	}
	if metrics.InterfaceID != "if1" {
		t.Fatalf("InterfaceID mismatch: got %q, want %q", metrics.InterfaceID, "if1")
	}
	if !metrics.Up {
		t.Fatalf("expected Up=true, got Up=false")
	}
	if metrics.BytesTx != 1000 {
		t.Fatalf("BytesTx mismatch: got %d, want 1000", metrics.BytesTx)
	}
	if metrics.BytesRx != 500 {
		t.Fatalf("BytesRx mismatch: got %d, want 500", metrics.BytesRx)
	}
}

func TestTelemetryServer_ExportMetrics_MultipleInterfaces(t *testing.T) {
	telemetryState := state.NewTelemetryState()
	log := logging.Noop()
	server := NewTelemetryServer(telemetryState, log)

	now := timestamppb.Now()
	statusUp := telemetrypb.IfOperStatus_IF_OPER_STATUS_UP
	txBytes1 := int64(1000)
	txBytes2 := int64(2000)

	req := &telemetrypb.ExportMetricsRequest{
		InterfaceMetrics: []*telemetrypb.InterfaceMetrics{
			{
				InterfaceId: stringPtr("if1"),
				OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{
					{Time: now, Value: &statusUp},
				},
				StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{
					{Time: now, TxBytes: &txBytes1},
				},
			},
			{
				InterfaceId: stringPtr("if2"),
				OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{
					{Time: now, Value: &statusUp},
				},
				StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{
					{Time: now, TxBytes: &txBytes2},
				},
			},
		},
	}

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-node-id", "node1"))

	_, err := server.ExportMetrics(ctx, req)
	if err != nil {
		t.Fatalf("ExportMetrics failed: %v", err)
	}

	// Verify both interfaces
	if1 := telemetryState.GetMetrics("node1", "if1")
	if if1 == nil || if1.BytesTx != 1000 {
		t.Fatalf("if1 metrics incorrect")
	}

	if2 := telemetryState.GetMetrics("node1", "if2")
	if if2 == nil || if2.BytesTx != 2000 {
		t.Fatalf("if2 metrics incorrect")
	}
}

func TestTelemetryServer_ExportMetrics_NoNodeID(t *testing.T) {
	telemetryState := state.NewTelemetryState()
	log := logging.Noop()
	server := NewTelemetryServer(telemetryState, log)

	now := timestamppb.Now()
	statusUp := telemetrypb.IfOperStatus_IF_OPER_STATUS_UP
	txBytes := int64(1000)

	req := &telemetrypb.ExportMetricsRequest{
		InterfaceMetrics: []*telemetrypb.InterfaceMetrics{
			{
				InterfaceId: stringPtr("if1"),
				OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{
					{Time: now, Value: &statusUp},
				},
				StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{
					{Time: now, TxBytes: &txBytes},
				},
			},
		},
	}

	// Context without node_id metadata
	ctx := context.Background()

	_, err := server.ExportMetrics(ctx, req)
	if err != nil {
		t.Fatalf("ExportMetrics should succeed even without node_id: %v", err)
	}

	// Metrics should be stored with empty node_id
	metrics := telemetryState.GetMetrics("", "if1")
	if metrics == nil {
		t.Fatalf("expected metrics to be stored even with empty node_id")
	}
}

func TestTelemetryServer_ExportMetrics_NilTelemetryState(t *testing.T) {
	log := logging.Noop()
	server := NewTelemetryServer(nil, log)

	req := &telemetrypb.ExportMetricsRequest{}
	ctx := context.Background()

	_, err := server.ExportMetrics(ctx, req)
	if err == nil {
		t.Fatalf("expected error when TelemetryState is nil")
	}
}

func TestTelemetryServer_ExportMetrics_ModemMetrics(t *testing.T) {
	telemetryState := state.NewTelemetryState()
	log := logging.Noop()
	server := NewTelemetryServer(telemetryState, log)

	now := timestamppb.Now()
	sinrVal := 15.5
	req := &telemetrypb.ExportMetricsRequest{
		ModemMetrics: []*telemetrypb.ModemMetrics{
			{
				DemodulatorId: stringPtr("if1"),
				SinrDataPoints: []*telemetrypb.SinrDataPoint{
					{
						Time:        now,
						ModulatorId: stringPtr("QPSK"),
						SinrDb:      &sinrVal,
					},
				},
			},
		},
	}

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-node-id", "node1"))

	if _, err := server.ExportMetrics(ctx, req); err != nil {
		t.Fatalf("ExportMetrics failed: %v", err)
	}

	modemMetrics, err := telemetryState.GetModemMetrics("node1", "if1")
	if err != nil {
		t.Fatalf("GetModemMetrics failed: %v", err)
	}
	if modemMetrics == nil {
		t.Fatalf("expected modem metrics to be stored")
	}
	if modemMetrics.SNRdB != sinrVal {
		t.Fatalf("expected SNR %f got %f", sinrVal, modemMetrics.SNRdB)
	}
	if modemMetrics.Modulation != "QPSK" {
		t.Fatalf("expected modulation QPSK, got %s", modemMetrics.Modulation)
	}
}

func stringPtr(s string) *string {
	return &s
}
