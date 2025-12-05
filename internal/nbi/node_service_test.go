// internal/nbi/node_service_test.go
package nbi

import (
	"context"
	"errors"
	"testing"

	common "aalyria.com/spacetime/api/common"
	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
	core "github.com/signalsfoundry/constellation-simulator/core"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newNodeServiceForTest creates a NetworkNodeService with in-memory KBs.
func newNodeServiceForTest() (*NetworkNodeService, *sim.ScenarioState) {
	state := sim.NewScenarioState(kb.NewKnowledgeBase(), core.NewKnowledgeBase())
	return NewNetworkNodeService(state, nil), state
}

// newNodeServiceWithTransceivers creates a service with a minimal
// transceiver-model registry populated so wireless interface validation
// can pass/fail.
func newNodeServiceWithTransceivers(trxIDs ...string) (*NetworkNodeService, *sim.ScenarioState) {
	state := sim.NewScenarioState(kb.NewKnowledgeBase(), core.NewKnowledgeBase())
	for _, id := range trxIDs {
		if err := state.NetworkKB().AddTransceiverModel(&core.TransceiverModel{
			ID:   id,
			Name: id,
			Band: core.FrequencyBand{MinGHz: 1, MaxGHz: 2},
		}); err != nil {
			panic(err)
		}
	}
	return NewNetworkNodeService(state, nil), state
}

func requirePlatform(t *testing.T, state *sim.ScenarioState, id string) {
	t.Helper()

	if err := state.CreatePlatform(&model.PlatformDefinition{
		ID:   id,
		Name: id,
		Type: "SATELLITE",
	}); err != nil {
		t.Fatalf("CreatePlatform(%s) error: %v", id, err)
	}
}

func strPtr(s string) *string {
	return &s
}

func wiredInterfaceProto(id, platformID string) *resources.NetworkInterface {
	return &resources.NetworkInterface{
		InterfaceId: strPtr(id),
		InterfaceMedium: &resources.NetworkInterface_Wired{
			Wired: &resources.WiredDevice{
				PlatformId: strPtr(platformID),
			},
		},
	}
}

func wirelessInterfaceProto(id, trxID, platformID string) *resources.NetworkInterface {
	iface := &resources.NetworkInterface{
		InterfaceId: strPtr(id),
		InterfaceMedium: &resources.NetworkInterface_Wireless{
			Wireless: &resources.WirelessDevice{
				Platform: strPtr(platformID),
			},
		},
	}
	if trxID != "" {
		iface.GetWireless().TransceiverModelId = &common.TransceiverModelId{
			TransceiverModelId: strPtr(trxID),
		}
	}
	return iface
}

// addNodeWithInterface seeds ScenarioState with a node with a single interface
// and returns the interface ID.
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

// -----------------------------------------------------------------------------
// CreateNode: happy path
// -----------------------------------------------------------------------------

func TestNetworkNodeServiceCreateNodeWithInterfaces(t *testing.T) {
	trxID := "trx-create"
	svc, state := newNodeServiceWithTransceivers(trxID)

	platformID := "platform-create"
	requirePlatform(t, state, platformID)

	nodeID := "node-create"
	req := &resources.NetworkNode{
		NodeId: strPtr(nodeID),
		Name:   strPtr("node-create-name"),
		Type:   strPtr("ROUTER"),
		NodeInterface: []*resources.NetworkInterface{
			wiredInterfaceProto("eth0", platformID),
			wirelessInterfaceProto("rf0", trxID, platformID),
		},
	}

	resp, err := svc.CreateNode(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateNode error: %v", err)
	}

	if resp.GetNodeId() != nodeID || resp.GetName() != "node-create-name" || resp.GetType() != "ROUTER" {
		t.Fatalf("CreateNode response = %+v, want node_id %s name %s type ROUTER", resp, nodeID, "node-create-name")
	}
	if len(resp.GetNodeInterface()) != 2 {
		t.Fatalf("CreateNode response interfaces = %d, want 2", len(resp.GetNodeInterface()))
	}

	storedNode, storedIfaces, err := state.GetNode(nodeID)
	if err != nil {
		t.Fatalf("state.GetNode error: %v", err)
	}
	if storedNode.PlatformID != platformID {
		t.Fatalf("stored PlatformID = %q, want %q", storedNode.PlatformID, platformID)
	}
	if len(storedIfaces) != 2 {
		t.Fatalf("stored interfaces = %d, want 2", len(storedIfaces))
	}

	if got := state.NetworkKB().GetNetworkInterface("node-create/eth0"); got == nil || got.Medium != core.MediumWired {
		t.Fatalf("NetworkKB eth0 = %+v, want wired interface", got)
	}
	wireless := state.NetworkKB().GetNetworkInterface("node-create/rf0")
	if wireless == nil || wireless.Medium != core.MediumWireless || wireless.TransceiverID != trxID {
		t.Fatalf("NetworkKB rf0 = %+v, want wireless with transceiver %s", wireless, trxID)
	}
}

// -----------------------------------------------------------------------------
// CreateNode: validation failures
// -----------------------------------------------------------------------------

func TestNetworkNodeServiceCreateNodeValidationErrors(t *testing.T) {
	trxID := "trx-valid"
	svc, state := newNodeServiceWithTransceivers(trxID)

	validPlatform := "platform-valid"
	requirePlatform(t, state, validPlatform)

	tests := []struct {
		name string
		node *resources.NetworkNode
		code codes.Code
	}{
		{
			name: "non-existent platform",
			node: &resources.NetworkNode{
				NodeId: strPtr("node-no-platform"),
				Type:   strPtr("ROUTER"),
				NodeInterface: []*resources.NetworkInterface{
					wiredInterfaceProto("if0", "missing-platform"),
				},
			},
			code: codes.InvalidArgument,
		},
		{
			name: "duplicate interfaces",
			node: &resources.NetworkNode{
				NodeId: strPtr("node-dup-iface"),
				Type:   strPtr("ROUTER"),
				NodeInterface: []*resources.NetworkInterface{
					wiredInterfaceProto("dup", validPlatform),
					wirelessInterfaceProto("dup", trxID, validPlatform),
				},
			},
			code: codes.InvalidArgument,
		},
		{
			name: "wireless missing transceiver reference",
			node: &resources.NetworkNode{
				NodeId: strPtr("node-missing-trx-ref"),
				Type:   strPtr("ROUTER"),
				NodeInterface: []*resources.NetworkInterface{
					wirelessInterfaceProto("rf0", "", validPlatform),
				},
			},
			code: codes.InvalidArgument,
		},
		{
			name: "wireless unknown transceiver model",
			node: &resources.NetworkNode{
				NodeId: strPtr("node-unknown-trx"),
				Type:   strPtr("ROUTER"),
				NodeInterface: []*resources.NetworkInterface{
					wirelessInterfaceProto("rf0", "missing-trx", validPlatform),
				},
			},
			code: codes.InvalidArgument,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateNode(context.Background(), tc.node)
			if status.Code(err) != tc.code {
				t.Fatalf("CreateNode(%s) code = %v, want %v (err=%v)", tc.name, status.Code(err), tc.code, err)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// GetNode / ListNodes
// -----------------------------------------------------------------------------

func TestNetworkNodeServiceGetAndList(t *testing.T) {
	trxID := "trx-list"
	svc, state := newNodeServiceWithTransceivers(trxID)

	platformID := "platform-list"
	requirePlatform(t, state, platformID)

	nodes := []string{"node-a", "node-b"}
	for _, id := range nodes {
		req := &resources.NetworkNode{
			NodeId: strPtr(id),
			Type:   strPtr("ROUTER"),
			NodeInterface: []*resources.NetworkInterface{
				wiredInterfaceProto("eth0", platformID),
				wirelessInterfaceProto("rf0", trxID, platformID),
			},
		}
		if _, err := svc.CreateNode(context.Background(), req); err != nil {
			t.Fatalf("CreateNode(%s) error: %v", id, err)
		}
	}

	getResp, err := svc.GetNode(context.Background(), &v1alpha.GetNodeRequest{NodeId: strPtr(nodes[0])})
	if err != nil {
		t.Fatalf("GetNode error: %v", err)
	}
	if getResp.GetNodeId() != nodes[0] || len(getResp.GetNodeInterface()) != 2 {
		t.Fatalf("GetNode response = %+v, want node_id %s with 2 interfaces", getResp, nodes[0])
	}

	listResp, err := svc.ListNodes(context.Background(), &v1alpha.ListNodesRequest{})
	if err != nil {
		t.Fatalf("ListNodes error: %v", err)
	}
	if len(listResp.GetNodes()) != len(nodes) {
		t.Fatalf("ListNodes count = %d, want %d", len(listResp.GetNodes()), len(nodes))
	}
}

// -----------------------------------------------------------------------------
// UpdateNode
// -----------------------------------------------------------------------------

func TestNetworkNodeServiceUpdateNode(t *testing.T) {
	trxID := "trx-update"
	svc, state := newNodeServiceWithTransceivers(trxID)

	platformID := "platform-update"
	requirePlatform(t, state, platformID)

	nodeID := "node-update"
	createReq := &resources.NetworkNode{
		NodeId: strPtr(nodeID),
		Type:   strPtr("ROUTER"),
		NodeInterface: []*resources.NetworkInterface{
			wiredInterfaceProto("eth0", platformID),
		},
	}
	if _, err := svc.CreateNode(context.Background(), createReq); err != nil {
		t.Fatalf("CreateNode seed error: %v", err)
	}

	// Replace interfaces and update name.
	updateReq := &v1alpha.UpdateNodeRequest{
		Node: &resources.NetworkNode{
			NodeId: strPtr(nodeID),
			Name:   strPtr("node-update-new-name"),
			Type:   strPtr("ROUTER"),
			NodeInterface: []*resources.NetworkInterface{
				wiredInterfaceProto("eth1", platformID),
				wirelessInterfaceProto("rf0", trxID, platformID),
			},
		},
	}
	if _, err := svc.UpdateNode(context.Background(), updateReq); err != nil {
		t.Fatalf("UpdateNode error: %v", err)
	}

	stored, ifaces, err := state.GetNode(nodeID)
	if err != nil {
		t.Fatalf("state.GetNode after update error: %v", err)
	}
	if stored.Name != "node-update-new-name" {
		t.Fatalf("stored node name = %q, want node-update-new-name", stored.Name)
	}
	if stored.PlatformID != platformID {
		t.Fatalf("stored PlatformID = %q, want %q", stored.PlatformID, platformID)
	}
	if len(ifaces) != 2 {
		t.Fatalf("stored interfaces after update = %d, want 2", len(ifaces))
	}

	if state.NetworkKB().GetNetworkInterface("node-update/eth0") != nil {
		t.Fatalf("expected eth0 to be removed on replace")
	}
	if state.NetworkKB().GetNetworkInterface("node-update/eth1") == nil ||
		state.NetworkKB().GetNetworkInterface("node-update/rf0") == nil {
		t.Fatalf("expected eth1 and rf0 to exist after update")
	}

	// Duplicate interface IDs in update should fail validation.
	dupReq := &v1alpha.UpdateNodeRequest{
		Node: &resources.NetworkNode{
			NodeId: strPtr(nodeID),
			Type:   strPtr("ROUTER"),
			NodeInterface: []*resources.NetworkInterface{
				wiredInterfaceProto("dup", platformID),
				wirelessInterfaceProto("dup", trxID, platformID),
			},
		},
	}
	if _, err := svc.UpdateNode(context.Background(), dupReq); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("UpdateNode duplicate interfaces code = %v, want InvalidArgument (err=%v)", status.Code(err), err)
	}
}

// -----------------------------------------------------------------------------
// DeleteNode: success and error semantics
// -----------------------------------------------------------------------------

func TestNetworkNodeServiceDeleteSuccess(t *testing.T) {
	svc, state := newNodeServiceForTest()

	nodeID := "node-delete"
	ifaceID := addNodeWithInterface(t, state, nodeID)

	ctx := context.Background()
	if _, err := svc.DeleteNode(ctx, &v1alpha.DeleteNodeRequest{NodeId: strPtr(nodeID)}); err != nil {
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
	if _, err := svc.DeleteNode(ctx, &v1alpha.DeleteNodeRequest{NodeId: strPtr(missing)}); status.Code(err) != codes.NotFound {
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
	if _, err := svc.DeleteNode(ctx, &v1alpha.DeleteNodeRequest{NodeId: strPtr(nodeID)}); status.Code(err) != codes.FailedPrecondition {
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
	if _, err := svc.DeleteNode(ctx, &v1alpha.DeleteNodeRequest{NodeId: strPtr(nodeID)}); status.Code(err) != codes.FailedPrecondition {
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
