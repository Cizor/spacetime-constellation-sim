package controller

import (
	"context"
	"strings"
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

// TestScheduler_MultipleLinks_BeamAndRouteScheduling verifies that
// the scheduler correctly handles multiple independent links, emitting
// the expected actions for each link.
func TestScheduler_MultipleLinks_BeamAndRouteScheduling(t *testing.T) {
	// Setup: Create a scenario with three nodes and two independent links
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()

	// Add transceiver models
	for _, id := range []string{"trx-A", "trx-B", "trx-C"} {
		if err := netKB.AddTransceiverModel(&core.TransceiverModel{
			ID:   id,
			Name: "Transceiver " + id,
			Band: core.FrequencyBand{
				MinGHz: 10.0,
				MaxGHz: 10.1,
			},
		}); err != nil {
			t.Fatalf("AddTransceiverModel(%s) failed: %v", id, err)
		}
	}

	scenarioState := state.NewScenarioState(physKB, netKB, logging.Noop())
	telemetryState := state.NewTelemetryState()

	// Create platforms
	for _, id := range []string{"platform-A", "platform-B", "platform-C"} {
		if err := scenarioState.CreatePlatform(&model.PlatformDefinition{
			ID:   id,
			Name: "Platform " + id,
		}); err != nil {
			t.Fatalf("CreatePlatform(%s) failed: %v", id, err)
		}
	}

	// Create nodes with interfaces
	nodes := []struct {
		nodeID string
		ifID   string
		trxID  string
	}{
		{"node-A", "if-A", "trx-A"},
		{"node-B", "if-B", "trx-B"},
		{"node-C", "if-C", "trx-C"},
	}

	for _, n := range nodes {
		if err := scenarioState.CreateNode(&model.NetworkNode{
			ID:         n.nodeID,
			Name:       "Node " + n.nodeID,
			PlatformID: "platform-" + n.nodeID[len("node-"):],
		}, []*core.NetworkInterface{
			{
				ID:            n.ifID,
				Name:          "Interface " + n.ifID,
				Medium:        core.MediumWireless,
				ParentNodeID:  n.nodeID,
				IsOperational: true,
				TransceiverID: n.trxID,
			},
		}); err != nil {
			t.Fatalf("CreateNode(%s) failed: %v", n.nodeID, err)
		}
	}

	// Set node positions for clear LoS
	netKB.SetNodeECEFPosition("node-A", core.Vec3{X: core.EarthRadiusKm + 500, Y: 0, Z: 0})
	netKB.SetNodeECEFPosition("node-B", core.Vec3{X: core.EarthRadiusKm + 500, Y: 100, Z: 0})
	netKB.SetNodeECEFPosition("node-C", core.Vec3{X: core.EarthRadiusKm + 500, Y: 200, Z: 0})

	// Create two independent potential links: A-B and B-C
	links := []struct {
		linkID string
		ifA    string
		ifB    string
	}{
		{"link-ab", "if-A", "if-B"},
		{"link-bc", "if-B", "if-C"},
	}

	for _, l := range links {
		if err := scenarioState.CreateLink(&core.NetworkLink{
			ID:         l.linkID,
			InterfaceA: l.ifA,
			InterfaceB: l.ifB,
			Medium:     core.MediumWireless,
			Status:     core.LinkStatusPotential,
		}); err != nil {
			t.Fatalf("CreateLink(%s) failed: %v", l.linkID, err)
		}
	}

	// Create fake clock and event scheduler
	T0 := time.Unix(1000, 0)
	fakeClock := &fakeClockForTest{now: T0}
	eventScheduler := sbi.NewEventScheduler(fakeClock)

	// Create fake CDPI server with registered agents
	fakeCDPI := newFakeCDPIServerForScheduler(scenarioState, eventScheduler)

	// Register agents for all three nodes
	agentHandles := make(map[string]*AgentHandle)
	for _, nodeID := range []string{"node-A", "node-B", "node-C"} {
		handle := &AgentHandle{
			AgentID:  nodeID,
			NodeID:   nodeID,
			Stream:   &fakeStream{},
			outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 20),
			token:    "tok-123",
			seqNo:    0,
		}
		fakeCDPI.agentsMu.Lock()
		fakeCDPI.agents[nodeID] = handle
		fakeCDPI.agentsMu.Unlock()
		agentHandles[nodeID] = handle
	}

	scheduler := NewScheduler(scenarioState, eventScheduler, fakeCDPI, logging.Noop(), telemetryState)
	ctx := context.Background()

	// Test ScheduleLinkBeams for multiple links
	err := scheduler.ScheduleLinkBeams(ctx)
	if err != nil {
		t.Fatalf("ScheduleLinkBeams failed: %v", err)
	}

	// Collect all beam messages from all agents
	allBeamMessages := make(map[string][]*schedulingpb.ReceiveRequestsMessageFromController)
	for nodeID, handle := range agentHandles {
		var messages []*schedulingpb.ReceiveRequestsMessageFromController
		for {
			select {
			case msg := <-handle.outgoing:
				messages = append(messages, msg)
			default:
				goto doneNode
			}
		}
	doneNode:
		allBeamMessages[nodeID] = messages
	}

	// Verify: Each link should produce 2 beam actions (UpdateBeam + DeleteBeam)
	// Link A-B: controlled by node-A (InterfaceA's parent)
	// Link B-C: controlled by node-B (InterfaceA's parent)
	// So we expect:
	// - node-A: 2 actions (UpdateBeam + DeleteBeam for link A-B)
	// - node-B: 2 actions (UpdateBeam + DeleteBeam for link B-C)
	// - node-C: 0 actions (not controlling any link)

	if len(allBeamMessages["node-A"]) < 2 {
		t.Errorf("expected at least 2 beam actions for node-A (link A-B), got %d", len(allBeamMessages["node-A"]))
	}
	if len(allBeamMessages["node-B"]) < 2 {
		t.Errorf("expected at least 2 beam actions for node-B (link B-C), got %d", len(allBeamMessages["node-B"]))
	}

	// Determine contact windows for each link
	windowABs := scheduler.contactWindowsForLink("link-ab")
	if len(windowABs) == 0 {
		t.Fatalf("expected contact windows for link-ab")
	}
	windowAB := windowABs[0]

	windowBCs := scheduler.contactWindowsForLink("link-bc")
	if len(windowBCs) == 0 {
		t.Fatalf("expected contact windows for link-bc")
	}
	windowBC := windowBCs[0]

	// Verify UpdateBeam and DeleteBeam for each link
	verifyBeamActions(t, allBeamMessages["node-A"], windowAB, T0)
	verifyBeamActions(t, allBeamMessages["node-B"], windowBC, T0)

	// Test ScheduleLinkRoutes for multiple links
	err = scheduler.ScheduleLinkRoutes(ctx)
	if err != nil {
		t.Fatalf("ScheduleLinkRoutes failed: %v", err)
	}

	// Collect all route messages from all agents
	allRouteMessages := make(map[string][]*schedulingpb.ReceiveRequestsMessageFromController)
	for nodeID, handle := range agentHandles {
		var messages []*schedulingpb.ReceiveRequestsMessageFromController
		for {
			select {
			case msg := <-handle.outgoing:
				messages = append(messages, msg)
			default:
				goto doneRouteNode
			}
		}
	doneRouteNode:
		allRouteMessages[nodeID] = messages
	}

	// Verify: Each link should produce 4 route actions (2x SetRoute + 2x DeleteRoute)
	// Link A-B: SetRoute on node-A and node-B at T_on, DeleteRoute on both at T_off
	// Link B-C: SetRoute on node-B and node-C at T_on, DeleteRoute on both at T_off
	// So we expect:
	// - node-A: 2 actions (SetRoute + DeleteRoute for link A-B)
	// - node-B: 4 actions (2x SetRoute + 2x DeleteRoute for both links A-B and B-C)
	// - node-C: 2 actions (SetRoute + DeleteRoute for link B-C)

	if len(allRouteMessages["node-A"]) < 2 {
		t.Errorf("expected at least 2 route actions for node-A (link A-B), got %d", len(allRouteMessages["node-A"]))
	}
	if len(allRouteMessages["node-B"]) < 4 {
		t.Errorf("expected at least 4 route actions for node-B (links A-B and B-C), got %d", len(allRouteMessages["node-B"]))
	}
	if len(allRouteMessages["node-C"]) < 2 {
		t.Errorf("expected at least 2 route actions for node-C (link B-C), got %d", len(allRouteMessages["node-C"]))
	}

	// Verify SetRoute and DeleteRoute actions
	routeWindowsAB := map[string]ContactWindow{"node-B": windowAB}
	routeWindowsBA := map[string]ContactWindow{
		"node-A": windowAB,
		"node-C": windowBC,
	}
	routeWindowsCB := map[string]ContactWindow{"node-B": windowBC}

	verifyRouteActionsForNode(t, allRouteMessages["node-A"], []string{"node-B"}, routeWindowsAB, T0)
	verifyRouteActionsForNode(t, allRouteMessages["node-B"], []string{"node-A", "node-C"}, routeWindowsBA, T0)
	verifyRouteActionsForNode(t, allRouteMessages["node-C"], []string{"node-B"}, routeWindowsCB, T0)
}

// verifyBeamActions verifies that beam messages contain UpdateBeam and DeleteBeam actions.
func verifyBeamActions(t *testing.T, messages []*schedulingpb.ReceiveRequestsMessageFromController, window ContactWindow, T0 time.Time) {
	updateBeamFound := false
	deleteBeamFound := false

	for _, msg := range messages {
		createEntry := msg.GetCreateEntry()
		if createEntry == nil {
			continue
		}

		updateBeam := createEntry.GetUpdateBeam()
		deleteBeam := createEntry.GetDeleteBeam()

		if updateBeam != nil {
			updateBeamFound = true
			when := createEntry.GetTime()
			if when != nil {
				whenTime := when.AsTime()
				if !whenTime.Equal(window.StartTime) {
					t.Errorf("UpdateBeam When = %v, expected %v", whenTime, window.StartTime)
				}
			}
		}

		if deleteBeam != nil {
			deleteBeamFound = true
			when := createEntry.GetTime()
			if when != nil {
				whenTime := when.AsTime()
				if !whenTime.Equal(window.EndTime) {
					t.Errorf("DeleteBeam When = %v, expected %v", whenTime, window.EndTime)
				}
			}
		}
	}

	if !updateBeamFound {
		t.Errorf("UpdateBeam action not found")
	}
	if !deleteBeamFound {
		t.Errorf("DeleteBeam action not found")
	}
}

// verifyRouteActionsForNode verifies that route messages contain SetRoute and DeleteRoute actions
// for at least one of the expected destination nodes.
func verifyRouteActionsForNode(t *testing.T, messages []*schedulingpb.ReceiveRequestsMessageFromController, expectedDestNodeIDs []string, windows map[string]ContactWindow, T0 time.Time) {
	setRouteFound := false
	deleteRouteFound := false

	for _, destID := range expectedDestNodeIDs {
		if _, ok := windows[destID]; !ok {
			t.Fatalf("missing contact window for expected destination %s", destID)
		}
	}

	for _, msg := range messages {
		createEntry := msg.GetCreateEntry()
		if createEntry == nil {
			continue
		}

		setRoute := createEntry.GetSetRoute()
		deleteRoute := createEntry.GetDeleteRoute()

		if setRoute != nil {
			setRouteFound = true
			dest := destNodeIDFromCIDR(setRoute.GetTo())
			window, ok := windows[dest]
			if !ok {
				t.Errorf("unexpected SetRoute destination %s", dest)
				continue
			}
			when := createEntry.GetTime()
			if when != nil {
				whenTime := when.AsTime()
				if !whenTime.Equal(window.StartTime) {
					t.Errorf("SetRoute When = %v, expected %v", whenTime, window.StartTime)
				}
			}
		}

		if deleteRoute != nil {
			deleteRouteFound = true
			dest := destNodeIDFromCIDR(deleteRoute.GetTo())
			window, ok := windows[dest]
			if !ok {
				t.Errorf("unexpected DeleteRoute destination %s", dest)
				continue
			}
			when := createEntry.GetTime()
			if when != nil {
				whenTime := when.AsTime()
				if !whenTime.Equal(window.EndTime) {
					t.Errorf("DeleteRoute When = %v, expected %v", whenTime, window.EndTime)
				}
			}
		}
	}

	if !setRouteFound {
		t.Errorf("SetRoute action not found for expected destinations %v", expectedDestNodeIDs)
	}
	if !deleteRouteFound {
		t.Errorf("DeleteRoute action not found for expected destinations %v", expectedDestNodeIDs)
	}
}

func destNodeIDFromCIDR(cidr string) string {
	const prefix = "node:"
	if !strings.HasPrefix(cidr, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(cidr, prefix)
	if idx := strings.Index(rest, "/"); idx >= 0 {
		return rest[:idx]
	}
	return rest
}
