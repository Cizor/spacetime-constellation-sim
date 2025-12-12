package agent

import (
	"context"
	"fmt"
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

// fakeStreamForTokenTest is a minimal stub for testing
type fakeStreamForTokenTest struct {
	grpc.BidiStreamingClient[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController]
}

func (f *fakeStreamForTokenTest) Send(msg *schedulingpb.ReceiveRequestsMessageToController) error {
	return nil
}

func (f *fakeStreamForTokenTest) Recv() (*schedulingpb.ReceiveRequestsMessageFromController, error) {
	select {} // block forever
}

// fakeTelemetryClientForTokenTest is a minimal stub for testing
type fakeTelemetryClientForTokenTest struct {
	telemetrypb.TelemetryClient
}

func TestSimAgent_FirstMessageSetsToken(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClientForTokenTest{}
	stream := &fakeStreamForTokenTest{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	// Verify token starts empty
	if agent.GetToken() != "" {
		t.Fatalf("expected empty token initially, got %q", agent.GetToken())
	}

	// Create a CreateEntryRequest with token
	req := &schedulingpb.CreateEntryRequest{
		ScheduleManipulationToken: "tok-1",
		Seqno:                     1,
		Id:                        "entry-1",
		Time:                      timestamppb.Now(),
		ConfigurationChange: &schedulingpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedulingpb.SetRoute{
				To:  "10.0.0.0/24",
				Dev: "if1",
			},
		},
	}

	// Process the message
	ctx := context.Background()
	agent.ctx = ctx
	if err := agent.handleCreateEntry(1, req); err != nil {
		t.Fatalf("handleCreateEntry failed: %v", err)
	}

	// Verify token was established
	if agent.GetToken() != "tok-1" {
		t.Errorf("expected token 'tok-1', got %q", agent.GetToken())
	}

	// Verify entry was scheduled
	agent.mu.Lock()
	_, exists := agent.pending["entry-1"]
	agent.mu.Unlock()
	if !exists {
		t.Errorf("expected entry 'entry-1' to be scheduled")
	}
}

func TestSimAgent_MismatchedTokenIsRejected(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClientForTokenTest{}
	stream := &fakeStreamForTokenTest{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	// Establish initial token
	req1 := &schedulingpb.CreateEntryRequest{
		ScheduleManipulationToken: "tok-1",
		Seqno:                     1,
		Id:                        "entry-1",
		Time:                      timestamppb.Now(),
		ConfigurationChange: &schedulingpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedulingpb.SetRoute{
				To:  "10.0.0.0/24",
				Dev: "if1",
			},
		},
	}

	ctx := context.Background()
	agent.ctx = ctx
	if err := agent.handleCreateEntry(1, req1); err != nil {
		t.Fatalf("handleCreateEntry failed: %v", err)
	}

	// Verify token was established
	if agent.GetToken() != "tok-1" {
		t.Fatalf("expected token 'tok-1', got %q", agent.GetToken())
	}

	// Try to send a message with mismatched token
	req2 := &schedulingpb.CreateEntryRequest{
		ScheduleManipulationToken: "tok-2", // different token
		Seqno:                     2,
		Id:                        "entry-2",
		Time:                      timestamppb.Now(),
		ConfigurationChange: &schedulingpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedulingpb.SetRoute{
				To:  "10.0.1.0/24",
				Dev: "if2",
			},
		},
	}

	if err := agent.handleCreateEntry(2, req2); err != nil {
		t.Fatalf("handleCreateEntry should not return error for token mismatch (it should ignore silently)")
	}

	// Verify entry-2 was NOT scheduled
	agent.mu.Lock()
	_, exists := agent.pending["entry-2"]
	if exists {
		agent.mu.Unlock()
		t.Errorf("expected entry 'entry-2' to be rejected due to token mismatch")
		return
	}

	// Verify entry-1 still exists
	_, exists = agent.pending["entry-1"]
	agent.mu.Unlock()
	if !exists {
		t.Errorf("expected entry 'entry-1' to still exist")
	}
}

func TestSimAgent_SeqnoStrictlyIncreasingIsAccepted(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClientForTokenTest{}
	stream := &fakeStreamForTokenTest{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	ctx := context.Background()
	agent.ctx = ctx

	// Send messages with strictly increasing seqno
	token := "tok-1"
	for i := uint64(1); i <= 3; i++ {
		req := &schedulingpb.CreateEntryRequest{
			ScheduleManipulationToken: token,
			Seqno:                     i,
			Id:                        fmt.Sprintf("entry-%d", i),
			Time:                      timestamppb.Now(),
			ConfigurationChange: &schedulingpb.CreateEntryRequest_SetRoute{
				SetRoute: &schedulingpb.SetRoute{
					To:  "10.0.0.0/24",
					Dev: "if1",
				},
			},
		}

		if err := agent.handleCreateEntry(int64(i), req); err != nil {
			t.Fatalf("handleCreateEntry failed for seqno %d: %v", i, err)
		}
	}

	// Verify lastSeqNoSeen is 3
	agent.mu.Lock()
	if agent.lastSeqNoSeen != 3 {
		t.Errorf("expected lastSeqNoSeen 3, got %d", agent.lastSeqNoSeen)
	}
	agent.mu.Unlock()
}

func TestSimAgent_OutOfOrderSeqnoOnlyLogs(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClientForTokenTest{}
	stream := &fakeStreamForTokenTest{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	ctx := context.Background()
	agent.ctx = ctx

	token := "tok-1"

	// Send message with seqno 5
	req1 := &schedulingpb.CreateEntryRequest{
		ScheduleManipulationToken: token,
		Seqno:                     5,
		Id:                        "entry-1",
		Time:                      timestamppb.Now(),
		ConfigurationChange: &schedulingpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedulingpb.SetRoute{
				To:  "10.0.0.0/24",
				Dev: "if1",
			},
		},
	}

	if err := agent.handleCreateEntry(1, req1); err != nil {
		t.Fatalf("handleCreateEntry failed: %v", err)
	}

	// Verify lastSeqNoSeen is 5
	agent.mu.Lock()
	if agent.lastSeqNoSeen != 5 {
		t.Errorf("expected lastSeqNoSeen 5, got %d", agent.lastSeqNoSeen)
	}
	agent.mu.Unlock()

	// Send message with seqno 4 (out of order)
	req2 := &schedulingpb.CreateEntryRequest{
		ScheduleManipulationToken: token,
		Seqno:                     4, // out of order
		Id:                        "entry-2",
		Time:                      timestamppb.Now(),
		ConfigurationChange: &schedulingpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedulingpb.SetRoute{
				To:  "10.0.1.0/24",
				Dev: "if2",
			},
		},
	}

	// Should not error (only logs warning)
	if err := agent.handleCreateEntry(2, req2); err != nil {
		t.Fatalf("handleCreateEntry should not return error for out-of-order seqno")
	}

	// Verify entry-2 was still scheduled (we don't drop messages for seqno issues)
	agent.mu.Lock()
	_, exists := agent.pending["entry-2"]
	agent.mu.Unlock()
	if !exists {
		t.Errorf("expected entry 'entry-2' to be scheduled despite out-of-order seqno")
	}

	// Verify lastSeqNoSeen is now 4 (updated to the received value)
	agent.mu.Lock()
	if agent.lastSeqNoSeen != 4 {
		t.Errorf("expected lastSeqNoSeen 4 (updated from received value), got %d", agent.lastSeqNoSeen)
	}
	agent.mu.Unlock()
}

func TestSimAgent_ResetClearsTokenAndSeqno(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClientForTokenTest{}
	stream := &fakeStreamForTokenTest{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	ctx := context.Background()
	agent.ctx = ctx

	// Establish token and seqno
	req := &schedulingpb.CreateEntryRequest{
		ScheduleManipulationToken: "tok-1",
		Seqno:                     5,
		Id:                        "entry-1",
		Time:                      timestamppb.Now(),
		ConfigurationChange: &schedulingpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedulingpb.SetRoute{
				To:  "10.0.0.0/24",
				Dev: "if1",
			},
		},
	}

	if err := agent.handleCreateEntry(1, req); err != nil {
		t.Fatalf("handleCreateEntry failed: %v", err)
	}

	// Verify token and seqno are set
	agent.mu.Lock()
	if agent.token != "tok-1" {
		t.Errorf("expected token 'tok-1', got %q", agent.token)
	}
	if agent.lastSeqNoSeen != 5 {
		t.Errorf("expected lastSeqNoSeen 5, got %d", agent.lastSeqNoSeen)
	}
	if len(agent.pending) == 0 {
		t.Errorf("expected pending entries before reset")
	}
	agent.mu.Unlock()

	// Call Reset
	agent.Reset()

	// Verify token and seqno are cleared
	agent.mu.Lock()
	if agent.token != "" {
		t.Errorf("expected empty token after reset, got %q", agent.token)
	}
	if agent.lastSeqNoSeen != 0 {
		t.Errorf("expected lastSeqNoSeen 0 after reset, got %d", agent.lastSeqNoSeen)
	}
	if len(agent.pending) != 0 {
		t.Errorf("expected empty pending after reset, got %d entries", len(agent.pending))
	}
	agent.mu.Unlock()
}

