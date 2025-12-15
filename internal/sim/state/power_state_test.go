package state

import (
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/kb"
)

func setupPowerState(t *testing.T) *ScenarioState {
	t.Helper()
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	trx := &network.TransceiverModel{
		ID:            "trx",
		MaxPowerWatts: 10,
	}
	if err := net.AddTransceiverModel(trx); err != nil {
		t.Fatalf("AddTransceiverModel failed: %v", err)
	}
	iface := &network.NetworkInterface{
		ID:            "if0",
		ParentNodeID:  "node-A",
		TransceiverID: trx.ID,
	}
	if err := net.AddInterface(iface); err != nil {
		t.Fatalf("AddInterface failed: %v", err)
	}
	return NewScenarioState(phys, net, logging.Noop())
}

func TestInterfacePowerAllocation(t *testing.T) {
	state := setupPowerState(t)

	if err := state.AllocatePower("if0", "entry1", 3); err != nil {
		t.Fatalf("AllocatePower failed: %v", err)
	}
	if avail := state.GetAvailablePower("if0"); avail != 7 {
		t.Fatalf("expected 7W available, got %v", avail)
	}

	err := state.AllocatePower("if0", "entry2", 8)
	if err == nil {
		t.Fatalf("expected error when over-allocating power")
	}

	state.ReleasePower("if0", "entry1")
	if avail := state.GetAvailablePower("if0"); avail != 10 {
		t.Fatalf("expected full power available after release, got %v", avail)
	}
}
