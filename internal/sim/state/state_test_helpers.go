package state

import (
	"errors"
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func newTestStateWithPlatform(t *testing.T) *ScenarioState {
	t.Helper()
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	state := NewScenarioState(phys, net, logging.Noop())
	platform := &model.PlatformDefinition{
		ID:          "plat-1",
		Coordinates: model.Motion{},
	}
	if err := state.CreatePlatform(platform); err != nil && !errors.Is(err, ErrPlatformExists) {
		t.Fatalf("CreatePlatform() error = %v", err)
	}
	return state
}

func ensureNode(t *testing.T, s *ScenarioState, nodeID string) {
	t.Helper()
	node := &model.NetworkNode{
		ID:         nodeID,
		PlatformID: "plat-1",
	}
	if err := s.CreateNode(node, nil); err != nil && !errors.Is(err, ErrNodeExists) {
		t.Fatalf("CreateNode(%q) error = %v", nodeID, err)
	}
}
