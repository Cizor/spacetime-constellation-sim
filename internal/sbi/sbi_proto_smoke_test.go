package sbi

import (
	"testing"

	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestSchedulingProtoSmoke verifies that scheduling proto messages can be
// marshaled and unmarshaled successfully, confirming the generated code is usable.
func TestSchedulingProtoSmoke(t *testing.T) {
	// Create a minimal ReceiveRequestsMessageFromController with CreateEntryRequest
	// Using SetRoute which is simpler than UpdateBeam
	msg := &schedulingpb.ReceiveRequestsMessageFromController{
		RequestId: 123,
		Request: &schedulingpb.ReceiveRequestsMessageFromController_CreateEntry{
			CreateEntry: &schedulingpb.CreateEntryRequest{
				ScheduleManipulationToken: "test-token",
				Seqno:                     1,
				Id:                        "test-entry-1",
				Time:                      timestamppb.Now(),
				ConfigurationChange: &schedulingpb.CreateEntryRequest_SetRoute{
					SetRoute: &schedulingpb.SetRoute{
						From: "10.0.0.0/24",
						To:   "192.168.1.0/24",
						Dev:  "if-test",
					},
				},
			},
		},
	}

	// Marshal
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal ReceiveRequestsMessageFromController: %v", err)
	}

	// Unmarshal
	unmarshaled := &schedulingpb.ReceiveRequestsMessageFromController{}
	if err := proto.Unmarshal(data, unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal ReceiveRequestsMessageFromController: %v", err)
	}

	// Verify key fields round-trip
	if unmarshaled.GetCreateEntry() == nil {
		t.Fatalf("CreateEntry was lost during marshal/unmarshal")
	}
	if unmarshaled.GetCreateEntry().Id != "test-entry-1" {
		t.Fatalf("Id mismatch: got %q, want %q",
			unmarshaled.GetCreateEntry().Id, "test-entry-1")
	}
	if unmarshaled.RequestId != 123 {
		t.Fatalf("RequestId mismatch: got %d, want 123", unmarshaled.RequestId)
	}
	// Verify SetRoute round-trips
	setRoute := unmarshaled.GetCreateEntry().GetSetRoute()
	if setRoute == nil {
		t.Fatalf("SetRoute was lost during marshal/unmarshal")
	}
	if setRoute.From != "10.0.0.0/24" {
		t.Fatalf("SetRoute.From mismatch: got %q, want %q", setRoute.From, "10.0.0.0/24")
	}
}

// TestTelemetryProtoSmoke verifies that telemetry proto messages can be
// marshaled and unmarshaled successfully.
func TestTelemetryProtoSmoke(t *testing.T) {
	// Create a minimal ExportMetricsRequest
	now := timestamppb.Now()
	status := telemetrypb.IfOperStatus_IF_OPER_STATUS_UP
	msg := &telemetrypb.ExportMetricsRequest{
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
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal ExportMetricsRequest: %v", err)
	}

	// Unmarshal
	unmarshaled := &telemetrypb.ExportMetricsRequest{}
	if err := proto.Unmarshal(data, unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal ExportMetricsRequest: %v", err)
	}

	// Verify key fields round-trip
	if len(unmarshaled.InterfaceMetrics) != 1 {
		t.Fatalf("InterfaceMetrics count mismatch: got %d, want 1", len(unmarshaled.InterfaceMetrics))
	}
	if unmarshaled.InterfaceMetrics[0].GetInterfaceId() != "if-test" {
		t.Fatalf("InterfaceId mismatch: got %q, want %q",
			unmarshaled.InterfaceMetrics[0].GetInterfaceId(), "if-test")
	}
	if len(unmarshaled.InterfaceMetrics[0].OperationalStateDataPoints) != 1 {
		t.Fatalf("OperationalStateDataPoints count mismatch: got %d, want 1",
			len(unmarshaled.InterfaceMetrics[0].OperationalStateDataPoints))
	}
}

