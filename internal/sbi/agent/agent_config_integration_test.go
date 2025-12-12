package agent

import (
	"context"
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestSimAgent_Start_TelemetryInterval_Zero(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClient{}
	stream := &fakeStream{}

	// Create agent with telemetry disabled
	agent := NewSimAgentWithConfig(
		"agent-1",
		"node1",
		scenarioState,
		scheduler,
		telemetryCli,
		stream,
		TelemetryConfig{Enabled: false},
	)

	ctx := context.Background()
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer agent.Stop()

	// Advance time - telemetry should not be scheduled
	scheduler.AdvanceTo(time.Now().Add(5 * time.Second))
	time.Sleep(50 * time.Millisecond)

	// Verify no telemetry was scheduled (we can't easily check this without
	// exposing internal state, but at least it shouldn't crash)
}

func TestSimAgent_Start_TelemetryInterval_Positive(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	
	// Add a node and interface
	node := &model.NetworkNode{ID: "node1"}
	if err := scenarioState.CreateNode(node, []*core.NetworkInterface{
		{ID: "if1", ParentNodeID: "node1", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	scheduler := sbi.NewFakeEventScheduler(startTime)
	telemetryCli := &fakeTelemetryClientForTesting{}
	stream := &fakeStreamForTelemetry{}

	// Create agent with telemetry enabled and custom interval
	agent := NewSimAgentWithConfig(
		"agent-1",
		"node1",
		scenarioState,
		scheduler,
		telemetryCli,
		stream,
		TelemetryConfig{
			Enabled:  true,
			Interval: 2 * time.Second,
		},
	)

	ctx := context.Background()
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer agent.Stop()

	// Advance time to trigger telemetry
	scheduler.AdvanceTo(startTime.Add(3 * time.Second))
	time.Sleep(100 * time.Millisecond)

	// Verify telemetry was called
	calls := telemetryCli.getCalls()
	if len(calls) == 0 {
		t.Fatalf("expected at least one ExportMetrics call when telemetry is enabled")
	}
}

func TestSimAgent_NewSimAgent_UsesDefaults(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClient{}
	stream := &fakeStream{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream)

	// Verify default interval is set
	if agent.telemetryInterval != 1*time.Second {
		t.Fatalf("expected default telemetryInterval=1s, got %v", agent.telemetryInterval)
	}
}

