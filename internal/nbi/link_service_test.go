// internal/nbi/link_service_test.go
package nbi

import (
	"context"
	"testing"

	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
	core "github.com/signalsfoundry/constellation-simulator/core"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newLinkServiceForTest creates a NetworkLinkService backed by in-memory KBs.
func newLinkServiceForTest() (*NetworkLinkService, *sim.ScenarioState) {
	state := newScenarioStateForTest()
	return NewNetworkLinkService(state, nil), state
}

// addTransceiver registers a minimal transceiver model for wireless interfaces.
func addTransceiver(t *testing.T, state *sim.ScenarioState, id string) {
	t.Helper()
	if err := state.NetworkKB().AddTransceiverModel(&core.TransceiverModel{
		ID:   id,
		Name: id,
		Band: core.FrequencyBand{MinGHz: 1, MaxGHz: 2},
	}); err != nil {
		t.Fatalf("AddTransceiverModel(%s) error: %v", id, err)
	}
}

// addNodeWithMedium seeds ScenarioState with a node and single interface.
func addNodeWithMedium(t *testing.T, state *sim.ScenarioState, nodeID, ifaceLocal string, medium core.MediumType, trxID string) string {
	t.Helper()

	iface := &core.NetworkInterface{
		ID:           nodeID + "/" + ifaceLocal,
		ParentNodeID: nodeID,
		Medium:       medium,
		TransceiverID: func() string {
			if medium == core.MediumWireless {
				return trxID
			}
			return ""
		}(),
	}

	if err := state.CreateNode(&model.NetworkNode{ID: nodeID}, []*core.NetworkInterface{iface}); err != nil {
		t.Fatalf("CreateNode(%s) error: %v", nodeID, err)
	}
	return iface.ID
}

// -----------------------------------------------------------------------------
// CreateLink: wired happy path + always-on semantics
// -----------------------------------------------------------------------------

func TestNetworkLinkServiceCreateStaticWiredLink(t *testing.T) {
	ctx := context.Background()
	svc, state := newLinkServiceForTest()

	nodeA := "node-wired-a"
	nodeB := "node-wired-b"

	ifaceA := addNodeWithMedium(t, state, nodeA, "fiber0", core.MediumWired, "")
	ifaceB := addNodeWithMedium(t, state, nodeB, "fiber0", core.MediumWired, "")

	req := &resources.BidirectionalLink{
		ANetworkNodeId: strPtr(nodeA),
		ATxInterfaceId: strPtr("fiber0"),
		ARxInterfaceId: strPtr("fiber0"),
		BNetworkNodeId: strPtr(nodeB),
		BTxInterfaceId: strPtr("fiber0"),
		BRxInterfaceId: strPtr("fiber0"),
	}

	resp, err := svc.CreateLink(ctx, req)
	if err != nil {
		t.Fatalf("CreateLink error: %v", err)
	}

	if resp.GetANetworkNodeId() != nodeA || resp.GetBNetworkNodeId() != nodeB {
		t.Fatalf("CreateLink response nodes = (%s, %s), want (%s, %s)",
			resp.GetANetworkNodeId(), resp.GetBNetworkNodeId(), nodeA, nodeB)
	}
	if resp.GetATxInterfaceId() != "fiber0" || resp.GetBTxInterfaceId() != "fiber0" {
		t.Fatalf("CreateLink response interfaces = (%s, %s), want fiber0/fiber0",
			resp.GetATxInterfaceId(), resp.GetBTxInterfaceId())
	}

	links := state.ListLinks()
	if len(links) != 1 {
		t.Fatalf("scenario links count = %d, want 1 bidirectional link", len(links))
	}

	l := links[0]
	if l.InterfaceA != ifaceA || l.InterfaceB != ifaceB {
		t.Fatalf("stored link endpoints = (%s, %s), want (%s, %s)", l.InterfaceA, l.InterfaceB, ifaceA, ifaceB)
	}
	if l.Medium != core.MediumWired {
		t.Fatalf("stored link %+v Medium = %s, want %s", l, l.Medium, core.MediumWired)
	}
	if !l.IsStatic || !l.IsUp {
		t.Fatalf("wired link %+v should be marked always-on (IsStatic && IsUp)", l)
	}
}

// -----------------------------------------------------------------------------
// CreateLink: invalid endpoints
// -----------------------------------------------------------------------------

func TestNetworkLinkServiceCreateLinkInvalidEndpoints(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		req  *resources.BidirectionalLink
		svc  func(t *testing.T) (*NetworkLinkService, *sim.ScenarioState)
	}{
		{
			name: "missing interfaces",
			req: &resources.BidirectionalLink{
				ANetworkNodeId: strPtr("node-a"),
				ATxInterfaceId: strPtr("missing-a"),
				ARxInterfaceId: strPtr("missing-a"),
				BNetworkNodeId: strPtr("node-b"),
				BTxInterfaceId: strPtr("missing-b"),
				BRxInterfaceId: strPtr("missing-b"),
			},
			svc: func(t *testing.T) (*NetworkLinkService, *sim.ScenarioState) {
				return newLinkServiceForTest()
			},
		},
		{
			name: "interfaces for unknown nodes",
			req: &resources.BidirectionalLink{
				ANetworkNodeId: strPtr("ghost"),
				ATxInterfaceId: strPtr("ifA"),
				ARxInterfaceId: strPtr("ifA"),
				BNetworkNodeId: strPtr("ghost"),
				BTxInterfaceId: strPtr("ifB"),
				BRxInterfaceId: strPtr("ifB"),
			},
			svc: func(t *testing.T) (*NetworkLinkService, *sim.ScenarioState) {
				svc, state := newLinkServiceForTest()
				if err := state.NetworkKB().AddInterface(&core.NetworkInterface{
					ID:           "ghost/ifA",
					ParentNodeID: "ghost",
					Medium:       core.MediumWired,
				}); err != nil {
					t.Fatalf("AddInterface(ghost/ifA) error: %v", err)
				}
				if err := state.NetworkKB().AddInterface(&core.NetworkInterface{
					ID:           "ghost/ifB",
					ParentNodeID: "ghost",
					Medium:       core.MediumWired,
				}); err != nil {
					t.Fatalf("AddInterface(ghost/ifB) error: %v", err)
				}
				return svc, state
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, state := tc.svc(t)

			_, err := svc.CreateLink(ctx, tc.req)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("CreateLink(%s) code = %v, want InvalidArgument (err=%v)",
					tc.name, status.Code(err), err)
			}
			if got := len(state.ListLinks()); got != 0 {
				t.Fatalf("ScenarioState should not store links on failure; found %d", got)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// CreateLink: wireless validation
// -----------------------------------------------------------------------------

func TestNetworkLinkServiceWirelessValidation(t *testing.T) {
	ctx := context.Background()
	svc, state := newLinkServiceForTest()

	trxID := "trx-wireless"
	addTransceiver(t, state, trxID)

	addNodeWithMedium(t, state, "node-wired", "eth0", core.MediumWired, "")
	addNodeWithMedium(t, state, "node-wireless-a", "rf0", core.MediumWireless, trxID)

	// Mixed wired/wireless endpoints should be rejected.
	mixedReq := &resources.BidirectionalLink{
		ANetworkNodeId: strPtr("node-wired"),
		ATxInterfaceId: strPtr("eth0"),
		ARxInterfaceId: strPtr("eth0"),
		BNetworkNodeId: strPtr("node-wireless-a"),
		BTxInterfaceId: strPtr("rf0"),
		BRxInterfaceId: strPtr("rf0"),
	}

	if _, err := svc.CreateLink(ctx, mixedReq); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CreateLink(mixed mediums) code = %v, want InvalidArgument (err=%v)",
			status.Code(err), err)
	}
	if got := len(state.ListLinks()); got != 0 {
		t.Fatalf("links stored after mixed-medium failure = %d, want 0", got)
	}

	// Wireless endpoints on both sides should succeed.
	addNodeWithMedium(t, state, "node-wireless-b", "rf1", core.MediumWireless, trxID)

	wirelessReq := &resources.BidirectionalLink{
		ANetworkNodeId: strPtr("node-wireless-a"),
		ATxInterfaceId: strPtr("rf0"),
		ARxInterfaceId: strPtr("rf0"),
		BNetworkNodeId: strPtr("node-wireless-b"),
		BTxInterfaceId: strPtr("rf1"),
		BRxInterfaceId: strPtr("rf1"),
	}

	if _, err := svc.CreateLink(ctx, wirelessReq); err != nil {
		t.Fatalf("CreateLink(wireless) error: %v", err)
	}
	links := state.ListLinks()
	if len(links) != 1 {
		t.Fatalf("wireless link creation stored %d links, want 1", len(links))
	}
	for _, l := range links {
		if l.Medium != core.MediumWireless {
			t.Fatalf("wireless link %+v Medium = %s, want %s", l, l.Medium, core.MediumWireless)
		}
	}
}

// -----------------------------------------------------------------------------
// Get/List/Delete round-trip
// -----------------------------------------------------------------------------

func TestNetworkLinkServiceGetListDelete(t *testing.T) {
	ctx := context.Background()
	svc, state := newLinkServiceForTest()

	nodeA := "node-crud-a"
	nodeB := "node-crud-b"
	addNodeWithMedium(t, state, nodeA, "eth0", core.MediumWired, "")
	addNodeWithMedium(t, state, nodeB, "eth0", core.MediumWired, "")

	createReq := &resources.BidirectionalLink{
		ANetworkNodeId: strPtr(nodeA),
		ATxInterfaceId: strPtr("eth0"),
		ARxInterfaceId: strPtr("eth0"),
		BNetworkNodeId: strPtr(nodeB),
		BTxInterfaceId: strPtr("eth0"),
		BRxInterfaceId: strPtr("eth0"),
	}

	if _, err := svc.CreateLink(ctx, createReq); err != nil {
		t.Fatalf("CreateLink seed error: %v", err)
	}
	links := state.ListLinks()
	if len(links) != 1 {
		t.Fatalf("after create, link count = %d, want 1", len(links))
	}
	firstID := links[0].ID

	getResp, err := svc.GetLink(ctx, &v1alpha.GetLinkRequest{LinkId: strPtr(firstID)})
	if err != nil {
		t.Fatalf("GetLink error: %v", err)
	}
	nodes := map[string]struct{}{
		getResp.GetANetworkNodeId(): {},
		getResp.GetBNetworkNodeId(): {},
	}
	if _, ok := nodes[nodeA]; !ok {
		t.Fatalf("GetLink response missing node %s: %+v", nodeA, getResp)
	}
	if _, ok := nodes[nodeB]; !ok {
		t.Fatalf("GetLink response missing node %s: %+v", nodeB, getResp)
	}

	listResp, err := svc.ListLinks(ctx, &v1alpha.ListLinksRequest{})
	if err != nil {
		t.Fatalf("ListLinks error: %v", err)
	}
	if len(listResp.GetLinks()) != 1 {
		t.Fatalf("ListLinks response count = %d, want 1 BidirectionalLink", len(listResp.GetLinks()))
	}

	if _, err := svc.DeleteLink(ctx, &v1alpha.DeleteLinkRequest{LinkId: strPtr(firstID)}); err != nil {
		t.Fatalf("DeleteLink(%s) error: %v", firstID, err)
	}
	if _, err := svc.GetLink(ctx, &v1alpha.GetLinkRequest{LinkId: strPtr(firstID)}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetLink deleted code = %v, want NotFound (err=%v)", status.Code(err), err)
	}
	if state.NetworkKB().GetNetworkLink(firstID) != nil {
		t.Fatalf("NetworkKB still holds deleted link %s", firstID)
	}
	if got := len(state.ListLinks()); got != 0 {
		t.Fatalf("link count after delete = %d, want 0", got)
	}

	finalList, err := svc.ListLinks(ctx, &v1alpha.ListLinksRequest{})
	if err != nil {
		t.Fatalf("ListLinks after deletes error: %v", err)
	}
	if len(finalList.GetLinks()) != 0 {
		t.Fatalf("ListLinks after deletes = %d, want 0", len(finalList.GetLinks()))
	}
	if got := len(state.ListLinks()); got != 0 {
		t.Fatalf("ScenarioState links after deletes = %d, want 0", got)
	}
}
