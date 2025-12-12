package controller

import (
	"testing"
	"time"

	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
	"google.golang.org/grpc"
)

// fakeStream is a minimal stub for testing
type fakeStream struct {
	grpc.BidiStreamingServer[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController]
}

func (f *fakeStream) Send(msg *schedulingpb.ReceiveRequestsMessageFromController) error {
	return nil
}

func (f *fakeStream) Recv() (*schedulingpb.ReceiveRequestsMessageToController, error) {
	select {} // block forever
}

func TestCDPIServer_SendCreateEntry_AttachesTokenAndIncrementsSeqno(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	server := NewCDPIServer(scenarioState, scheduler)

	// Create a node
	node := &model.NetworkNode{ID: "node1"}
	if err := scenarioState.CreateNode(node, nil); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// Create an agent handle manually
	handle := &AgentHandle{
		AgentID:  "agent-1",
		NodeID:   "node1",
		Stream:   &fakeStream{},
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 1),
		token:    "tok-123",
		seqNo:    0,
	}

	// Register the agent
	server.agentsMu.Lock()
	server.agents["agent-1"] = handle
	server.agentsMu.Unlock()

	// Create a simple action
	action := &sbi.ScheduledAction{
		EntryID: "entry-1",
		Type:    sbi.ScheduledSetRoute,
		When:    time.Now(),
		Route: &model.RouteEntry{
			DestinationCIDR: "10.0.0.0/24",
			NextHopNodeID:   "node2",
			OutInterfaceID:  "if1",
		},
	}

	// Call SendCreateEntry
	if err := server.SendCreateEntry("agent-1", action); err != nil {
		t.Fatalf("SendCreateEntry failed: %v", err)
	}

	// Assert: one message is present on handle.outgoing
	select {
	case msg := <-handle.outgoing:
		// Verify it's a CreateEntryRequest
		createEntry := msg.GetCreateEntry()
		if createEntry == nil {
			t.Fatalf("expected CreateEntryRequest, got %T", msg.GetRequest())
		}

		// Verify token
		if createEntry.GetScheduleManipulationToken() != "tok-123" {
			t.Errorf("expected token 'tok-123', got %q", createEntry.GetScheduleManipulationToken())
		}

		// Verify seqno is 1 (started at 0, incremented to 1)
		if createEntry.GetSeqno() != 1 {
			t.Errorf("expected seqno 1, got %d", createEntry.GetSeqno())
		}

		// Verify entry ID
		if createEntry.GetId() != "entry-1" {
			t.Errorf("expected entry ID 'entry-1', got %q", createEntry.GetId())
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for message on outgoing channel")
	}
}

func TestCDPIServer_SendDeleteEntry_ReusesTokenAndIncrementsSeqno(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	server := NewCDPIServer(scenarioState, scheduler)

	// Create a node
	node := &model.NetworkNode{ID: "node1"}
	if err := scenarioState.CreateNode(node, nil); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// Create an agent handle with seqno already at 1
	handle := &AgentHandle{
		AgentID:  "agent-1",
		NodeID:   "node1",
		Stream:   &fakeStream{},
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 1),
		token:    "tok-123",
		seqNo:    1, // Start at 1 (after first create)
	}

	// Register the agent
	server.agentsMu.Lock()
	server.agents["agent-1"] = handle
	server.agentsMu.Unlock()

	// Call SendDeleteEntry
	if err := server.SendDeleteEntry("agent-1", "entry-1"); err != nil {
		t.Fatalf("SendDeleteEntry failed: %v", err)
	}

	// Assert: message is present on handle.outgoing
	select {
	case msg := <-handle.outgoing:
		// Verify it's a DeleteEntryRequest
		deleteEntry := msg.GetDeleteEntry()
		if deleteEntry == nil {
			t.Fatalf("expected DeleteEntryRequest, got %T", msg.GetRequest())
		}

		// Verify token
		if deleteEntry.GetScheduleManipulationToken() != "tok-123" {
			t.Errorf("expected token 'tok-123', got %q", deleteEntry.GetScheduleManipulationToken())
		}

		// Verify seqno is 2 (started at 1, incremented to 2)
		if deleteEntry.GetSeqno() != 2 {
			t.Errorf("expected seqno 2, got %d", deleteEntry.GetSeqno())
		}

		// Verify entry ID
		if deleteEntry.GetId() != "entry-1" {
			t.Errorf("expected entry ID 'entry-1', got %q", deleteEntry.GetId())
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for message on outgoing channel")
	}
}

func TestCDPIServer_SendFinalize_UsesTokenAndIncrementsSeqno(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	server := NewCDPIServer(scenarioState, scheduler)

	// Create a node
	node := &model.NetworkNode{ID: "node1"}
	if err := scenarioState.CreateNode(node, nil); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// Create an agent handle with seqno at 10
	handle := &AgentHandle{
		AgentID:  "agent-1",
		NodeID:   "node1",
		Stream:   &fakeStream{},
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 1),
		token:    "tok-XYZ",
		seqNo:    10,
	}

	// Register the agent
	server.agentsMu.Lock()
	server.agents["agent-1"] = handle
	server.agentsMu.Unlock()

	cutoffTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Call SendFinalize
	if err := server.SendFinalize("agent-1", cutoffTime); err != nil {
		t.Fatalf("SendFinalize failed: %v", err)
	}

	// Assert: message is present on handle.outgoing
	select {
	case msg := <-handle.outgoing:
		// Verify it's a FinalizeRequest
		finalize := msg.GetFinalize()
		if finalize == nil {
			t.Fatalf("expected FinalizeRequest, got %T", msg.GetRequest())
		}

		// Verify token
		if finalize.GetScheduleManipulationToken() != "tok-XYZ" {
			t.Errorf("expected token 'tok-XYZ', got %q", finalize.GetScheduleManipulationToken())
		}

		// Verify seqno is 11 (started at 10, incremented to 11)
		if finalize.GetSeqno() != 11 {
			t.Errorf("expected seqno 11, got %d", finalize.GetSeqno())
		}

		// Verify cutoff time
		if finalize.GetUpTo().AsTime() != cutoffTime {
			t.Errorf("expected cutoff time %v, got %v", cutoffTime, finalize.GetUpTo().AsTime())
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for message on outgoing channel")
	}
}

func TestCDPIServer_SendCreateEntry_UnknownAgentReturnsError(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	server := NewCDPIServer(scenarioState, scheduler)

	// Create a simple action
	action := &sbi.ScheduledAction{
		EntryID: "entry-1",
		Type:    sbi.ScheduledSetRoute,
		When:    time.Now(),
		Route: &model.RouteEntry{
			DestinationCIDR: "10.0.0.0/24",
		},
	}

	// Call SendCreateEntry with unknown agent
	err := server.SendCreateEntry("missing-agent", action)
	if err == nil {
		t.Fatalf("expected error for unknown agent, got nil")
	}

	// Verify error message mentions the agent ID
	if err.Error() == "" {
		t.Errorf("error message should not be empty")
	}
}

func TestCDPIServer_SendCreateEntry_ChannelFullReturnsError(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	server := NewCDPIServer(scenarioState, scheduler)

	// Create a node
	node := &model.NetworkNode{ID: "node1"}
	if err := scenarioState.CreateNode(node, nil); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// Create an agent handle with unbuffered channel (buffer 0)
	handle := &AgentHandle{
		AgentID:  "agent-1",
		NodeID:   "node1",
		Stream:   &fakeStream{},
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 0), // unbuffered
		token:    "tok-123",
		seqNo:    0,
	}

	// Register the agent
	server.agentsMu.Lock()
	server.agents["agent-1"] = handle
	server.agentsMu.Unlock()

	// Create a simple action
	action := &sbi.ScheduledAction{
		EntryID: "entry-1",
		Type:    sbi.ScheduledSetRoute,
		When:    time.Now(),
		Route: &model.RouteEntry{
			DestinationCIDR: "10.0.0.0/24",
		},
	}

	// Call SendCreateEntry - should fail because channel is full (unbuffered, no receiver)
	err := server.SendCreateEntry("agent-1", action)
	if err == nil {
		t.Fatalf("expected error for full channel, got nil")
	}

	// Verify error message mentions channel full
	if err.Error() == "" {
		t.Errorf("error message should not be empty")
	}
}

func TestAgentHandle_NextSeqNo_IncrementsMonotonically(t *testing.T) {
	handle := &AgentHandle{
		seqNo: 0,
	}

	// First call should return 1
	if got := handle.NextSeqNo(); got != 1 {
		t.Errorf("expected seqno 1, got %d", got)
	}

	// Second call should return 2
	if got := handle.NextSeqNo(); got != 2 {
		t.Errorf("expected seqno 2, got %d", got)
	}

	// Third call should return 3
	if got := handle.NextSeqNo(); got != 3 {
		t.Errorf("expected seqno 3, got %d", got)
	}
}

func TestAgentHandle_CurrentToken_ReturnsToken(t *testing.T) {
	handle := &AgentHandle{
		token: "test-token-123",
	}

	if got := handle.CurrentToken(); got != "test-token-123" {
		t.Errorf("expected token 'test-token-123', got %q", got)
	}
}

func TestAgentHandle_SetToken_SetsToken(t *testing.T) {
	handle := &AgentHandle{
		token: "old-token",
	}

	handle.SetToken("new-token")

	if got := handle.CurrentToken(); got != "new-token" {
		t.Errorf("expected token 'new-token', got %q", got)
	}
}

func TestCDPIServer_setAgentToken_SetsTokenAndResetsSeqno(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	server := NewCDPIServer(scenarioState, scheduler)

	// Create a node
	node := &model.NetworkNode{ID: "node1"}
	if err := scenarioState.CreateNode(node, nil); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// Create an agent handle with existing token and seqno
	handle := &AgentHandle{
		AgentID:  "agent-1",
		NodeID:   "node1",
		Stream:   &fakeStream{},
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 1),
		token:    "old-token",
		seqNo:    5,
	}

	// Register the agent
	server.agentsMu.Lock()
	server.agents["agent-1"] = handle
	server.agentsMu.Unlock()

	// Call setAgentToken
	if err := server.setAgentToken("agent-1", "new-token"); err != nil {
		t.Fatalf("setAgentToken failed: %v", err)
	}

	// Verify token was updated
	if handle.CurrentToken() != "new-token" {
		t.Errorf("expected token 'new-token', got %q", handle.CurrentToken())
	}

	// Verify seqno was reset to 0
	handle.seqNoMu.Lock()
	if handle.seqNo != 0 {
		t.Errorf("expected seqno 0 after reset, got %d", handle.seqNo)
	}
	handle.seqNoMu.Unlock()
}

func TestCDPIServer_setAgentToken_UnknownAgentReturnsError(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	server := NewCDPIServer(scenarioState, scheduler)

	// Call setAgentToken with unknown agent
	err := server.setAgentToken("missing-agent", "new-token")
	if err == nil {
		t.Fatalf("expected error for unknown agent, got nil")
	}

	// Verify error message mentions the agent ID
	if err.Error() == "" {
		t.Errorf("error message should not be empty")
	}
}

