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
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeStreamForFinalizeTest is a minimal stub for testing
type fakeStreamForFinalizeTest struct {
	grpc.BidiStreamingClient[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController]
}

func (f *fakeStreamForFinalizeTest) Send(msg *schedulingpb.ReceiveRequestsMessageToController) error {
	return nil
}

func (f *fakeStreamForFinalizeTest) Recv() (*schedulingpb.ReceiveRequestsMessageFromController, error) {
	select {} // block forever
}

// fakeTelemetryClientForFinalizeTest is a minimal stub for testing
type fakeTelemetryClientForFinalizeTest struct {
	telemetrypb.TelemetryClient
}

func TestSimAgent_HandleFinalize_PrunePastEntriesKeepFuture(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	now := time.Unix(1000, 0)
	scheduler := sbi.NewFakeEventScheduler(now)
	telemetryCli := &fakeTelemetryClientForFinalizeTest{}
	stream := &fakeStreamForFinalizeTest{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	ctx := context.Background()
	agent.ctx = ctx

	// Create actions: one in the past, one in the future
	pastAction := &sbi.ScheduledAction{
		EntryID:   "entry-1",
		AgentID:   "agent-1",
		Type:      sbi.ScheduledSetRoute,
		When:      now.Add(-10 * time.Second),
		Route:     &model.RouteEntry{DestinationCIDR: "10.0.0.0/24"},
		RequestID: "req-1",
		SeqNo:     1,
		Token:     "tok-1",
	}

	futureAction := &sbi.ScheduledAction{
		EntryID:   "entry-2",
		AgentID:   "agent-1",
		Type:      sbi.ScheduledSetRoute,
		When:      now.Add(10 * time.Second),
		Route:     &model.RouteEntry{DestinationCIDR: "10.0.1.0/24"},
		RequestID: "req-2",
		SeqNo:     2,
		Token:     "tok-1",
	}

	// Schedule both actions
	if err := agent.HandleScheduledAction(ctx, pastAction); err != nil {
		t.Fatalf("HandleScheduledAction failed for pastAction: %v", err)
	}
	if err := agent.HandleScheduledAction(ctx, futureAction); err != nil {
		t.Fatalf("HandleScheduledAction failed for futureAction: %v", err)
	}

	// Verify both are in pending
	agent.mu.Lock()
	if len(agent.pending) != 2 {
		t.Fatalf("expected 2 pending actions, got %d", len(agent.pending))
	}
	agent.mu.Unlock()

	// Create FinalizeRequest with cutoff at now
	req := &schedulingpb.FinalizeRequest{
		ScheduleManipulationToken: "tok-1",
		Seqno:                     3,
		UpTo:                      timestamppb.New(now),
	}

	// Handle FinalizeRequest
	if err := agent.handleFinalize(0, req); err != nil {
		t.Fatalf("handleFinalize failed: %v", err)
	}

	// Verify past entry was removed, future entry remains
	agent.mu.Lock()
	if len(agent.pending) != 1 {
		t.Fatalf("expected 1 pending action after finalize, got %d", len(agent.pending))
	}
	if _, exists := agent.pending["entry-1"]; exists {
		t.Errorf("expected entry-1 to be pruned, but it still exists")
	}
	if _, exists := agent.pending["entry-2"]; !exists {
		t.Errorf("expected entry-2 to remain, but it was pruned")
	}
	agent.mu.Unlock()
}

func TestSimAgent_HandleFinalize_CutoffBeforeAllEntriesNoPruning(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	now := time.Unix(1000, 0)
	scheduler := sbi.NewFakeEventScheduler(now)
	telemetryCli := &fakeTelemetryClientForFinalizeTest{}
	stream := &fakeStreamForFinalizeTest{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	ctx := context.Background()
	agent.ctx = ctx

	// Create actions all in the future
	action1 := &sbi.ScheduledAction{
		EntryID:   "entry-1",
		AgentID:   "agent-1",
		Type:      sbi.ScheduledSetRoute,
		When:      now.Add(10 * time.Second),
		Route:     &model.RouteEntry{DestinationCIDR: "10.0.0.0/24"},
		RequestID: "req-1",
		SeqNo:     1,
		Token:     "tok-1",
	}

	action2 := &sbi.ScheduledAction{
		EntryID:   "entry-2",
		AgentID:   "agent-1",
		Type:      sbi.ScheduledSetRoute,
		When:      now.Add(20 * time.Second),
		Route:     &model.RouteEntry{DestinationCIDR: "10.0.1.0/24"},
		RequestID: "req-2",
		SeqNo:     2,
		Token:     "tok-1",
	}

	// Schedule both actions
	if err := agent.HandleScheduledAction(ctx, action1); err != nil {
		t.Fatalf("HandleScheduledAction failed: %v", err)
	}
	if err := agent.HandleScheduledAction(ctx, action2); err != nil {
		t.Fatalf("HandleScheduledAction failed: %v", err)
	}

	// Create FinalizeRequest with cutoff before all entries
	cutoff := now.Add(-5 * time.Second)
	req := &schedulingpb.FinalizeRequest{
		ScheduleManipulationToken: "tok-1",
		Seqno:                     3,
		UpTo:                      timestamppb.New(cutoff),
	}

	// Handle FinalizeRequest
	if err := agent.handleFinalize(0, req); err != nil {
		t.Fatalf("handleFinalize failed: %v", err)
	}

	// Verify all entries remain
	agent.mu.Lock()
	if len(agent.pending) != 2 {
		t.Fatalf("expected 2 pending actions after finalize, got %d", len(agent.pending))
	}
	agent.mu.Unlock()
}

func TestSimAgent_HandleFinalize_CutoffAfterAllEntriesAllPruned(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	now := time.Unix(1000, 0)
	scheduler := sbi.NewFakeEventScheduler(now)
	telemetryCli := &fakeTelemetryClientForFinalizeTest{}
	stream := &fakeStreamForFinalizeTest{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	ctx := context.Background()
	agent.ctx = ctx

	// Create actions all in the past
	action1 := &sbi.ScheduledAction{
		EntryID:   "entry-1",
		AgentID:   "agent-1",
		Type:      sbi.ScheduledSetRoute,
		When:      now.Add(-20 * time.Second),
		Route:     &model.RouteEntry{DestinationCIDR: "10.0.0.0/24"},
		RequestID: "req-1",
		SeqNo:     1,
		Token:     "tok-1",
	}

	action2 := &sbi.ScheduledAction{
		EntryID:   "entry-2",
		AgentID:   "agent-1",
		Type:      sbi.ScheduledSetRoute,
		When:      now.Add(-10 * time.Second),
		Route:     &model.RouteEntry{DestinationCIDR: "10.0.1.0/24"},
		RequestID: "req-2",
		SeqNo:     2,
		Token:     "tok-1",
	}

	// Schedule both actions
	if err := agent.HandleScheduledAction(ctx, action1); err != nil {
		t.Fatalf("HandleScheduledAction failed: %v", err)
	}
	if err := agent.HandleScheduledAction(ctx, action2); err != nil {
		t.Fatalf("HandleScheduledAction failed: %v", err)
	}

	// Create FinalizeRequest with cutoff after all entries
	cutoff := now.Add(5 * time.Second)
	req := &schedulingpb.FinalizeRequest{
		ScheduleManipulationToken: "tok-1",
		Seqno:                     3,
		UpTo:                      timestamppb.New(cutoff),
	}

	// Handle FinalizeRequest
	if err := agent.handleFinalize(0, req); err != nil {
		t.Fatalf("handleFinalize failed: %v", err)
	}

	// Verify all entries were pruned
	agent.mu.Lock()
	if len(agent.pending) != 0 {
		t.Fatalf("expected 0 pending actions after finalize, got %d", len(agent.pending))
	}
	agent.mu.Unlock()
}
