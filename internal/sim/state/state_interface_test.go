package state

import (
	"errors"
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestDeleteInterfaceFailsWhenLinksPresent(t *testing.T) {
	s := NewScenarioState(kb.NewKnowledgeBase(), network.NewKnowledgeBase())

	nodeID := "node-iface"
	ifaceID := nodeID + "/if0"
	if err := s.CreateNode(&model.NetworkNode{ID: nodeID}, []*network.NetworkInterface{
		{ID: ifaceID, ParentNodeID: nodeID, Medium: network.MediumWired},
	}); err != nil {
		t.Fatalf("CreateNode error: %v", err)
	}

	if err := s.netKB.AddInterface(&network.NetworkInterface{
		ID:           "peer/if0",
		ParentNodeID: "peer",
		Medium:       network.MediumWired,
	}); err != nil {
		t.Fatalf("AddInterface(peer) error: %v", err)
	}
	if err := s.CreateLink(&network.NetworkLink{
		ID:         "link-iface",
		InterfaceA: ifaceID,
		InterfaceB: "peer/if0",
		Medium:     network.MediumWired,
	}); err != nil {
		t.Fatalf("CreateLink error: %v", err)
	}

	if err := s.DeleteInterface(ifaceID); !errors.Is(err, ErrInterfaceInUse) {
		t.Fatalf("DeleteInterface error = %v, want ErrInterfaceInUse", err)
	}

	if got := s.netKB.GetNetworkInterface(ifaceID); got == nil {
		t.Fatalf("interface should remain after failed delete; got nil")
	}
	if got, err := s.GetLink("link-iface"); err != nil || got == nil {
		t.Fatalf("link should remain after failed interface delete, got (%+v, %v)", got, err)
	}
}

func TestDeleteInterfaceRemovesInterface(t *testing.T) {
	s := NewScenarioState(kb.NewKnowledgeBase(), network.NewKnowledgeBase())

	nodeID := "node-iface-remove"
	ifaceID := nodeID + "/if0"
	if err := s.CreateNode(&model.NetworkNode{ID: nodeID}, []*network.NetworkInterface{
		{ID: ifaceID, ParentNodeID: nodeID, Medium: network.MediumWired},
	}); err != nil {
		t.Fatalf("CreateNode error: %v", err)
	}

	if err := s.DeleteInterface(ifaceID); err != nil {
		t.Fatalf("DeleteInterface error: %v", err)
	}
	if got := s.netKB.GetNetworkInterface(ifaceID); got != nil {
		t.Fatalf("interface should be removed after DeleteInterface; got %+v", got)
	}
}
