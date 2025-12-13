// Package sbi contains smoke tests for SBI telemetry protos.
//
// These are basic smoke tests that verify the generated telemetry proto code
// can be marshaled/unmarshaled successfully. Future SBI-related tests can
// extend these to validate more detailed mappings or invariants.
package sbi

import (
	"testing"

	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestTelemetryProtoSmoke verifies that telemetry proto messages can be
// marshaled and unmarshaled successfully.
func TestTelemetryProtoSmoke(t *testing.T) {
	// Create a minimal ExportMetricsRequest
	now := timestamppb.Now()
	status := telemetrypb.IfOperStatus_IF_OPER_STATUS_UP
	req := &telemetrypb.ExportMetricsRequest{
		InterfaceMetrics: []*telemetrypb.InterfaceMetrics{
			{
				InterfaceId: proto.String("if-test"),
				OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{
					{
						Time:  now,
						Value: &status,
					},
				},
			},
		},
	}

	// Marshal
	data, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Unmarshal
	var decoded telemetrypb.ExportMetricsRequest
	if err := proto.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Sanity check that at least one key field round-trips.
	if len(decoded.GetInterfaceMetrics()) == 0 {
		t.Fatalf("expected at least one interface metrics entry after round-trip")
	}
	if decoded.InterfaceMetrics[0].GetInterfaceId() != "if-test" {
		t.Fatalf("InterfaceId mismatch: got %q, want %q",
			decoded.InterfaceMetrics[0].GetInterfaceId(), "if-test")
	}
	if len(decoded.InterfaceMetrics[0].OperationalStateDataPoints) != 1 {
		t.Fatalf("OperationalStateDataPoints count mismatch: got %d, want 1",
			len(decoded.InterfaceMetrics[0].OperationalStateDataPoints))
	}
}

