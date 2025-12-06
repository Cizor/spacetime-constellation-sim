package state

import (
	"errors"
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestDeleteNodeRemovesNodeAndInterfaces(t *testing.T) {
	s := NewScenarioState(kb.NewKnowledgeBase(), network.NewKnowledgeBase(), logging.Noop())

	nodeID := "node-delete"
	ifaceID := nodeID + "/if0"
	if err := s.CreateNode(&model.NetworkNode{ID: nodeID}, []*network.NetworkInterface{
		{ID: ifaceID, ParentNodeID: nodeID, Medium: network.MediumWired},
	}); err != nil {
		t.Fatalf("CreateNode error: %v", err)
	}

	if err := s.DeleteNode(nodeID); err != nil {
		t.Fatalf("DeleteNode error: %v", err)
	}

	if _, _, err := s.GetNode(nodeID); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("GetNode after delete error = %v, want ErrNodeNotFound", err)
	}
	if got := s.physKB.GetNetworkNode(nodeID); got != nil {
		t.Fatalf("physKB.GetNetworkNode(%q) = %+v, want nil", nodeID, got)
	}
	if got := s.netKB.GetNetworkInterface(ifaceID); got != nil {
		t.Fatalf("netKB.GetNetworkInterface(%q) = %+v, want nil", ifaceID, got)
	}
}

func TestDeleteNodeFailsWhenLinksPresent(t *testing.T) {
	s := NewScenarioState(kb.NewKnowledgeBase(), network.NewKnowledgeBase(), logging.Noop())

	nodeID := "node-in-use"
	ifaceID := nodeID + "/if-link"
	if err := s.CreateNode(&model.NetworkNode{ID: nodeID}, []*network.NetworkInterface{
		{ID: ifaceID, ParentNodeID: nodeID, Medium: network.MediumWired},
	}); err != nil {
		t.Fatalf("CreateNode error: %v", err)
	}

	// Peer interface needed for link creation.
	if err := s.netKB.AddInterface(&network.NetworkInterface{
		ID:           "peer/if0",
		ParentNodeID: "peer",
		Medium:       network.MediumWired,
	}); err != nil {
		t.Fatalf("AddInterface(peer) error: %v", err)
	}
	if err := s.CreateLink(&network.NetworkLink{
		ID:         "link-node-peer",
		InterfaceA: ifaceID,
		InterfaceB: "peer/if0",
		Medium:     network.MediumWired,
	}); err != nil {
		t.Fatalf("CreateLink error: %v", err)
	}

	if err := s.DeleteNode(nodeID); !errors.Is(err, ErrNodeInUse) {
		t.Fatalf("DeleteNode error = %v, want ErrNodeInUse", err)
	}

	if got := s.physKB.GetNetworkNode(nodeID); got == nil {
		t.Fatalf("node should remain after failed delete; got nil")
	}
	if got := s.netKB.GetNetworkInterface(ifaceID); got == nil {
		t.Fatalf("interface should remain after failed delete; got nil")
	}
	if got, err := s.GetLink("link-node-peer"); err != nil || got == nil {
		t.Fatalf("link should remain after failed delete, got (%+v, %v)", got, err)
	}
}

func TestDeleteNodeFailsWhenServiceRequestsPresent(t *testing.T) {
	s := NewScenarioState(kb.NewKnowledgeBase(), network.NewKnowledgeBase(), logging.Noop())

	nodeID := "node-with-sr"
	ifaceID := nodeID + "/if-sr"
	if err := s.CreateNode(&model.NetworkNode{ID: nodeID}, []*network.NetworkInterface{
		{ID: ifaceID, ParentNodeID: nodeID, Medium: network.MediumWired},
	}); err != nil {
		t.Fatalf("CreateNode error: %v", err)
	}

	if err := s.CreateServiceRequest(&model.ServiceRequest{
		ID:        "sr-keep-node",
		SrcNodeID: nodeID,
		DstNodeID: "other",
	}); err != nil {
		t.Fatalf("CreateServiceRequest error: %v", err)
	}

	if err := s.DeleteNode(nodeID); !errors.Is(err, ErrNodeInUse) {
		t.Fatalf("DeleteNode error = %v, want ErrNodeInUse", err)
	}

	if got := s.physKB.GetNetworkNode(nodeID); got == nil {
		t.Fatalf("node should remain after failed delete; got nil")
	}
	if got := s.netKB.GetNetworkInterface(ifaceID); got == nil {
		t.Fatalf("interface should remain after failed delete; got nil")
	}
}
