// internal/nbi/servicerequest_service_test.go
package nbi

import (
	"context"
	"testing"

	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newServiceRequestServiceForTest wires up a ServiceRequestService with an
// in-memory ScenarioState.
func newServiceRequestServiceForTest() (*ServiceRequestService, *sim.ScenarioState) {
	state := newScenarioStateForTest()
	return NewServiceRequestService(state, nil), state
}

func flowReq(bps float64) *resources.ServiceRequest_FlowRequirements {
	return &resources.ServiceRequest_FlowRequirements{
		BandwidthBpsRequested: &bps,
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func floatPtr(f float64) *float64 {
	return &f
}

// -----------------------------------------------------------------------------
// CreateServiceRequest
// -----------------------------------------------------------------------------

func TestServiceRequestServiceCreate(t *testing.T) {
	svc, state := newServiceRequestServiceForTest()

	addNodeWithInterface(t, state, "src-node")
	addNodeWithInterface(t, state, "dst-node")

	id := "sr-create"
	bandwidthBps := 1_000_000.0
	req := &resources.ServiceRequest{
		Type:    &id,
		SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "src-node"},
		DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "dst-node"},
		Requirements: []*resources.ServiceRequest_FlowRequirements{
			{BandwidthBpsRequested: &bandwidthBps},
		},
	}

	resp, err := svc.CreateServiceRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateServiceRequest error: %v", err)
	}
	if resp.GetType() != id {
		t.Fatalf("CreateServiceRequest response Type = %q, want %q", resp.GetType(), id)
	}

	stored, err := state.GetServiceRequest(id)
	if err != nil {
		t.Fatalf("state.GetServiceRequest error: %v", err)
	}
	if stored.SrcNodeID != "src-node" || stored.DstNodeID != "dst-node" {
		t.Fatalf("stored ServiceRequest endpoints = (%s, %s), want (src-node, dst-node)", stored.SrcNodeID, stored.DstNodeID)
	}
	if len(stored.FlowRequirements) != 1 {
		t.Fatalf("stored FlowRequirements length = %d, want 1", len(stored.FlowRequirements))
	}
	if got, want := stored.FlowRequirements[0].RequestedBandwidth, bandwidthBps; got != want {
		t.Fatalf("stored FlowRequirements[0].RequestedBandwidth = %f, want %f", got, want)
	}
}

func TestServiceRequestServiceCreateGeneratesID(t *testing.T) {
	svc, state := newServiceRequestServiceForTest()

	addNodeWithInterface(t, state, "node-a")
	addNodeWithInterface(t, state, "node-b")

	req := &resources.ServiceRequest{
		SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "node-a"},
		DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "node-b"},
		Requirements: []*resources.ServiceRequest_FlowRequirements{
			flowReq(500_000), // 500 kbps
		},
	}

	resp, err := svc.CreateServiceRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateServiceRequest error: %v", err)
	}
	if resp.GetType() == "" {
		t.Fatalf("CreateServiceRequest response Type should carry generated ID")
	}

	if _, err := state.GetServiceRequest(resp.GetType()); err != nil {
		t.Fatalf("GetServiceRequest(generated) error: %v", err)
	}
}

func TestServiceRequestServiceCreateValidation(t *testing.T) {
	tests := []struct {
		name string
		req  *resources.ServiceRequest
	}{
		{
			name: "missing src node",
			req: &resources.ServiceRequest{
				SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "missing"},
				DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "dst"},
				Requirements: []*resources.ServiceRequest_FlowRequirements{
					flowReq(1_000_000),
				},
			},
		},
		{
			name: "missing dst node",
			req: &resources.ServiceRequest{
				SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "src"},
				DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "missing-dst"},
				Requirements: []*resources.ServiceRequest_FlowRequirements{
					flowReq(1_000_000),
				},
			},
		},
		{
			name: "no flow requirements",
			req: &resources.ServiceRequest{
				SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "src"},
				DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "dst"},
			},
		},
		{
			name: "negative bandwidth",
			req: &resources.ServiceRequest{
				SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "src"},
				DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "dst"},
				Requirements: []*resources.ServiceRequest_FlowRequirements{
					flowReq(-1),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, state := newServiceRequestServiceForTest()
			// Seed nodes so only the intended validation triggers when applicable.
			addNodeWithInterface(t, state, "src")
			addNodeWithInterface(t, state, "dst")

			_, err := svc.CreateServiceRequest(context.Background(), tc.req)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("CreateServiceRequest(%s) code = %v, want InvalidArgument (err=%v)", tc.name, status.Code(err), err)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Get/List
// -----------------------------------------------------------------------------

func TestServiceRequestServiceGetAndList(t *testing.T) {
	svc, state := newServiceRequestServiceForTest()
	addNodeWithInterface(t, state, "node-1")
	addNodeWithInterface(t, state, "node-2")

	id1 := "sr-1"
	id2 := "sr-2"
	for _, id := range []string{id1, id2} {
		bw := 10_000.0
		req := &resources.ServiceRequest{
			Type:    &id,
			SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "node-1"},
			DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "node-2"},
			Requirements: []*resources.ServiceRequest_FlowRequirements{
				{BandwidthBpsRequested: &bw},
			},
		}
		if _, err := svc.CreateServiceRequest(context.Background(), req); err != nil {
			t.Fatalf("CreateServiceRequest(%s) error: %v", id, err)
		}
	}

	getResp, err := svc.GetServiceRequest(context.Background(), &v1alpha.GetServiceRequestRequest{
		ServiceRequestId: &id1,
	})
	if err != nil {
		t.Fatalf("GetServiceRequest error: %v", err)
	}
	if getResp.GetType() != id1 {
		t.Fatalf("GetServiceRequest Type = %q, want %q", getResp.GetType(), id1)
	}

	listResp, err := svc.ListServiceRequests(context.Background(), &v1alpha.ListServiceRequestsRequest{})
	if err != nil {
		t.Fatalf("ListServiceRequests error: %v", err)
	}
	if got, want := len(listResp.GetServiceRequests()), 2; got != want {
		t.Fatalf("ListServiceRequests count = %d, want %d", got, want)
	}
}

func TestServiceRequestServiceGetNotFound(t *testing.T) {
	svc, _ := newServiceRequestServiceForTest()

	id := "missing-service-request"
	_, err := svc.GetServiceRequest(context.Background(), &v1alpha.GetServiceRequestRequest{
		ServiceRequestId: &id,
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("GetServiceRequest missing code = %v, want NotFound (err=%v)", status.Code(err), err)
	}
}

// -----------------------------------------------------------------------------
// Update
// -----------------------------------------------------------------------------

func TestServiceRequestServiceUpdate(t *testing.T) {
	svc, state := newServiceRequestServiceForTest()
	addNodeWithInterface(t, state, "node-a")
	addNodeWithInterface(t, state, "node-b")

	id := "sr-update"
	initialBps := 100_000.0
	createReq := &resources.ServiceRequest{
		Type:    &id,
		SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "node-a"},
		DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "node-b"},
		Requirements: []*resources.ServiceRequest_FlowRequirements{
			{BandwidthBpsRequested: &initialBps},
		},
	}
	if _, err := svc.CreateServiceRequest(context.Background(), createReq); err != nil {
		t.Fatalf("CreateServiceRequest seed error: %v", err)
	}

	newBps := 200_000.0
	updateReq := &v1alpha.UpdateServiceRequestRequest{
		ServiceRequest: &resources.ServiceRequest{
			Type:    &id,
			SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "node-a"},
			DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "node-b"},
			Requirements: []*resources.ServiceRequest_FlowRequirements{
				{BandwidthBpsRequested: &newBps},
			},
			Priority:              floatPtr(5),
			AllowPartnerResources: boolPtr(true),
		},
	}

	if _, err := svc.UpdateServiceRequest(context.Background(), updateReq); err != nil {
		t.Fatalf("UpdateServiceRequest error: %v", err)
	}

	stored, err := state.GetServiceRequest(id)
	if err != nil {
		t.Fatalf("GetServiceRequest after update error: %v", err)
	}
	if stored.Priority != 5 || !stored.AllowPartnerResources {
		t.Fatalf("stored ServiceRequest after update = %+v, want Priority=5 AllowPartnerResources=true", stored)
	}
	if len(stored.FlowRequirements) != 1 {
		t.Fatalf("stored FlowRequirements length after update = %d, want 1", len(stored.FlowRequirements))
	}
	if got, want := stored.FlowRequirements[0].RequestedBandwidth, newBps; got != want {
		t.Fatalf("stored FlowRequirements after update RequestedBandwidth = %f, want %f", got, want)
	}
}

func TestServiceRequestServiceUpdateErrors(t *testing.T) {
	svc, state := newServiceRequestServiceForTest()
	addNodeWithInterface(t, state, "node-a")
	addNodeWithInterface(t, state, "node-b")

	// Seed one ServiceRequest.
	id := "sr-existing"
	bw := 1_000_000.0
	createReq := &resources.ServiceRequest{
		Type:    &id,
		SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "node-a"},
		DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "node-b"},
		Requirements: []*resources.ServiceRequest_FlowRequirements{
			{BandwidthBpsRequested: &bw},
		},
	}
	if _, err := svc.CreateServiceRequest(context.Background(), createReq); err != nil {
		t.Fatalf("CreateServiceRequest seed error: %v", err)
	}

	// Missing ID should be invalid.
	_, err := svc.UpdateServiceRequest(context.Background(), &v1alpha.UpdateServiceRequestRequest{
		ServiceRequest: &resources.ServiceRequest{
			SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "node-a"},
			DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "node-b"},
			Requirements: []*resources.ServiceRequest_FlowRequirements{
				flowReq(1_000_000),
			},
		},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("UpdateServiceRequest missing ID code = %v, want InvalidArgument (err=%v)", status.Code(err), err)
	}

	// Unknown ID should be NotFound.
	missing := "sr-missing"
	_, err = svc.UpdateServiceRequest(context.Background(), &v1alpha.UpdateServiceRequestRequest{
		ServiceRequest: &resources.ServiceRequest{
			Type:    &missing,
			SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "node-a"},
			DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "node-b"},
			Requirements: []*resources.ServiceRequest_FlowRequirements{
				flowReq(1_000_000),
			},
		},
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("UpdateServiceRequest missing code = %v, want NotFound (err=%v)", status.Code(err), err)
	}
}

// -----------------------------------------------------------------------------
// Delete
// -----------------------------------------------------------------------------

func TestServiceRequestServiceDelete(t *testing.T) {
	svc, state := newServiceRequestServiceForTest()
	addNodeWithInterface(t, state, "node-a")
	addNodeWithInterface(t, state, "node-b")

	id := "sr-delete"
	bw := 1_000_000.0
	req := &resources.ServiceRequest{
		Type:    &id,
		SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "node-a"},
		DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "node-b"},
		Requirements: []*resources.ServiceRequest_FlowRequirements{
			{BandwidthBpsRequested: &bw},
		},
	}
	if _, err := svc.CreateServiceRequest(context.Background(), req); err != nil {
		t.Fatalf("CreateServiceRequest seed error: %v", err)
	}

	if _, err := svc.DeleteServiceRequest(context.Background(), &v1alpha.DeleteServiceRequestRequest{
		ServiceRequestId: &id,
	}); err != nil {
		t.Fatalf("DeleteServiceRequest error: %v", err)
	}
	if _, err := svc.GetServiceRequest(context.Background(), &v1alpha.GetServiceRequestRequest{
		ServiceRequestId: &id,
	}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetServiceRequest after delete code = %v, want NotFound (err=%v)", status.Code(err), err)
	}
	if _, err := state.GetServiceRequest(id); err == nil {
		t.Fatalf("expected service request to be deleted from state")
	}

	if _, err := svc.DeleteServiceRequest(context.Background(), &v1alpha.DeleteServiceRequestRequest{
		ServiceRequestId: &id,
	}); status.Code(err) != codes.NotFound {
		t.Fatalf("DeleteServiceRequest second call code = %v, want NotFound (err=%v)", status.Code(err), err)
	}
}
