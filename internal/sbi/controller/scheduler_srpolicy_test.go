package controller

import (
	"context"
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestComputeSrPolicyPath(t *testing.T) {
	scheduler, linkIDs, now := setupThreeNodeScheduler(t, 0)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkIDs["linkAB"]: {{
			StartTime: now.Add(1 * time.Minute),
			EndTime:   now.Add(3 * time.Minute),
			Quality:   0,
		}},
		linkIDs["linkBC"]: {{
			StartTime: now.Add(5 * time.Minute),
			EndTime:   now.Add(7 * time.Minute),
			Quality:   0,
		}},
	}

	policy := &model.SrPolicy{
		PolicyID:      "policy-1",
		HeadendNodeID: "node-A",
		Endpoints:     []string{"node-C"},
		Segments: []model.Segment{
			{SID: "sid-ab", Type: "node", NodeID: "node-B"},
			{SID: "sid-bc", Type: "node", NodeID: "node-C"},
		},
	}

	path, err := scheduler.ComputeSrPolicyPath(context.Background(), policy)
	if err != nil {
		t.Fatalf("ComputeSrPolicyPath error: %v", err)
	}
	if path == nil || len(path.Hops) != 2 {
		t.Fatalf("expected two-hop path, got %+v", path)
	}
	if path.Hops[0].FromNodeID != "node-A" || path.Hops[0].ToNodeID != "node-B" {
		t.Fatalf("unexpected first hop %+v", path.Hops[0])
	}
	if path.Hops[1].FromNodeID != "node-B" || path.Hops[1].ToNodeID != "node-C" {
		t.Fatalf("unexpected second hop %+v", path.Hops[1])
	}
}

func TestComputeSrPolicyPathInvalidSegment(t *testing.T) {
	scheduler, _, now := setupPathfindingScheduler(t)
	scheduler.contactWindows = map[string][]ContactWindow{
		"link-ab": {{
			StartTime: now.Add(1 * time.Minute),
			EndTime:   now.Add(3 * time.Minute),
			Quality:   0,
		}},
	}

	policy := &model.SrPolicy{
		PolicyID:      "policy-2",
		HeadendNodeID: "node-A",
		Segments: []model.Segment{
			{SID: "bad", Type: "prefix"},
		},
	}

	if _, err := scheduler.ComputeSrPolicyPath(context.Background(), policy); err == nil {
		t.Fatalf("expected error for invalid segment")
	}
}

func TestComputeSrPolicyPathEndpointMismatch(t *testing.T) {
	scheduler, linkIDs, now := setupThreeNodeScheduler(t, 0)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkIDs["linkAB"]: {{
			StartTime: now.Add(1 * time.Minute),
			EndTime:   now.Add(3 * time.Minute),
			Quality:   0,
		}},
		linkIDs["linkBC"]: {{
			StartTime: now.Add(5 * time.Minute),
			EndTime:   now.Add(7 * time.Minute),
			Quality:   0,
		}},
	}

	policy := &model.SrPolicy{
		PolicyID:      "policy-3",
		HeadendNodeID: "node-A",
		Endpoints:     []string{"node-B"},
		Segments: []model.Segment{
			{SID: "sid-ab", Type: "node", NodeID: "node-B"},
			{SID: "sid-bc", Type: "node", NodeID: "node-C"},
		},
	}

	if _, err := scheduler.ComputeSrPolicyPath(context.Background(), policy); err == nil {
		t.Fatalf("expected error for endpoint mismatch")
	}
}

func TestScheduler_policyPathForServiceRequest(t *testing.T) {
	scheduler, linkIDs, now := setupThreeNodeScheduler(t, 0)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkIDs["linkAB"]: {{
			StartTime: now.Add(1 * time.Minute),
			EndTime:   now.Add(3 * time.Minute),
			Quality:   0,
		}},
		linkIDs["linkBC"]: {{
			StartTime: now.Add(5 * time.Minute),
			EndTime:   now.Add(7 * time.Minute),
			Quality:   0,
		}},
	}

	policy := &model.SrPolicy{
		PolicyID:      "policy-1",
		HeadendNodeID: "node-A",
		Endpoints:     []string{"node-C"},
		Segments: []model.Segment{
			{SID: "sid-ab", Type: "node", NodeID: "node-B"},
			{SID: "sid-bc", Type: "node", NodeID: "node-C"},
		},
	}
	if err := scheduler.State.InstallSrPolicy("node-A", policy); err != nil {
		t.Fatalf("InstallSrPolicy failed: %v", err)
	}

	sr := &model.ServiceRequest{
		ID:         "sr-policy",
		SrcNodeID:  "node-A",
		DstNodeID:  "node-C",
		SrPolicyID: policy.PolicyID,
	}
	path, err := scheduler.policyPathForServiceRequest(context.Background(), sr)
	if err != nil {
		t.Fatalf("policyPathForServiceRequest error: %v", err)
	}
	if path == nil || len(path.Hops) != 2 || path.Hops[1].ToNodeID != "node-C" {
		t.Fatalf("unexpected policy path: %+v", path)
	}
}

func TestScheduler_policyPathForServiceRequestMissingPolicy(t *testing.T) {
	scheduler, _, _ := setupThreeNodeScheduler(t, 0)
	sr := &model.ServiceRequest{
		ID:         "sr-missing",
		SrcNodeID:  "node-A",
		DstNodeID:  "node-B",
		SrPolicyID: "missing",
	}
	if _, err := scheduler.policyPathForServiceRequest(context.Background(), sr); err == nil {
		t.Fatalf("expected error for missing policy")
	}
}

func TestScheduler_policyPathForServiceRequestDestinationMismatch(t *testing.T) {
	scheduler, linkIDs, now := setupThreeNodeScheduler(t, 0)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkIDs["linkAB"]: {{
			StartTime: now.Add(1 * time.Minute),
			EndTime:   now.Add(3 * time.Minute),
			Quality:   0,
		}},
		linkIDs["linkBC"]: {{
			StartTime: now.Add(5 * time.Minute),
			EndTime:   now.Add(7 * time.Minute),
			Quality:   0,
		}},
	}
	policy := &model.SrPolicy{
		PolicyID:      "policy-2",
		HeadendNodeID: "node-A",
		Endpoints:     []string{"node-C"},
		Segments: []model.Segment{
			{SID: "sid-ab", Type: "node", NodeID: "node-B"},
			{SID: "sid-bc", Type: "node", NodeID: "node-C"},
		},
	}
	if err := scheduler.State.InstallSrPolicy("node-A", policy); err != nil {
		t.Fatalf("InstallSrPolicy failed: %v", err)
	}

	sr := &model.ServiceRequest{
		ID:         "sr-mismatch",
		SrcNodeID:  "node-A",
		DstNodeID:  "node-B",
		SrPolicyID: policy.PolicyID,
	}
	if _, err := scheduler.policyPathForServiceRequest(context.Background(), sr); err == nil {
		t.Fatalf("expected error for destination mismatch")
	}
}
