package agent

import (
	"context"
	"testing"
	"time"

	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
	"google.golang.org/grpc"
)

func TestSimAgent_ID(t *testing.T) {
	id := sbi.AgentID("test-agent-1")
	// Create minimal dependencies for test
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClient{}
	stream := &fakeStream{}

	agent := NewSimAgent(id, "node1", scenarioState, scheduler, telemetryCli, stream)

	if got := agent.ID(); got != id {
		t.Fatalf("ID() = %q, want %q", got, id)
	}
}

func TestSimAgent_HandleScheduledAction(t *testing.T) {
	id := sbi.AgentID("test-agent-1")
	// Create minimal dependencies for test
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClient{}
	stream := &fakeStream{}

	agent := NewSimAgent(id, "node1", scenarioState, scheduler, telemetryCli, stream)

	route := &model.RouteEntry{
		DestinationCIDR: "10.0.0.0/24",
		NextHopNodeID:   "nodeB",
		OutInterfaceID:  "if1",
	}
	meta := sbi.ActionMeta{
		RequestID: "req-1",
		SeqNo:     1,
		Token:     "token-abc",
	}
	action := sbi.NewRouteAction("action-1", id, sbi.ScheduledSetRoute, time.Now(), route, meta)

	// Should not panic and return nil
	if err := agent.HandleScheduledAction(context.Background(), action); err != nil {
		t.Fatalf("HandleScheduledAction returned error: %v", err)
	}
}

// fakeTelemetryClient is a minimal stub for testing
type fakeTelemetryClient struct {
	telemetrypb.TelemetryClient
}

// fakeStream is a minimal stub for testing
type fakeStream struct {
	grpc.BidiStreamingClient[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController]
}
