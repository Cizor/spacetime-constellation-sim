package agent

import (
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestSimAgent_deriveInterfaceState_NoLinks(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClient{}
	stream := &fakeStream{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	up, bandwidth := agent.deriveInterfaceState("node1", "if1")
	if up {
		t.Fatalf("expected up=false for interface with no links")
	}
	if bandwidth != 0 {
		t.Fatalf("expected bandwidth=0 for interface with no links, got %f", bandwidth)
	}
}

func TestSimAgent_deriveInterfaceState_ActiveLink(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)

	// Create nodes and interfaces
	node1 := &model.NetworkNode{ID: "node1"}
	node2 := &model.NetworkNode{ID: "node2"}
	if err := scenarioState.CreateNode(node1, []*core.NetworkInterface{
		{ID: "if1", ParentNodeID: "node1", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode(node1) failed: %v", err)
	}
	if err := scenarioState.CreateNode(node2, []*core.NetworkInterface{
		{ID: "if2", ParentNodeID: "node2", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode(node2) failed: %v", err)
	}

	// Create an active link (interface IDs are stored without node prefix)
	link := &core.NetworkLink{
		ID:             "link1",
		InterfaceA:     "if1",
		InterfaceB:     "if2",
		Medium:         core.MediumWireless,
		Status:         core.LinkStatusActive,
		IsUp:           true,
		MaxDataRateMbps: 100.0,
	}
	if err := scenarioState.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClient{}
	stream := &fakeStream{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	up, bandwidth := agent.deriveInterfaceState("node1", "if1")
	if !up {
		t.Fatalf("expected up=true for interface with active link")
	}
	expectedBandwidth := 100.0 * 1e6 // 100 Mbps = 100e6 bps
	if bandwidth != expectedBandwidth {
		t.Fatalf("expected bandwidth=%f bps, got %f", expectedBandwidth, bandwidth)
	}
}

func TestSimAgent_deriveInterfaceState_InactiveLink(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)

	node1 := &model.NetworkNode{ID: "node1"}
	node2 := &model.NetworkNode{ID: "node2"}
	if err := scenarioState.CreateNode(node1, []*core.NetworkInterface{
		{ID: "if1", ParentNodeID: "node1", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode(node1) failed: %v", err)
	}
	if err := scenarioState.CreateNode(node2, []*core.NetworkInterface{
		{ID: "if2", ParentNodeID: "node2", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode(node2) failed: %v", err)
	}

	// Create an inactive link (Potential status)
	link := &core.NetworkLink{
		ID:             "link1",
		InterfaceA:     "if1",
		InterfaceB:     "if2",
		Medium:         core.MediumWireless,
		Status:         core.LinkStatusPotential,
		IsUp:           false,
		MaxDataRateMbps: 100.0,
	}
	if err := scenarioState.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClient{}
	stream := &fakeStream{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	up, bandwidth := agent.deriveInterfaceState("node1", "if1")
	if up {
		t.Fatalf("expected up=false for interface with inactive link")
	}
	if bandwidth != 0 {
		t.Fatalf("expected bandwidth=0 for inactive link, got %f", bandwidth)
	}
}

func TestSimAgent_deriveInterfaceState_MultipleLinks(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)

	node1 := &model.NetworkNode{ID: "node1"}
	node2 := &model.NetworkNode{ID: "node2"}
	node3 := &model.NetworkNode{ID: "node3"}
	if err := scenarioState.CreateNode(node1, []*core.NetworkInterface{
		{ID: "if1", ParentNodeID: "node1", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode(node1) failed: %v", err)
	}
	if err := scenarioState.CreateNode(node2, []*core.NetworkInterface{
		{ID: "if2", ParentNodeID: "node2", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode(node2) failed: %v", err)
	}
	if err := scenarioState.CreateNode(node3, []*core.NetworkInterface{
		{ID: "if3", ParentNodeID: "node3", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode(node3) failed: %v", err)
	}

	// Create multiple links with different bandwidths
	link1 := &core.NetworkLink{
		ID:             "link1",
		InterfaceA:     "if1",
		InterfaceB:     "if2",
		Medium:         core.MediumWireless,
		Status:         core.LinkStatusActive,
		IsUp:           true,
		MaxDataRateMbps: 50.0,
	}
	link2 := &core.NetworkLink{
		ID:             "link2",
		InterfaceA:     "if1",
		InterfaceB:     "if3",
		Medium:         core.MediumWireless,
		Status:         core.LinkStatusActive,
		IsUp:           true,
		MaxDataRateMbps: 200.0, // Higher bandwidth
	}
	if err := scenarioState.CreateLinks(link1, link2); err != nil {
		t.Fatalf("CreateLinks failed: %v", err)
	}

	scheduler := sbi.NewFakeEventScheduler(time.Now())
	telemetryCli := &fakeTelemetryClient{}
	stream := &fakeStream{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream, logging.Noop())

	up, bandwidth := agent.deriveInterfaceState("node1", "if1")
	if !up {
		t.Fatalf("expected up=true for interface with active links")
	}
	// Should return maximum bandwidth
	expectedBandwidth := 200.0 * 1e6 // 200 Mbps = 200e6 bps
	if bandwidth != expectedBandwidth {
		t.Fatalf("expected bandwidth=%f bps (max of all links), got %f", expectedBandwidth, bandwidth)
	}
}

