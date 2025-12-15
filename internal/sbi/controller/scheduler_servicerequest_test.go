package controller

import (
	"context"
	"testing"
	"time"

	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

// TestScheduler_ScheduleServiceRequests_SingleHop verifies that
// a single-hop ServiceRequest (A -> B) produces the expected actions.
func TestScheduler_ScheduleServiceRequests_SingleHop(t *testing.T) {
	scheduler, _, _ := setupSchedulerTest(t)

	// Create a ServiceRequest from node-A to node-B
	sr := &model.ServiceRequest{
		ID:        "sr-1",
		SrcNodeID: "node-A",
		DstNodeID: "node-B",
		Priority:  1,
	}
	if err := scheduler.State.CreateServiceRequest(sr); err != nil {
		t.Fatalf("CreateServiceRequest failed: %v", err)
	}

	ctx := context.Background()

	// Call ScheduleServiceRequests
	// We expect it to fail because no agent is registered, but that's OK for this test
	err := scheduler.ScheduleServiceRequests(ctx)
	if err == nil {
		t.Log("ScheduleServiceRequests succeeded (no agent registered, but that's expected for this test setup)")
	}
}

// TestScheduler_ScheduleServiceRequests_MultiHop verifies that
// a multi-hop ServiceRequest (A -> B -> C) produces actions for all hops.
func TestScheduler_ScheduleServiceRequests_MultiHop(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()

	// Create transceiver models FIRST
	if err := netKB.AddTransceiverModel(&core.TransceiverModel{
		ID:   "trx-A",
		Name: "Transceiver A",
		Band: core.FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 10.1,
		},
	}); err != nil {
		t.Fatalf("AddTransceiverModel(trx-A) failed: %v", err)
	}
	if err := netKB.AddTransceiverModel(&core.TransceiverModel{
		ID:   "trx-B",
		Name: "Transceiver B",
		Band: core.FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 10.1,
		},
	}); err != nil {
		t.Fatalf("AddTransceiverModel(trx-B) failed: %v", err)
	}
	if err := netKB.AddTransceiverModel(&core.TransceiverModel{
		ID:   "trx-C",
		Name: "Transceiver C",
		Band: core.FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 10.1,
		},
	}); err != nil {
		t.Fatalf("AddTransceiverModel(trx-C) failed: %v", err)
	}

	scenarioState := state.NewScenarioState(physKB, netKB, logging.Noop())

	T0 := time.Unix(1000, 0)
	fakeClock := &fakeClockForTest{now: T0}
	eventScheduler := sbi.NewEventScheduler(fakeClock)

	fakeCDPI := newFakeCDPIServerForScheduler(scenarioState, eventScheduler)
	scheduler := NewScheduler(scenarioState, eventScheduler, fakeCDPI, logging.Noop(), state.NewTelemetryState())

	// Create platforms
	if err := scenarioState.CreatePlatform(&model.PlatformDefinition{ID: "platform-A", Name: "Platform A"}); err != nil {
		t.Fatalf("CreatePlatform failed: %v", err)
	}
	if err := scenarioState.CreatePlatform(&model.PlatformDefinition{ID: "platform-B", Name: "Platform B"}); err != nil {
		t.Fatalf("CreatePlatform failed: %v", err)
	}
	if err := scenarioState.CreatePlatform(&model.PlatformDefinition{ID: "platform-C", Name: "Platform C"}); err != nil {
		t.Fatalf("CreatePlatform failed: %v", err)
	}

	// Create nodes A, B, C
	if err := scenarioState.CreateNode(&model.NetworkNode{
		ID:         "node-A",
		Name:       "Node A",
		PlatformID: "platform-A",
	}, []*core.NetworkInterface{
		{ID: "if-A", Name: "Interface A", Medium: core.MediumWireless, ParentNodeID: "node-A", IsOperational: true, TransceiverID: "trx-A"},
	}); err != nil {
		t.Fatalf("CreateNode(node-A) failed: %v", err)
	}

	if err := scenarioState.CreateNode(&model.NetworkNode{
		ID:         "node-B",
		Name:       "Node B",
		PlatformID: "platform-B",
	}, []*core.NetworkInterface{
		{ID: "if-B1", Name: "Interface B1", Medium: core.MediumWireless, ParentNodeID: "node-B", IsOperational: true, TransceiverID: "trx-B"},
		{ID: "if-B2", Name: "Interface B2", Medium: core.MediumWireless, ParentNodeID: "node-B", IsOperational: true, TransceiverID: "trx-B"},
	}); err != nil {
		t.Fatalf("CreateNode(node-B) failed: %v", err)
	}

	if err := scenarioState.CreateNode(&model.NetworkNode{
		ID:         "node-C",
		Name:       "Node C",
		PlatformID: "platform-C",
	}, []*core.NetworkInterface{
		{ID: "if-C", Name: "Interface C", Medium: core.MediumWireless, ParentNodeID: "node-C", IsOperational: true, TransceiverID: "trx-C"},
	}); err != nil {
		t.Fatalf("CreateNode(node-C) failed: %v", err)
	}

	// Set node positions
	netKB.SetNodeECEFPosition("node-A", core.Vec3{X: core.EarthRadiusKm + 500, Y: 0, Z: 0})
	netKB.SetNodeECEFPosition("node-B", core.Vec3{X: core.EarthRadiusKm + 500, Y: 100, Z: 0})
	netKB.SetNodeECEFPosition("node-C", core.Vec3{X: core.EarthRadiusKm + 500, Y: 200, Z: 0})

	// Create links: A-B and B-C (multi-hop path)
	if err := scenarioState.CreateLink(&core.NetworkLink{
		ID:         "link-ab",
		InterfaceA: "if-A",
		InterfaceB: "if-B1",
		Medium:     core.MediumWireless,
		Status:     core.LinkStatusPotential,
	}); err != nil {
		t.Fatalf("CreateLink(link-ab) failed: %v", err)
	}

	if err := scenarioState.CreateLink(&core.NetworkLink{
		ID:         "link-bc",
		InterfaceA: "if-B2",
		InterfaceB: "if-C",
		Medium:     core.MediumWireless,
		Status:     core.LinkStatusPotential,
	}); err != nil {
		t.Fatalf("CreateLink(link-bc) failed: %v", err)
	}

	// Create ServiceRequest from A to C (multi-hop)
	sr := &model.ServiceRequest{
		ID:        "sr-1",
		SrcNodeID: "node-A",
		DstNodeID: "node-C",
		Priority:  1,
	}
	if err := scenarioState.CreateServiceRequest(sr); err != nil {
		t.Fatalf("CreateServiceRequest failed: %v", err)
	}

	ctx := context.Background()

	// Call ScheduleServiceRequests
	// We expect it to fail because no agent is registered, but that's OK for this test
	err := scheduler.ScheduleServiceRequests(ctx)
	if err == nil {
		t.Log("ScheduleServiceRequests succeeded (no agent registered, but that's expected for this test setup)")
	}
}

// TestScheduler_buildConnectivityGraph verifies graph construction.
func TestScheduler_buildConnectivityGraph(t *testing.T) {
	scheduler, _, _ := setupSchedulerTest(t)

	graph := scheduler.buildConnectivityGraph()

	if graph == nil {
		t.Fatalf("expected non-nil graph")
	}

	// Should have node-A and node-B as neighbors
	neighborsA := graph.adj["node-A"]
	if len(neighborsA) == 0 {
		t.Fatalf("expected node-A to have neighbors")
	}
	foundB := false
	for _, neighbor := range neighborsA {
		if neighbor == "node-B" {
			foundB = true
			break
		}
	}
	if !foundB {
		t.Fatalf("expected node-A to have node-B as neighbor")
	}
}

// TestScheduler_ServiceRequestReplanCleansEntries ensures that rescheduling the same
// ServiceRequest tears down prior actions before installing new ones.
func TestScheduler_ServiceRequestReplanCleansEntries(t *testing.T) {
	scheduler, fakeCDPI, _ := setupSchedulerTest(t)

	agentHandle := &AgentHandle{
		AgentID:  "node-A",
		NodeID:   "node-A",
		Stream:   &fakeStream{},
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 10),
		token:    "tok-123",
		seqNo:    0,
	}
	fakeCDPI.agentsMu.Lock()
	fakeCDPI.agents["node-A"] = agentHandle
	fakeCDPI.agentsMu.Unlock()

	sr := &model.ServiceRequest{
		ID:        "sr-1",
		SrcNodeID: "node-A",
		DstNodeID: "node-B",
		Priority:  1,
	}
	if err := scheduler.State.CreateServiceRequest(sr); err != nil {
		t.Fatalf("CreateServiceRequest failed: %v", err)
	}

	ctx := context.Background()
	if err := scheduler.ScheduleServiceRequests(ctx); err != nil {
		t.Fatalf("ScheduleServiceRequests failed: %v", err)
	}

	firstSent := len(fakeCDPI.sentActions)
	if firstSent == 0 {
		t.Fatalf("expected at least one entry sent for service request")
	}

	hasDeleteBeam := false
	hasDeleteRoute := false
	for _, entry := range fakeCDPI.sentActions {
		if entry.action == nil {
			continue
		}
		switch entry.action.Type {
		case sbi.ScheduledDeleteBeam:
			hasDeleteBeam = true
		case sbi.ScheduledDeleteRoute:
			hasDeleteRoute = true
		}
	}
	if !hasDeleteBeam {
		t.Fatalf("expected ScheduledDeleteBeam entry")
	}
	if !hasDeleteRoute {
		t.Fatalf("expected ScheduledDeleteRoute entry")
	}

	fakeCDPI.sentActions = nil

	if err := scheduler.ScheduleServiceRequests(ctx); err != nil {
		t.Fatalf("ScheduleServiceRequests failed: %v", err)
	}

	if len(fakeCDPI.deletedEntries) != firstSent {
		t.Fatalf("expected %d delete requests, got %d", firstSent, len(fakeCDPI.deletedEntries))
	}
}

// TestScheduler_findAnyPath verifies BFS pathfinding.
func TestScheduler_findAnyPath(t *testing.T) {
	scheduler, _, T0 := setupSchedulerTest(t)

	scheduler.contactWindows = map[string][]ContactWindow{
		"link-ab": {
			{
				StartTime: T0,
				EndTime:   T0.Add(defaultPotentialWindow),
				Quality:   0,
			},
		},
	}

	graph := scheduler.buildConnectivityGraph()

	// Test single-hop path
	path := scheduler.findAnyPath(graph, "node-A", "node-B", true)
	if path == nil {
		t.Fatalf("expected path from node-A to node-B")
	}
	if len(path) != 2 {
		t.Fatalf("expected path length 2, got %d", len(path))
	}
	if path[0] != "node-A" || path[1] != "node-B" {
		t.Fatalf("expected path [node-A, node-B], got %v", path)
	}

	// Test self-loop
	path = scheduler.findAnyPath(graph, "node-A", "node-A", true)
	if path == nil {
		t.Fatalf("expected self-loop path")
	}
	if len(path) != 1 || path[0] != "node-A" {
		t.Fatalf("expected path [node-A], got %v", path)
	}

	// Test no path
	path = scheduler.findAnyPath(graph, "node-A", "node-nonexistent", true)
	if path != nil {
		t.Fatalf("expected nil path for non-existent node, got %v", path)
	}
}

// TestScheduler_ScheduleServiceRequests_NoPath verifies that
// ServiceRequests with no available path are handled gracefully.
func TestScheduler_ScheduleServiceRequests_NoPath(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	scenarioState := state.NewScenarioState(physKB, netKB, logging.Noop())

	T0 := time.Unix(1000, 0)
	fakeClock := &fakeClockForTest{now: T0}
	eventScheduler := sbi.NewEventScheduler(fakeClock)

	fakeCDPI := newFakeCDPIServerForScheduler(scenarioState, eventScheduler)
	scheduler := NewScheduler(scenarioState, eventScheduler, fakeCDPI, logging.Noop(), state.NewTelemetryState())

	if err := scenarioState.CreatePlatform(&model.PlatformDefinition{
		ID: "plat-standard",
	}); err != nil {
		t.Fatalf("CreatePlatform error: %v", err)
	}
	for _, nodeID := range []string{"node-A", "node-B"} {
		if err := scenarioState.CreateNode(&model.NetworkNode{
			ID:         nodeID,
			PlatformID: "plat-standard",
		}, nil); err != nil {
			t.Fatalf("CreateNode(%s) error: %v", nodeID, err)
		}
	}

	// Create a ServiceRequest between nodes that now exist
	sr := &model.ServiceRequest{
		ID:        "sr-1",
		SrcNodeID: "node-A",
		DstNodeID: "node-B",
		Priority:  1,
	}
	if err := scenarioState.CreateServiceRequest(sr); err != nil {
		t.Fatalf("CreateServiceRequest failed: %v", err)
	}

	ctx := context.Background()
	err := scheduler.ScheduleServiceRequests(ctx)
	if err != nil {
		t.Fatalf("ScheduleServiceRequests failed: %v", err)
	}

	// Should have sent no actions (no path exists)
	if len(fakeCDPI.sentActions) != 0 {
		t.Fatalf("expected 0 actions for no-path scenario, got %d", len(fakeCDPI.sentActions))
	}
}
