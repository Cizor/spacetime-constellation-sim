// Package sbi contains smoke tests for SBI scheduling protos.
//
// These are basic smoke tests that verify the generated scheduling proto code
// can be marshaled/unmarshaled successfully. Future SBI-related tests can
// extend these to validate more detailed mappings or invariants.
package sbi

import (
	"testing"

	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
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
		t.Fatalf("marshal failed: %v", err)
	}

	// Unmarshal
	var decoded schedulingpb.ReceiveRequestsMessageFromController
	if err := proto.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Sanity check: fields should survive the round-trip.
	if decoded.GetCreateEntry() == nil {
		t.Fatalf("CreateEntry was lost during marshal/unmarshal")
	}
	if decoded.GetCreateEntry().Id != "test-entry-1" {
		t.Fatalf("unexpected entry_id after round-trip: got %q, want %q",
			decoded.GetCreateEntry().Id, "test-entry-1")
	}
	if decoded.RequestId != 123 {
		t.Fatalf("RequestId mismatch: got %d, want 123", decoded.RequestId)
	}
	// Verify SetRoute round-trips
	setRoute := decoded.GetCreateEntry().GetSetRoute()
	if setRoute == nil {
		t.Fatalf("SetRoute was lost during marshal/unmarshal")
	}
	if setRoute.From != "10.0.0.0/24" {
		t.Fatalf("SetRoute.From mismatch: got %q, want %q", setRoute.From, "10.0.0.0/24")
	}
}

