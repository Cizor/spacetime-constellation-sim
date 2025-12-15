package controller

import (
	"errors"
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestFindFederatedPath_CrossDomain(t *testing.T) {
	st := state.NewScenarioState(kb.NewKnowledgeBase(), network.NewKnowledgeBase(), logging.Noop())
	if err := st.CreatePlatform(&model.PlatformDefinition{ID: "plat-1"}); err != nil {
		t.Fatalf("CreatePlatform error: %v", err)
	}
	for _, nodeID := range []string{"node-left", "node-right"} {
		if err := st.CreateNode(&model.NetworkNode{ID: nodeID, PlatformID: "plat-1"}, nil); err != nil {
			t.Fatalf("CreateNode(%s) error: %v", nodeID, err)
		}
	}
	if err := st.CreateDomain(&model.SchedulingDomain{
		DomainID: "dom-left",
		Nodes:    []string{"node-left"},
	}); err != nil {
		t.Fatalf("CreateDomain(left) error = %v", err)
	}
	if err := st.CreateDomain(&model.SchedulingDomain{
		DomainID: "dom-right",
		Nodes:    []string{"node-right"},
	}); err != nil {
		t.Fatalf("CreateDomain(right) error = %v", err)
	}

	sr := &model.ServiceRequest{
		ID:              "sr-cross",
		SrcNodeID:       "node-left",
		DstNodeID:       "node-right",
		CrossDomain:     true,
		SourceDomain:    "dom-left",
		DestDomain:      "dom-right",
		FederationToken: "token",
	}
	path, err := FindFederatedPath(sr, st)
	if err != nil {
		t.Fatalf("FindFederatedPath error = %v", err)
	}
	if len(path.Segments) != 2 {
		t.Fatalf("Segments len = %d, want 2", len(path.Segments))
	}
	if path.DomainHops[0] != "dom-left" || path.DomainHops[1] != "dom-right" {
		t.Fatalf("DomainHops = %+v", path.DomainHops)
	}
}

func TestFindFederatedPath_InDomain(t *testing.T) {
	st := state.NewScenarioState(kb.NewKnowledgeBase(), network.NewKnowledgeBase(), logging.Noop())
	if err := st.CreatePlatform(&model.PlatformDefinition{ID: "plat-1"}); err != nil {
		t.Fatalf("CreatePlatform error: %v", err)
	}
	if err := st.CreateNode(&model.NetworkNode{ID: "node-single-federation", PlatformID: "plat-1"}, nil); err != nil && !errors.Is(err, state.ErrNodeExists) {
		t.Fatalf("CreateNode error: %v", err)
	}
	platform := &model.PlatformDefinition{ID: "plat"}
	if err := st.CreatePlatform(platform); err != nil {
		t.Fatalf("CreatePlatform error = %v", err)
	}
	if err := st.CreateNode(&model.NetworkNode{ID: "node-single", PlatformID: "plat"}, nil); err != nil {
		t.Fatalf("CreateNode error = %v", err)
	}
	if err := st.CreateDomain(&model.SchedulingDomain{
		DomainID: "dom-single",
		Nodes:    []string{"node-single-federation"},
	}); err != nil {
		t.Fatalf("CreateDomain error = %v", err)
	}
	sr := &model.ServiceRequest{
		ID:        "sr-local",
		SrcNodeID: "node-single-federation",
		DstNodeID: "node-single-federation",
	}
	path, err := FindFederatedPath(sr, st)
	if err != nil {
		t.Fatalf("FindFederatedPath error = %v", err)
	}
	if len(path.Segments) != 1 {
		t.Fatalf("Segments len = %d, want 1", len(path.Segments))
	}
}
