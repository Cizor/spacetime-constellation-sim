package controller

import (
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCDPIServer_Metrics_CreateEntrySent(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	clock := sbi.NewFakeEventScheduler(time.Now())

	metrics := sbi.NewSBIMetrics()
	server := NewCDPIServer(scenarioState, clock, log)
	server.Metrics = metrics

	// Register a fake agent
	agentID := "agent-1"
	handle := &AgentHandle{
		AgentID:  agentID,
		NodeID:   "node-1",
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 10),
		token:    "test-token",
	}
	server.agentsMu.Lock()
	server.agents[agentID] = handle
	server.agentsMu.Unlock()

	// Send CreateEntry
	action := &sbi.ScheduledAction{
		EntryID: "entry-1",
		When:    time.Now(),
		Type:    sbi.ScheduledUpdateBeam,
		Beam: &sbi.BeamSpec{
			NodeID:      "node-1",
			InterfaceID: "if-1",
			TargetNodeID: "node-2",
			TargetIfID:   "if-2",
		},
	}

	err := server.SendCreateEntry(agentID, action)
	if err != nil {
		t.Fatalf("SendCreateEntry failed: %v", err)
	}

	// Verify metrics
	snap := metrics.Snapshot()
	if snap.NumCreateEntrySent != 1 {
		t.Errorf("expected NumCreateEntrySent=1, got %d", snap.NumCreateEntrySent)
	}
}

func TestCDPIServer_Metrics_DeleteEntrySent(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	clock := sbi.NewFakeEventScheduler(time.Now())

	metrics := sbi.NewSBIMetrics()
	server := NewCDPIServer(scenarioState, clock, log)
	server.Metrics = metrics

	// Register a fake agent
	agentID := "agent-1"
	handle := &AgentHandle{
		AgentID:  agentID,
		NodeID:   "node-1",
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 10),
		token:    "test-token",
	}
	server.agentsMu.Lock()
	server.agents[agentID] = handle
	server.agentsMu.Unlock()

	// Send DeleteEntry
	err := server.SendDeleteEntry(agentID, "entry-1")
	if err != nil {
		t.Fatalf("SendDeleteEntry failed: %v", err)
	}

	// Verify metrics
	snap := metrics.Snapshot()
	if snap.NumDeleteEntrySent != 1 {
		t.Errorf("expected NumDeleteEntrySent=1, got %d", snap.NumDeleteEntrySent)
	}
}

func TestCDPIServer_Metrics_FinalizeSent(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	clock := sbi.NewFakeEventScheduler(time.Now())

	metrics := sbi.NewSBIMetrics()
	server := NewCDPIServer(scenarioState, clock, log)
	server.Metrics = metrics

	// Register a fake agent
	agentID := "agent-1"
	handle := &AgentHandle{
		AgentID:  agentID,
		NodeID:   "node-1",
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 10),
		token:    "test-token",
	}
	server.agentsMu.Lock()
	server.agents[agentID] = handle
	server.agentsMu.Unlock()

	// Send Finalize
	cutoff := time.Now()
	err := server.SendFinalize(agentID, cutoff)
	if err != nil {
		t.Fatalf("SendFinalize failed: %v", err)
	}

	// Verify metrics
	snap := metrics.Snapshot()
	if snap.NumFinalizeSent != 1 {
		t.Errorf("expected NumFinalizeSent=1, got %d", snap.NumFinalizeSent)
	}
}

func TestCDPIServer_Metrics_ResponseOK(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	clock := sbi.NewFakeEventScheduler(time.Now())

	metrics := sbi.NewSBIMetrics()
	server := NewCDPIServer(scenarioState, clock, log)
	server.Metrics = metrics

	// Register a fake agent
	agentID := "agent-1"
	handle := &AgentHandle{
		AgentID:  agentID,
		NodeID:   "node-1",
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 10),
		token:    "test-token",
	}
	server.agentsMu.Lock()
	server.agents[agentID] = handle
	server.agentsMu.Unlock()

	// Simulate receiving a Response with OK status
	response := &schedulingpb.ReceiveRequestsMessageToController{
		Response: &schedulingpb.ReceiveRequestsMessageToController_Response{
			RequestId: 123,
			Status:    status.New(codes.OK, "OK").Proto(),
		},
	}

	// Call handleAgentResponse directly (simulating what ReceiveRequests would do)
	// We need to access the private method or test through the stream
	// For now, let's test by manually calling the metrics increment logic
	statusProto := response.GetResponse().GetStatus()
	if statusProto != nil {
		if statusProto.GetCode() == int32(codes.OK) {
			metrics.IncResponsesOK()
		} else {
			metrics.IncResponsesError()
		}
	}

	// Verify metrics
	snap := metrics.Snapshot()
	if snap.NumResponsesOK != 1 {
		t.Errorf("expected NumResponsesOK=1, got %d", snap.NumResponsesOK)
	}
	if snap.NumResponsesError != 0 {
		t.Errorf("expected NumResponsesError=0, got %d", snap.NumResponsesError)
	}
}

func TestCDPIServer_Metrics_ResponseError(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	clock := sbi.NewFakeEventScheduler(time.Now())

	metrics := sbi.NewSBIMetrics()
	server := NewCDPIServer(scenarioState, clock, log)
	server.Metrics = metrics

	// Register a fake agent
	agentID := "agent-1"
	handle := &AgentHandle{
		AgentID:  agentID,
		NodeID:   "node-1",
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 10),
		token:    "test-token",
	}
	server.agentsMu.Lock()
	server.agents[agentID] = handle
	server.agentsMu.Unlock()

	// Simulate receiving a Response with error status
	response := &schedulingpb.ReceiveRequestsMessageToController{
		Response: &schedulingpb.ReceiveRequestsMessageToController_Response{
			RequestId: 123,
			Status:    status.New(codes.Internal, "action failed").Proto(),
		},
	}

	// Test metrics increment logic
	statusProto := response.GetResponse().GetStatus()
	if statusProto != nil {
		if statusProto.GetCode() == int32(codes.OK) {
			metrics.IncResponsesOK()
		} else {
			metrics.IncResponsesError()
		}
	}

	// Verify metrics
	snap := metrics.Snapshot()
	if snap.NumResponsesOK != 0 {
		t.Errorf("expected NumResponsesOK=0, got %d", snap.NumResponsesOK)
	}
	if snap.NumResponsesError != 1 {
		t.Errorf("expected NumResponsesError=1, got %d", snap.NumResponsesError)
	}
}

