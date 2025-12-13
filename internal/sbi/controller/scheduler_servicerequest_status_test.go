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

// TestScheduler_ServiceRequestStatusUpdates verifies that the scheduler
// updates ServiceRequest status fields when paths are found and scheduled.
func TestScheduler_ServiceRequestStatusUpdates(t *testing.T) {
	// Setup: Create a minimal scenario with two nodes and a link
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)

	// Add transceiver model
	trx := &core.TransceiverModel{
		ID: "trx-ku",
		Band: core.FrequencyBand{
			MinGHz: 12.0,
			MaxGHz: 18.0,
		},
		MaxRangeKm: 10000,
	}
	netKB.AddTransceiverModel(trx)

	// Create nodes and interfaces
	nodeA := &model.NetworkNode{ID: "nodeA"}
	ifaceA := &core.NetworkInterface{
		ID:            "ifA",
		ParentNodeID:  "nodeA",
		Medium:        core.MediumWireless,
		TransceiverID: "trx-ku",
		IsOperational: true,
	}
	if err := scenarioState.CreateNode(nodeA, []*core.NetworkInterface{ifaceA}); err != nil {
		t.Fatalf("CreateNode(nodeA) failed: %v", err)
	}

	nodeB := &model.NetworkNode{ID: "nodeB"}
	ifaceB := &core.NetworkInterface{
		ID:            "ifB",
		ParentNodeID:  "nodeB",
		Medium:        core.MediumWireless,
		TransceiverID: "trx-ku",
		IsOperational: true,
	}
	if err := scenarioState.CreateNode(nodeB, []*core.NetworkInterface{ifaceB}); err != nil {
		t.Fatalf("CreateNode(nodeB) failed: %v", err)
	}

	// Create a link between the nodes
	link := &core.NetworkLink{
		ID:         "linkAB",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     core.MediumWireless,
		Status:     core.LinkStatusPotential,
	}
	if err := scenarioState.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	// Create a ServiceRequest
	sr := &model.ServiceRequest{
		ID:        "sr-1",
		SrcNodeID: "nodeA",
		DstNodeID: "nodeB",
		FlowRequirements: []model.FlowRequirement{
			{
				RequestedBandwidth: 100e6,
				ValidFrom:          time.Unix(0, 0),
				ValidTo:            time.Unix(9999999999, 0),
			},
		},
	}
	if err := scenarioState.CreateServiceRequest(sr); err != nil {
		t.Fatalf("CreateServiceRequest failed: %v", err)
	}

	// Verify initial status
	sr, err := scenarioState.GetServiceRequest("sr-1")
	if err != nil {
		t.Fatalf("GetServiceRequest failed: %v", err)
	}
	if sr.IsProvisionedNow {
		t.Errorf("initial IsProvisionedNow = true, want false")
	}
	if len(sr.ProvisionedIntervals) != 0 {
		t.Errorf("initial ProvisionedIntervals = %d, want 0", len(sr.ProvisionedIntervals))
	}

	// Setup scheduler with fake CDPI and clock
	T0 := time.Unix(1000, 0)
	fakeClock := &fakeClockForTest{now: T0}
	clock := sbi.NewEventScheduler(fakeClock)
	fakeCDPI := newFakeCDPIServerForScheduler(scenarioState, clock)
	scheduler := NewScheduler(scenarioState, clock, fakeCDPI, log)

	// Register agents in CDPI server (required for SendCreateEntry to work)
	// Create minimal agent handles with fake streams
	agentHandleA := &AgentHandle{
		AgentID:  "nodeA",
		NodeID:   "nodeA",
		Stream:   &fakeStream{},
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 10),
		token:    "test-token",
		seqNo:    0,
	}
	agentHandleB := &AgentHandle{
		AgentID:  "nodeB",
		NodeID:   "nodeB",
		Stream:   &fakeStream{},
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 10),
		token:    "test-token",
		seqNo:    0,
	}
	fakeCDPI.agentsMu.Lock()
	fakeCDPI.agents["nodeA"] = agentHandleA
	fakeCDPI.agents["nodeB"] = agentHandleB
	fakeCDPI.agentsMu.Unlock()

	// Run scheduler
	ctx := context.Background()
	if err := scheduler.ScheduleServiceRequests(ctx); err != nil {
		t.Fatalf("ScheduleServiceRequests failed: %v", err)
	}

	// Verify status was updated
	sr, err = scenarioState.GetServiceRequest("sr-1")
	if err != nil {
		t.Fatalf("GetServiceRequest after scheduling failed: %v", err)
	}
	if !sr.IsProvisionedNow {
		t.Errorf("IsProvisionedNow = false after scheduling, want true")
	}
	if len(sr.ProvisionedIntervals) == 0 {
		t.Errorf("ProvisionedIntervals = 0 after scheduling, want at least 1")
	}
	if len(sr.ProvisionedIntervals) > 0 {
		interval := sr.ProvisionedIntervals[0]
		if interval.Start.IsZero() || interval.End.IsZero() {
			t.Errorf("ProvisionedInterval has zero times: %+v", interval)
		}
		if interval.End.Before(interval.Start) {
			t.Errorf("ProvisionedInterval End before Start: %+v", interval)
		}
	}
}

// TestScheduler_ServiceRequestStatusNoPath verifies that ServiceRequest
// status is set to not provisioned when no path is found.
func TestScheduler_ServiceRequestStatusNoPath(t *testing.T) {
	// Setup: Create nodes without a connecting link
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)

	nodeA := &model.NetworkNode{ID: "nodeA"}
	if err := scenarioState.CreateNode(nodeA, []*core.NetworkInterface{}); err != nil {
		t.Fatalf("CreateNode(nodeA) failed: %v", err)
	}

	nodeB := &model.NetworkNode{ID: "nodeB"}
	if err := scenarioState.CreateNode(nodeB, []*core.NetworkInterface{}); err != nil {
		t.Fatalf("CreateNode(nodeB) failed: %v", err)
	}

	// Create a ServiceRequest
	sr := &model.ServiceRequest{
		ID:        "sr-1",
		SrcNodeID: "nodeA",
		DstNodeID: "nodeB",
	}
	if err := scenarioState.CreateServiceRequest(sr); err != nil {
		t.Fatalf("CreateServiceRequest failed: %v", err)
	}

	// Setup scheduler
	T0 := time.Unix(1000, 0)
	clock := sbi.NewFakeEventScheduler(T0)
	fakeCDPI := newFakeCDPIServerForScheduler(scenarioState, clock)
	scheduler := NewScheduler(scenarioState, clock, fakeCDPI, log)

	// Run scheduler (no path should be found)
	ctx := context.Background()
	if err := scheduler.ScheduleServiceRequests(ctx); err != nil {
		t.Fatalf("ScheduleServiceRequests failed: %v", err)
	}

	// Verify status is not provisioned
	sr, err := scenarioState.GetServiceRequest("sr-1")
	if err != nil {
		t.Fatalf("GetServiceRequest failed: %v", err)
	}
	if sr.IsProvisionedNow {
		t.Errorf("IsProvisionedNow = true when no path found, want false")
	}
	if len(sr.ProvisionedIntervals) != 0 {
		t.Errorf("ProvisionedIntervals = %d when no path found, want 0", len(sr.ProvisionedIntervals))
	}
}
