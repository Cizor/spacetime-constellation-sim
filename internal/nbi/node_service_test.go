// internal/nbi/node_service_test.go
package nbi

import (
	"context"
	"errors"
	"testing"

	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	core "github.com/signalsfoundry/constellation-simulator/core"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newNodeServiceForTest() (*NetworkNodeService, *sim.ScenarioState) {
	state := sim.NewScenarioState(kb.NewKnowledgeBase(), core.NewKnowledgeBase())
	return NewNetworkNodeService(state, nil), state
}

func addNodeWithInterface(t *testing.T, state *sim.ScenarioState, nodeID string) string {
	t.Helper()

	ifaceID := nodeID + "/if0"
	if err := state.CreateNode(
		&model.NetworkNode{ID: nodeID},
		[]*core.NetworkInterface{
			{
				ID:           ifaceID,
				ParentNodeID: nodeID,
				Medium:       core.MediumWired,
			},
		},
	); err != nil {
		t.Fatalf("CreateNode(%s) error: %v", nodeID, err)
	}
	return ifaceID
}

func TestNetworkNodeServiceDeleteSuccess(t *testing.T) {
	svc, state := newNodeServiceForTest()
	nodeID := "node-delete"
	ifaceID := addNodeWithInterface(t, state, nodeID)

	ctx := context.Background()
	if _, err := svc.DeleteNode(ctx, &v1alpha.DeleteNodeRequest{NodeId: &nodeID}); err != nil {
		t.Fatalf("DeleteNode RPC error: %v", err)
	}

	if _, _, err := state.GetNode(nodeID); !errors.Is(err, sim.ErrNodeNotFound) {
		t.Fatalf("GetNode after delete error = %v, want ErrNodeNotFound", err)
	}
	if got := state.NetworkKB().GetNetworkInterface(ifaceID); got != nil {
		t.Fatalf("NetworkKB interface %q = %+v, want nil", ifaceID, got)
	}
}

func TestNetworkNodeServiceDeleteNotFound(t *testing.T) {
	svc, _ := newNodeServiceForTest()
	ctx := context.Background()

	missing := "missing"
	if _, err := svc.DeleteNode(ctx, &v1alpha.DeleteNodeRequest{NodeId: &missing}); status.Code(err) != codes.NotFound {
		t.Fatalf("DeleteNode missing code = %v, want NotFound (err=%v)", status.Code(err), err)
	}
}

func TestNetworkNodeServiceDeleteFailedPreconditionForLink(t *testing.T) {
	svc, state := newNodeServiceForTest()
	nodeID := "node-in-use"
	ifaceID := addNodeWithInterface(t, state, nodeID)

	// Peer interface needed for link creation.
	if err := state.NetworkKB().AddInterface(&core.NetworkInterface{
		ID:           "peer/if0",
		ParentNodeID: "peer",
		Medium:       core.MediumWired,
	}); err != nil {
		t.Fatalf("AddInterface(peer) error: %v", err)
	}

	if err := state.CreateLink(&core.NetworkLink{
		ID:         "link-node-peer",
		InterfaceA: ifaceID,
		InterfaceB: "peer/if0",
		Medium:     core.MediumWired,
	}); err != nil {
		t.Fatalf("CreateLink error: %v", err)
	}

	ctx := context.Background()
	if _, err := svc.DeleteNode(ctx, &v1alpha.DeleteNodeRequest{NodeId: &nodeID}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("DeleteNode with link code = %v, want FailedPrecondition (err=%v)", status.Code(err), err)
	}

	// Node must remain.
	if _, _, err := state.GetNode(nodeID); err != nil {
		t.Fatalf("node should remain after failed delete; GetNode error = %v", err)
	}
	// Interface must remain.
	if got := state.NetworkKB().GetNetworkInterface(ifaceID); got == nil {
		t.Fatalf("interface should remain after failed delete; got nil")
	}
}

func TestNetworkNodeServiceDeleteFailedPreconditionForServiceRequest(t *testing.T) {
	svc, state := newNodeServiceForTest()
	nodeID := "node-with-sr"
	ifaceID := addNodeWithInterface(t, state, nodeID)

	if err := state.CreateServiceRequest(&model.ServiceRequest{
		ID:        "sr-uses-node",
		SrcNodeID: nodeID,
		DstNodeID: "other",
	}); err != nil {
		t.Fatalf("CreateServiceRequest error: %v", err)
	}

	ctx := context.Background()
	if _, err := svc.DeleteNode(ctx, &v1alpha.DeleteNodeRequest{NodeId: &nodeID}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("DeleteNode with service request code = %v, want FailedPrecondition (err=%v)", status.Code(err), err)
	}

	// Node must remain.
	if _, _, err := state.GetNode(nodeID); err != nil {
		t.Fatalf("node should remain after failed delete; GetNode error = %v", err)
	}
	// Interface must remain.
	if got := state.NetworkKB().GetNetworkInterface(ifaceID); got == nil {
		t.Fatalf("interface should remain after failed delete; got nil")
	}
}
