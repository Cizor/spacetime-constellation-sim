package state

import (
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func newScenarioWithNodes(t *testing.T, nodeIDs ...string) *ScenarioState {
	t.Helper()
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	for _, id := range nodeIDs {
		if err := phys.AddNetworkNode(&model.NetworkNode{ID: id}); err != nil {
			t.Fatalf("setup node %q: %v", id, err)
		}
	}
	return NewScenarioState(phys, net, logging.Noop())
}

func TestInstallSrPolicyStoresCopy(t *testing.T) {
	state := newScenarioWithNodes(t, "node-head", "ep1", "seg-node")
	policy := &model.SrPolicy{
		PolicyID:      "policy-1",
		Color:         5,
		HeadendNodeID: "node-head",
		Endpoints:     []string{"ep1"},
		Segments: []model.Segment{
			{SID: "sid-1", Type: "node", NodeID: "seg-node"},
		},
	}

	if err := state.InstallSrPolicy("node-head", policy); err != nil {
		t.Fatalf("InstallSrPolicy error: %v", err)
	}

	found := state.GetSrPolicies("node-head")
	if len(found) != 1 || found[0].PolicyID != "policy-1" {
		t.Fatalf("unexpected policies: %+v", found)
	}

	found[0].PolicyID = "mutated"
	again := state.GetSrPolicies("node-head")
	if again[0].PolicyID != "policy-1" {
		t.Fatalf("policy should not mutate original, got %v", again[0].PolicyID)
	}
}

func TestInstallSrPolicyDuplicateFails(t *testing.T) {
	state := newScenarioWithNodes(t, "node-head", "ep1", "seg-node")
	policy := &model.SrPolicy{
		PolicyID:      "policy-1",
		HeadendNodeID: "node-head",
		Endpoints:     []string{"ep1"},
		Segments: []model.Segment{
			{SID: "sid-1", Type: "node", NodeID: "seg-node"},
		},
	}

	if err := state.InstallSrPolicy("node-head", policy); err != nil {
		t.Fatalf("install error: %v", err)
	}
	if err := state.InstallSrPolicy("node-head", policy); err == nil {
		t.Fatalf("expected duplicate error")
	}
}

func TestInstallSrPolicyValidation(t *testing.T) {
	state := newScenarioWithNodes(t, "node-head", "ep1")
	policy := &model.SrPolicy{
		PolicyID:      "policy-1",
		HeadendNodeID: "node-head",
		Endpoints:     []string{"unknown"},
		Segments: []model.Segment{
			{SID: "sid-1", Type: "node", NodeID: "node-head"},
		},
	}
	if err := state.InstallSrPolicy("node-head", policy); err == nil {
		t.Fatalf("expected endpoint validation error")
	}
}

func TestRemoveSrPolicy(t *testing.T) {
	state := newScenarioWithNodes(t, "node-head", "ep1", "seg-node")
	policy := &model.SrPolicy{
		PolicyID:      "policy-1",
		HeadendNodeID: "node-head",
		Endpoints:     []string{"ep1"},
		Segments: []model.Segment{
			{SID: "sid-1", Type: "node", NodeID: "seg-node"},
		},
	}

	if err := state.InstallSrPolicy("node-head", policy); err != nil {
		t.Fatalf("install error: %v", err)
	}

	if err := state.RemoveSrPolicy("node-head", "policy-1"); err != nil {
		t.Fatalf("remove error: %v", err)
	}
	if err := state.RemoveSrPolicy("node-head", "policy-1"); err == nil {
		t.Fatalf("expected not found after removal")
	}
}
