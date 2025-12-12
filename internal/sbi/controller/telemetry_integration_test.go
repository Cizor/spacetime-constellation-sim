package controller

import (
	"context"
	"net"
	"testing"
	"time"

	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// telemetryTestHarness sets up an in-process gRPC server with TelemetryServer for testing.
type telemetryTestHarness struct {
	Server        *grpc.Server
	Listener      net.Listener
	TelemetryState *state.TelemetryState
	TelemetryServer *TelemetryServer
	Address       string
}

// newTelemetryTestHarness creates a new telemetry test harness with an in-process gRPC server.
func newTelemetryTestHarness(t *testing.T) *telemetryTestHarness {
	t.Helper()

	// Create telemetry state
	telemetryState := state.NewTelemetryState()

	// Create telemetry server
	telemetryServer := NewTelemetryServer(telemetryState, logging.Noop())

	// Create gRPC server
	grpcServer := grpc.NewServer()
	telemetrypb.RegisterTelemetryServer(grpcServer, telemetryServer)

	// Listen on random port
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}

	// Start server in goroutine
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			t.Logf("gRPC server error: %v", err)
		}
	}()

	// Give server a moment to start
	time.Sleep(50 * time.Millisecond)

	return &telemetryTestHarness{
		Server:         grpcServer,
		Listener:      lis,
		TelemetryState: telemetryState,
		TelemetryServer: telemetryServer,
		Address:       lis.Addr().String(),
	}
}

// Close shuts down the test harness.
func (h *telemetryTestHarness) Close() {
	if h.Server != nil {
		h.Server.GracefulStop()
	}
	if h.Listener != nil {
		h.Listener.Close()
	}
}

// TestTelemetryExportMetrics_Integration tests the full TelemetryService.ExportMetrics flow:
// 1. Create in-process gRPC server with TelemetryServer
// 2. Create TelemetryServiceClient and connect
// 3. Send ExportMetricsRequest with interface metrics
// 4. Verify TelemetryState is updated correctly
func TestTelemetryExportMetrics_Integration(t *testing.T) {
	harness := newTelemetryTestHarness(t)
	defer harness.Close()

	// Create gRPC client connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		harness.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("grpc.DialContext failed: %v", err)
	}
	defer conn.Close()

	// Create telemetry client
	telemetryClient := telemetrypb.NewTelemetryClient(conn)

	// Create context with node_id metadata
	ctxWithMetadata := metadata.NewOutgoingContext(ctx, metadata.Pairs("x-node-id", "node-1"))

	// Create ExportMetricsRequest with interface metrics
	now := timestamppb.Now()
	statusUp := telemetrypb.IfOperStatus_IF_OPER_STATUS_UP
	txBytes := int64(1234)
	rxBytes := int64(5678)

	req := &telemetrypb.ExportMetricsRequest{
		InterfaceMetrics: []*telemetrypb.InterfaceMetrics{
			{
				InterfaceId: stringPtr("if-1"),
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
			{
				InterfaceId: stringPtr("if-2"),
				OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{
					{
						Time:  now,
						Value: &statusUp,
					},
				},
				StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{
					{
						Time:    now,
						TxBytes: int64Ptr(0),
						RxBytes: int64Ptr(0),
					},
				},
			},
		},
	}

	// Call ExportMetrics via gRPC
	resp, err := telemetryClient.ExportMetrics(ctxWithMetadata, req)
	if err != nil {
		t.Fatalf("ExportMetrics failed: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected non-nil response")
	}

	// Verify metrics were stored in TelemetryState
	m1 := harness.TelemetryState.GetMetrics("node-1", "if-1")
	if m1 == nil {
		t.Fatalf("expected metrics for node-1/if-1")
	}
	if m1.NodeID != "node-1" {
		t.Errorf("NodeID mismatch: got %q, want %q", m1.NodeID, "node-1")
	}
	if m1.InterfaceID != "if-1" {
		t.Errorf("InterfaceID mismatch: got %q, want %q", m1.InterfaceID, "if-1")
	}
	if !m1.Up {
		t.Errorf("expected Up=true for node-1/if-1, got false")
	}
	if m1.BytesTx != 1234 {
		t.Errorf("BytesTx mismatch: got %d, want 1234", m1.BytesTx)
	}
	if m1.BytesRx != 5678 {
		t.Errorf("BytesRx mismatch: got %d, want 5678", m1.BytesRx)
	}

	m2 := harness.TelemetryState.GetMetrics("node-1", "if-2")
	if m2 == nil {
		t.Fatalf("expected metrics for node-1/if-2")
	}
	if m2.NodeID != "node-1" {
		t.Errorf("NodeID mismatch: got %q, want %q", m2.NodeID, "node-1")
	}
	if m2.InterfaceID != "if-2" {
		t.Errorf("InterfaceID mismatch: got %q, want %q", m2.InterfaceID, "if-2")
	}
	if !m2.Up {
		t.Errorf("expected Up=true for node-1/if-2, got false")
	}
	if m2.BytesTx != 0 {
		t.Errorf("BytesTx mismatch: got %d, want 0", m2.BytesTx)
	}
	if m2.BytesRx != 0 {
		t.Errorf("BytesRx mismatch: got %d, want 0", m2.BytesRx)
	}
}

// TestTelemetryExportMetrics_UpdatesExistingEntries tests that multiple
// ExportMetrics calls update existing entries correctly.
func TestTelemetryExportMetrics_UpdatesExistingEntries(t *testing.T) {
	harness := newTelemetryTestHarness(t)
	defer harness.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		harness.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("grpc.DialContext failed: %v", err)
	}
	defer conn.Close()

	telemetryClient := telemetrypb.NewTelemetryClient(conn)
	ctxWithMetadata := metadata.NewOutgoingContext(ctx, metadata.Pairs("x-node-id", "node-1"))

	now := timestamppb.Now()
	statusUp := telemetrypb.IfOperStatus_IF_OPER_STATUS_UP

	// First call
	txBytes1 := int64(1000)
	req1 := &telemetrypb.ExportMetricsRequest{
		InterfaceMetrics: []*telemetrypb.InterfaceMetrics{
			{
				InterfaceId: stringPtr("if-1"),
				OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{
					{Time: now, Value: &statusUp},
				},
				StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{
					{Time: now, TxBytes: &txBytes1},
				},
			},
		},
	}

	_, err = telemetryClient.ExportMetrics(ctxWithMetadata, req1)
	if err != nil {
		t.Fatalf("first ExportMetrics failed: %v", err)
	}

	// Second call with updated counters
	txBytes2 := int64(2500)
	req2 := &telemetrypb.ExportMetricsRequest{
		InterfaceMetrics: []*telemetrypb.InterfaceMetrics{
			{
				InterfaceId: stringPtr("if-1"),
				OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{
					{Time: now, Value: &statusUp},
				},
				StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{
					{Time: now, TxBytes: &txBytes2},
				},
			},
		},
	}

	_, err = telemetryClient.ExportMetrics(ctxWithMetadata, req2)
	if err != nil {
		t.Fatalf("second ExportMetrics failed: %v", err)
	}

	// Verify the last call's values are reflected
	m := harness.TelemetryState.GetMetrics("node-1", "if-1")
	if m == nil {
		t.Fatalf("expected metrics for node-1/if-1")
	}
	if m.BytesTx != 2500 {
		t.Errorf("BytesTx mismatch: got %d, want 2500 after update", m.BytesTx)
	}
}

// TestTelemetryExportMetrics_EmptyRequest tests that an empty request
// is handled gracefully.
func TestTelemetryExportMetrics_EmptyRequest(t *testing.T) {
	harness := newTelemetryTestHarness(t)
	defer harness.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		harness.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("grpc.DialContext failed: %v", err)
	}
	defer conn.Close()

	telemetryClient := telemetrypb.NewTelemetryClient(conn)
	ctxWithMetadata := metadata.NewOutgoingContext(ctx, metadata.Pairs("x-node-id", "node-1"))

	// Empty request
	req := &telemetrypb.ExportMetricsRequest{}

	_, err = telemetryClient.ExportMetrics(ctxWithMetadata, req)
	if err != nil {
		t.Fatalf("ExportMetrics(empty) failed: %v", err)
	}

	// No metrics should be stored
	if got := harness.TelemetryState.GetMetrics("node-1", "if-1"); got != nil {
		t.Fatalf("expected no metrics, found %+v", got)
	}
}

// Helper function
func int64Ptr(i int64) *int64 {
	return &i
}

