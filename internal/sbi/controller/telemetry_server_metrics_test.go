package controller

import (
	"context"
	"testing"

	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestTelemetryServer_Metrics_TelemetryReports(t *testing.T) {
	telemetryState := state.NewTelemetryState()
	log := logging.Noop()

	metrics := sbi.NewSBIMetrics()
	server := NewTelemetryServer(telemetryState, log)
	server.Metrics = metrics

	// Create ExportMetricsRequest with one interface metric
	req := &telemetrypb.ExportMetricsRequest{
		InterfaceMetrics: []*telemetrypb.InterfaceMetrics{
			{
				InterfaceId: func(s string) *string { return &s }("if-1"),
				OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{
					{
						Time:  timestamppb.Now(),
						Value: statusPtr(telemetrypb.IfOperStatus_IF_OPER_STATUS_UP),
					},
				},
			},
		},
	}

	ctx := context.Background()
	_, err := server.ExportMetrics(ctx, req)
	if err != nil {
		t.Fatalf("ExportMetrics failed: %v", err)
	}

	// Verify metrics
	snap := metrics.Snapshot()
	if snap.NumTelemetryReports != 1 {
		t.Errorf("expected NumTelemetryReports=1, got %d", snap.NumTelemetryReports)
	}
}

func TestTelemetryServer_Metrics_MultipleReports(t *testing.T) {
	telemetryState := state.NewTelemetryState()
	log := logging.Noop()

	metrics := sbi.NewSBIMetrics()
	server := NewTelemetryServer(telemetryState, log)
	server.Metrics = metrics

	ctx := context.Background()

	// Call ExportMetrics multiple times
	for i := 0; i < 3; i++ {
		req := &telemetrypb.ExportMetricsRequest{
			InterfaceMetrics: []*telemetrypb.InterfaceMetrics{
				{
					InterfaceId: func(s string) *string { return &s }("if-1"),
					OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{
						{
							Time:  timestamppb.Now(),
							Value: statusPtr(telemetrypb.IfOperStatus_IF_OPER_STATUS_UP),
						},
					},
				},
			},
		}

		_, err := server.ExportMetrics(ctx, req)
		if err != nil {
			t.Fatalf("ExportMetrics failed: %v", err)
		}
	}

	// Verify metrics
	snap := metrics.Snapshot()
	if snap.NumTelemetryReports != 3 {
		t.Errorf("expected NumTelemetryReports=3, got %d", snap.NumTelemetryReports)
	}
}

func TestTelemetryServer_Metrics_NilMetrics(t *testing.T) {
	telemetryState := state.NewTelemetryState()
	log := logging.Noop()

	server := NewTelemetryServer(telemetryState, log)
	// Metrics is nil by default

	req := &telemetrypb.ExportMetricsRequest{
		InterfaceMetrics: []*telemetrypb.InterfaceMetrics{
			{
				InterfaceId: func(s string) *string { return &s }("if-1"),
				OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{
					{
						Time:  timestamppb.Now(),
						Value: statusPtr(telemetrypb.IfOperStatus_IF_OPER_STATUS_UP),
					},
				},
			},
		},
	}

	ctx := context.Background()
	_, err := server.ExportMetrics(ctx, req)
	if err != nil {
		t.Fatalf("ExportMetrics failed: %v", err)
	}

	// Should not panic even with nil metrics
}

// Helper function
func statusPtr(s telemetrypb.IfOperStatus) *telemetrypb.IfOperStatus {
	return &s
}

