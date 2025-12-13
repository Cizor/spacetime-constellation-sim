package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
)

func TestCDPIServer_DumpAgentState(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	clock := sbi.NewFakeEventScheduler(time.Now())

	server := NewCDPIServer(scenarioState, clock, log)

	// Register a fake agent
	agentID := "agent-1"
	handle := &AgentHandle{
		AgentID:  agentID,
		NodeID:   "node-1",
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 10),
		token:    "test-token-123",
	}
	handle.seqNoMu.Lock()
	handle.seqNo = 5
	handle.seqNoMu.Unlock()

	server.agentsMu.Lock()
	server.agents[agentID] = handle
	server.agentsMu.Unlock()

	// Dump agent state
	dump, err := server.DumpAgentState(agentID)
	if err != nil {
		t.Fatalf("DumpAgentState failed: %v", err)
	}

	// Verify output
	if !strings.Contains(dump, "CDPI Agent Handle") {
		t.Errorf("dump should contain header, got: %s", dump)
	}
	if !strings.Contains(dump, "AgentID: agent-1") {
		t.Errorf("dump should contain AgentID, got: %s", dump)
	}
	if !strings.Contains(dump, "NodeID: node-1") {
		t.Errorf("dump should contain NodeID, got: %s", dump)
	}
	if !strings.Contains(dump, "Token: test-token-123") {
		t.Errorf("dump should contain token, got: %s", dump)
	}
	if !strings.Contains(dump, "SeqNo: 5") {
		t.Errorf("dump should contain SeqNo, got: %s", dump)
	}
}

func TestCDPIServer_DumpAgentState_NotFound(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	clock := sbi.NewFakeEventScheduler(time.Now())

	server := NewCDPIServer(scenarioState, clock, log)

	// Try to dump non-existent agent
	_, err := server.DumpAgentState("non-existent")
	if err == nil {
		t.Fatalf("expected error for non-existent agent")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

