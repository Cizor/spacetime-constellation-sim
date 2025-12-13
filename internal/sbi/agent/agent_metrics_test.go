package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestSimAgent_Metrics_ActionsExecuted(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)

	// Create a node with an interface
	node := &model.NetworkNode{ID: "node1"}
	if err := scenarioState.CreateNode(node, []*core.NetworkInterface{
		{ID: "if1", ParentNodeID: "node1", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	scheduler := sbi.NewFakeEventScheduler(startTime)
	telemetryCli := &fakeTelemetryClient{}
	stream := &fakeStream{}

	metrics := sbi.NewSBIMetrics()
	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, log)
	agent.Metrics = metrics
	agent.ctx = context.Background()

	// Execute an action
	action := &sbi.ScheduledAction{
		EntryID: "entry-1",
		When:    startTime,
		Type:    sbi.ScheduledSetRoute,
		Route: &model.RouteEntry{
			DestinationCIDR: "10.0.0.0/24",
			NextHopNodeID:  "node2",
			OutInterfaceID: "if1",
		},
	}

	agent.execute(action)

	// Verify metrics
	snap := metrics.Snapshot()
	if snap.NumActionsExecuted != 1 {
		t.Errorf("expected NumActionsExecuted=1, got %d", snap.NumActionsExecuted)
	}
}

func TestSimAgent_Metrics_MultipleActionsExecuted(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)

	// Create a node with an interface
	node := &model.NetworkNode{ID: "node1"}
	if err := scenarioState.CreateNode(node, []*core.NetworkInterface{
		{ID: "if1", ParentNodeID: "node1", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	scheduler := sbi.NewFakeEventScheduler(startTime)
	telemetryCli := &fakeTelemetryClient{}
	stream := &fakeStream{}

	metrics := sbi.NewSBIMetrics()
	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, log)
	agent.Metrics = metrics
	agent.ctx = context.Background()

	// Execute multiple actions
	for i := 0; i < 5; i++ {
		action := &sbi.ScheduledAction{
			EntryID: fmt.Sprintf("entry-%d", i),
			When:    startTime,
			Type:    sbi.ScheduledSetRoute,
			Route: &model.RouteEntry{
				DestinationCIDR: fmt.Sprintf("10.0.%d.0/24", i),
				NextHopNodeID:  "node2",
				OutInterfaceID: "if1",
			},
		}
		agent.execute(action)
	}

	// Verify metrics
	snap := metrics.Snapshot()
	if snap.NumActionsExecuted != 5 {
		t.Errorf("expected NumActionsExecuted=5, got %d", snap.NumActionsExecuted)
	}
}

