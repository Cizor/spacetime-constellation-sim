package state

import (
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestBandwidthReservationAndRelease(t *testing.T) {
	state := NewScenarioState(kb.NewKnowledgeBase(), network.NewKnowledgeBase(), logging.Noop())

	nodeA := &model.NetworkNode{ID: "nodeA"}
	nodeB := &model.NetworkNode{ID: "nodeB"}
	if err := state.CreateNode(nodeA, []*network.NetworkInterface{
		{ID: "ifA", ParentNodeID: "nodeA"},
	}); err != nil {
		t.Fatalf("CreateNode nodeA: %v", err)
	}
	if err := state.CreateNode(nodeB, []*network.NetworkInterface{
		{ID: "ifB", ParentNodeID: "nodeB"},
	}); err != nil {
		t.Fatalf("CreateNode nodeB: %v", err)
	}

	link := &network.NetworkLink{
		ID:                    "linkAB",
		InterfaceA:            "ifA",
		InterfaceB:            "ifB",
		Medium:                network.MediumWireless,
		MaxBandwidthBps:       1_000_000,
		AvailableBandwidthBps: 1_000_000,
	}
	if err := state.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	avail, err := state.GetAvailableBandwidth("linkAB")
	if err != nil {
		t.Fatalf("GetAvailableBandwidth failed: %v", err)
	}
	if avail != 1_000_000 {
		t.Fatalf("expected available 1_000_000, got %d", avail)
	}

	if err := state.ReserveBandwidth("linkAB", 300_000); err != nil {
		t.Fatalf("ReserveBandwidth failed: %v", err)
	}
	avail, _ = state.GetAvailableBandwidth("linkAB")
	if avail != 700_000 {
		t.Fatalf("expected available 700_000, got %d", avail)
	}

	if err := state.ReserveBandwidth("linkAB", 800_000); err == nil {
		t.Fatalf("expected insufficient bandwidth error")
	}
	avail, _ = state.GetAvailableBandwidth("linkAB")
	if avail != 700_000 {
		t.Fatalf("available changed after failed reservation: %d", avail)
	}

	if err := state.ReleaseBandwidth("linkAB", 200_000); err != nil {
		t.Fatalf("ReleaseBandwidth failed: %v", err)
	}
	avail, _ = state.GetAvailableBandwidth("linkAB")
	if avail != 900_000 {
		t.Fatalf("expected available 900_000, got %d", avail)
	}

	if err := state.ReleaseBandwidth("linkAB", 900_000); err == nil {
		t.Fatalf("expected release exceeds reserved bandwidth error")
	}
}
