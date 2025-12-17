package controller

import (
	"context"
	"testing"

	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestPreemptConflictingServiceRequests(t *testing.T) {
	scheduler, linkID, _ := setupPathfindingScheduler(t)

	low := &model.ServiceRequest{
		ID:               "sr-low",
		SrcNodeID:        "node-A",
		DstNodeID:        "node-B",
		Priority:         1,
		IsProvisionedNow: true,
	}
	if err := scheduler.State.CreateServiceRequest(low); err != nil {
		t.Fatalf("CreateServiceRequest(low) failed: %v", err)
	}
	if err := scheduler.State.UpdateServiceRequest(low); err != nil {
		t.Fatalf("UpdateServiceRequest(low) failed: %v", err)
	}
	scheduler.bandwidthReservations[low.ID] = map[string]uint64{linkID: 1000000}

	incoming := &model.ServiceRequest{
		ID:        "sr-high",
		SrcNodeID: "node-A",
		DstNodeID: "node-B",
		Priority:  10,
	}
	path := []string{incoming.SrcNodeID, incoming.DstNodeID}
	ok, err := scheduler.preemptConflictingSRs(context.Background(), incoming, path, 500000, []string{linkID})
	if err != nil {
		t.Fatalf("preemptConflictingSRs failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected preemption to succeed")
	}
	if _, exists := scheduler.bandwidthReservations[low.ID]; exists {
		t.Fatalf("expected low priority reservation removed")
	}

	status, err := scheduler.State.GetServiceRequestStatus(low.ID)
	if err != nil {
		t.Fatalf("GetServiceRequestStatus failed: %v", err)
	}
	if status.IsProvisionedNow {
		t.Fatalf("low-priority service request should be unprovisioned")
	}

	record, found := scheduler.preemptionRecords[low.ID]
	if !found {
		t.Fatalf("preemption record missing for %s", low.ID)
	}
	if record.PreemptedBy != incoming.ID {
		t.Fatalf("preemption record PreemptedBy = %q, want %q", record.PreemptedBy, incoming.ID)
	}
}
