package agent

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestSimAgent_DumpAgentState_Basic(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClient{}
	stream := &fakeStream{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, log)

	// Add a pending action
	agent.mu.Lock()
	agent.pending["entry-1"] = &sbi.ScheduledAction{
		EntryID: "entry-1",
		When:    time.Now(),
		Type:    sbi.ScheduledUpdateBeam,
		Beam: &sbi.BeamSpec{
			NodeID:       "node1",
			InterfaceID:  "if1",
			TargetNodeID: "node2",
			TargetIfID:   "if2",
		},
	}
	agent.mu.Unlock()

	dump := agent.DumpAgentState(nil)

	// Verify basic fields
	if !strings.Contains(dump, "Agent: agent-1") {
		t.Errorf("dump should contain Agent ID, got: %s", dump)
	}
	if !strings.Contains(dump, "NodeID: node1") {
		t.Errorf("dump should contain NodeID, got: %s", dump)
	}
	if !strings.Contains(dump, "Pending Actions: 1") {
		t.Errorf("dump should show 1 pending action, got: %s", dump)
	}
	if !strings.Contains(dump, "entry-1") {
		t.Errorf("dump should contain entry ID, got: %s", dump)
	}
	if !strings.Contains(dump, "UpdateBeam") {
		t.Errorf("dump should contain action type, got: %s", dump)
	}
}

func TestSimAgent_DumpAgentState_WithTelemetry(t *testing.T) {
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

	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClient{}
	stream := &fakeStream{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, log)

	// Create telemetry state with metrics
	telemetryState := state.NewTelemetryState()
	telemetryState.UpdateMetrics(&state.InterfaceMetrics{
		NodeID:      "node1",
		InterfaceID: "if1",
		Up:          true,
		BytesTx:     1000,
		BytesRx:     500,
		SNRdB:       25.5,
		Modulation:  "QPSK",
	})

	dump := agent.DumpAgentState(telemetryState)

	// Verify telemetry is included
	if !strings.Contains(dump, "Last Telemetry Metrics") {
		t.Errorf("dump should contain telemetry section, got: %s", dump)
	}
	if !strings.Contains(dump, "Interface: if1") {
		t.Errorf("dump should contain interface ID, got: %s", dump)
	}
	if !strings.Contains(dump, "BytesTx: 1000") {
		t.Errorf("dump should contain BytesTx, got: %s", dump)
	}
	if !strings.Contains(dump, "SNRdB: 25.50") {
		t.Errorf("dump should contain SNRdB, got: %s", dump)
	}
}

func TestSimAgent_DumpAgentState_NoPendingActions(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClient{}
	stream := &fakeStream{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, log)

	dump := agent.DumpAgentState(nil)

	// Verify it handles empty pending actions gracefully
	if !strings.Contains(dump, "Pending Actions: 0") {
		t.Errorf("dump should show 0 pending actions, got: %s", dump)
	}
	if !strings.Contains(dump, "(no pending actions)") {
		t.Errorf("dump should indicate no pending actions, got: %s", dump)
	}
}

func TestSimAgent_DumpAgentState_WithMetrics(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClient{}
	stream := &fakeStream{}

	metrics := sbi.NewSBIMetrics()
	metrics.IncActionsExecuted()
	metrics.IncActionsExecuted()

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, log)
	agent.Metrics = metrics

	dump := agent.DumpAgentState(nil)

	// Verify metrics snapshot is included
	if !strings.Contains(dump, "Metrics Snapshot") {
		t.Errorf("dump should contain metrics snapshot, got: %s", dump)
	}
	if !strings.Contains(dump, "Actions Executed: 2") {
		t.Errorf("dump should show actions executed count, got: %s", dump)
	}
}

func TestSimAgent_CollectModemMetrics(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)

	node1 := &model.NetworkNode{ID: "node1"}
	if err := scenarioState.CreateNode(node1, []*core.NetworkInterface{
		{ID: "if1", ParentNodeID: "node1", IsOperational: true, TransceiverID: "trx1"},
	}); err != nil {
		t.Fatalf("CreateNode node1 failed: %v", err)
	}
	if err := scenarioState.CreateNode(&model.NetworkNode{ID: "node2"}, []*core.NetworkInterface{
		{ID: "if2", ParentNodeID: "node2", IsOperational: true, TransceiverID: "trx1"},
	}); err != nil {
		t.Fatalf("CreateNode node2 failed: %v", err)
	}

	trx := &core.TransceiverModel{ID: "trx1", Name: "QPSK"}
	if err := netKB.AddTransceiverModel(trx); err != nil {
		t.Fatalf("AddTransceiverModel failed: %v", err)
	}

	link := &core.NetworkLink{
		ID:              "link1",
		InterfaceA:      "if1",
		InterfaceB:      "if2",
		Status:          core.LinkStatusActive,
		IsUp:            true,
		MaxDataRateMbps: 25,
		SNRdB:           12.3,
	}
	if err := scenarioState.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	scheduler := sbi.NewFakeEventScheduler(time.Now())
	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, nil, nil, log)

	ifaces := scenarioState.ListInterfacesForNode("node1")
	if len(ifaces) == 0 {
		t.Fatalf("expected at least one interface")
	}

	modem := agent.CollectModemMetrics("node1", ifaces[0])
	if modem == nil {
		t.Fatalf("expected modem metrics, got nil")
	}
	if modem.InterfaceID != "if1" {
		t.Fatalf("unexpected interface: %s", modem.InterfaceID)
	}
	if math.Abs(modem.SNRdB-12.3) > 1e-7 {
		t.Fatalf("unexpected SNR: %f", modem.SNRdB)
	}
	if modem.Modulation != "QPSK" {
		t.Fatalf("unexpected modulation: %s", modem.Modulation)
	}
	if modem.ThroughputBps == 0 {
		t.Fatalf("expected non-zero throughput")
	}
}
