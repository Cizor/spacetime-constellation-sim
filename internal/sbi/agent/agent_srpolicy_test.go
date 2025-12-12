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
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeStreamForSrPolicyTest is a minimal stub for testing
type fakeStreamForSrPolicyTest struct {
	grpc.BidiStreamingClient[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController]
}

func (f *fakeStreamForSrPolicyTest) Send(msg *schedulingpb.ReceiveRequestsMessageToController) error {
	return nil
}

func (f *fakeStreamForSrPolicyTest) Recv() (*schedulingpb.ReceiveRequestsMessageFromController, error) {
	select {} // block forever
}

// fakeTelemetryClientForSrPolicyTest is a minimal stub for testing
type fakeTelemetryClientForSrPolicyTest struct {
	telemetrypb.TelemetryClient
}

func TestSimAgent_SetSrPolicyStoresPolicy(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClientForSrPolicyTest{}
	stream := &fakeStreamForSrPolicyTest{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	ctx := context.Background()
	agent.ctx = ctx

	// Create a CreateEntryRequest with SetSrPolicy
	req := &schedulingpb.CreateEntryRequest{
		ScheduleManipulationToken: "tok-1",
		Seqno:                     1,
		Id:                        "entry-1",
		Time:                      timestamppb.Now(),
		ConfigurationChange: &schedulingpb.CreateEntryRequest_SetSrPolicy{
			SetSrPolicy: &schedulingpb.SetSrPolicy{
				Id: "pol-1",
			},
		},
	}

	// Process the message
	if err := agent.handleCreateEntry(1, req); err != nil {
		t.Fatalf("handleCreateEntry failed: %v", err)
	}

	// Verify policy was stored
	policies := agent.DumpSrPolicies()
	if len(policies) != 1 {
		t.Fatalf("expected 1 SR policy, got %d", len(policies))
	}
	if policies[0].PolicyID != "pol-1" {
		t.Errorf("expected policy ID 'pol-1', got %q", policies[0].PolicyID)
	}
}

func TestSimAgent_DeleteSrPolicyRemovesPolicy(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClientForSrPolicyTest{}
	stream := &fakeStreamForSrPolicyTest{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	ctx := context.Background()
	agent.ctx = ctx

	// First, add a policy
	req1 := &schedulingpb.CreateEntryRequest{
		ScheduleManipulationToken: "tok-1",
		Seqno:                     1,
		Id:                        "entry-1",
		Time:                      timestamppb.Now(),
		ConfigurationChange: &schedulingpb.CreateEntryRequest_SetSrPolicy{
			SetSrPolicy: &schedulingpb.SetSrPolicy{
				Id: "pol-1",
			},
		},
	}

	if err := agent.handleCreateEntry(1, req1); err != nil {
		t.Fatalf("handleCreateEntry failed: %v", err)
	}

	// Verify policy exists
	policies := agent.DumpSrPolicies()
	if len(policies) != 1 {
		t.Fatalf("expected 1 SR policy before delete, got %d", len(policies))
	}

	// Now delete it
	req2 := &schedulingpb.CreateEntryRequest{
		ScheduleManipulationToken: "tok-1",
		Seqno:                     2,
		Id:                        "entry-2",
		Time:                      timestamppb.Now(),
		ConfigurationChange: &schedulingpb.CreateEntryRequest_DeleteSrPolicy{
			DeleteSrPolicy: &schedulingpb.DeleteSrPolicy{
				Id: "pol-1",
			},
		},
	}

	if err := agent.handleCreateEntry(2, req2); err != nil {
		t.Fatalf("handleCreateEntry failed: %v", err)
	}

	// Verify policy was removed
	policies = agent.DumpSrPolicies()
	if len(policies) != 0 {
		t.Errorf("expected 0 SR policies after delete, got %d", len(policies))
	}
}

func TestSimAgent_DeleteSrPolicySafeForUnknownPolicy(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClientForSrPolicyTest{}
	stream := &fakeStreamForSrPolicyTest{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	ctx := context.Background()
	agent.ctx = ctx

	// Try to delete a non-existent policy
	req := &schedulingpb.CreateEntryRequest{
		ScheduleManipulationToken: "tok-1",
		Seqno:                     1,
		Id:                        "entry-1",
		Time:                      timestamppb.Now(),
		ConfigurationChange: &schedulingpb.CreateEntryRequest_DeleteSrPolicy{
			DeleteSrPolicy: &schedulingpb.DeleteSrPolicy{
				Id: "missing",
			},
		},
	}

	// Should not panic
	if err := agent.handleCreateEntry(1, req); err != nil {
		t.Fatalf("handleCreateEntry should not fail for unknown policy: %v", err)
	}

	// Verify policies map is still valid (empty)
	policies := agent.DumpSrPolicies()
	if len(policies) != 0 {
		t.Errorf("expected 0 SR policies, got %d", len(policies))
	}
}

func TestSimAgent_HandleSetSrPolicyNilSafe(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClientForSrPolicyTest{}
	stream := &fakeStreamForSrPolicyTest{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	// Should not panic
	agent.handleSetSrPolicy(nil)

	// Should not panic with empty PolicyID
	agent.handleSetSrPolicy(&sbi.SrPolicySpec{PolicyID: ""})

	// Verify policies map is still valid (empty)
	policies := agent.DumpSrPolicies()
	if len(policies) != 0 {
		t.Errorf("expected 0 SR policies, got %d", len(policies))
	}
}

func TestSimAgent_HandleDeleteSrPolicyNilSafe(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClientForSrPolicyTest{}
	stream := &fakeStreamForSrPolicyTest{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	// Should not panic
	agent.handleDeleteSrPolicy("")
	agent.handleDeleteSrPolicy("missing")

	// Verify policies map is still valid (empty)
	policies := agent.DumpSrPolicies()
	if len(policies) != 0 {
		t.Errorf("expected 0 SR policies, got %d", len(policies))
	}
}

func TestSimAgent_ResetClearsSrPolicies(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClientForSrPolicyTest{}
	stream := &fakeStreamForSrPolicyTest{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	ctx := context.Background()
	agent.ctx = ctx

	// Add a policy
	req := &schedulingpb.CreateEntryRequest{
		ScheduleManipulationToken: "tok-1",
		Seqno:                     1,
		Id:                        "entry-1",
		Time:                      timestamppb.Now(),
		ConfigurationChange: &schedulingpb.CreateEntryRequest_SetSrPolicy{
			SetSrPolicy: &schedulingpb.SetSrPolicy{
				Id: "pol-1",
			},
		},
	}

	if err := agent.handleCreateEntry(1, req); err != nil {
		t.Fatalf("handleCreateEntry failed: %v", err)
	}

	// Verify policy exists
	policies := agent.DumpSrPolicies()
	if len(policies) != 1 {
		t.Fatalf("expected 1 SR policy before reset, got %d", len(policies))
	}

	// Call Reset
	agent.Reset()

	// Verify policies are cleared
	policies = agent.DumpSrPolicies()
	if len(policies) != 0 {
		t.Errorf("expected 0 SR policies after reset, got %d", len(policies))
	}
}

