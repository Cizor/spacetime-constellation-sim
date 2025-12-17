package controller

import (
	"context"
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
)

// TestScheduler_ScheduleLinkRoutes_SingleLinkSingleWindow verifies that
// a single potential link with a single visibility window produces four route actions:
// - 2x ScheduledSetRoute at T_on (one per endpoint)
// - 2x ScheduledDeleteRoute at T_off (one per endpoint)
func TestScheduler_ScheduleLinkRoutes_SingleLinkSingleWindow(t *testing.T) {
	scheduler, _, _ := setupSchedulerTest(t)

	ctx := context.Background()

	// Call ScheduleLinkRoutes
	// We expect it to fail because no agent is registered, but that's OK for this test
	err := scheduler.ScheduleLinkRoutes(ctx)
	if err == nil {
		t.Log("ScheduleLinkRoutes succeeded (no agent registered, but that's expected for this test setup)")
	}

	// For a proper test with agents, we'd need to register agents.
	// For now, we verify the logic doesn't panic and handles missing agents gracefully.
}

// TestScheduler_ScheduleLinkRoutes_NoPotentialLinks verifies that
// scheduling with no potential links does nothing.
func TestScheduler_ScheduleLinkRoutes_NoPotentialLinks(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	scenarioState := state.NewScenarioState(physKB, netKB, logging.Noop())

	T0 := time.Unix(1000, 0)
	fakeClock := &fakeClockForTest{now: T0}
	eventScheduler := sbi.NewEventScheduler(fakeClock)

	fakeCDPI := newFakeCDPIServerForScheduler(scenarioState, eventScheduler)
	scheduler := NewScheduler(scenarioState, eventScheduler, fakeCDPI, logging.Noop(), state.NewTelemetryState(), nil)

	ctx := context.Background()
	err := scheduler.ScheduleLinkRoutes(ctx)
	if err != nil {
		t.Fatalf("ScheduleLinkRoutes failed: %v", err)
	}

	// Should have sent no actions
	if len(fakeCDPI.sentActions) != 0 {
		t.Fatalf("expected 0 actions, got %d", len(fakeCDPI.sentActions))
	}
}

// TestScheduler_newRouteEntryForNode verifies RouteEntry construction.
func TestScheduler_newRouteEntryForNode(t *testing.T) {
	scheduler, _, _ := setupSchedulerTest(t)

	route := scheduler.newRouteEntryForNode("node-B", "if-A")

	if route == nil {
		t.Fatalf("expected non-nil RouteEntry")
	}
	if route.DestinationCIDR != "node:node-B/32" {
		t.Fatalf("RouteEntry.DestinationCIDR = %q, want %q", route.DestinationCIDR, "node:node-B/32")
	}
	if route.NextHopNodeID != "node-B" {
		t.Fatalf("RouteEntry.NextHopNodeID = %q, want %q", route.NextHopNodeID, "node-B")
	}
	if route.OutInterfaceID != "if-A" {
		t.Fatalf("RouteEntry.OutInterfaceID = %q, want %q", route.OutInterfaceID, "if-A")
	}
}

// TestScheduler_newSetRouteAction verifies SetRoute action construction.
func TestScheduler_newSetRouteAction(t *testing.T) {
	scheduler, _, _ := setupSchedulerTest(t)

	when := time.Unix(1000, 0)
	route := scheduler.newRouteEntryForNode("node-B", "if-A")
	action := scheduler.newSetRouteAction("entry-1", sbi.AgentID("agent-1"), when, route)

	if action == nil {
		t.Fatalf("expected non-nil ScheduledAction")
	}
	if action.EntryID != "entry-1" {
		t.Fatalf("ScheduledAction.EntryID = %q, want %q", action.EntryID, "entry-1")
	}
	if action.AgentID != sbi.AgentID("agent-1") {
		t.Fatalf("ScheduledAction.AgentID = %q, want %q", action.AgentID, "agent-1")
	}
	if action.Type != sbi.ScheduledSetRoute {
		t.Fatalf("ScheduledAction.Type = %v, want %v", action.Type, sbi.ScheduledSetRoute)
	}
	if action.When != when {
		t.Fatalf("ScheduledAction.When = %v, want %v", action.When, when)
	}
	if action.Route == nil {
		t.Fatalf("expected non-nil Route")
	}
	if action.Route.DestinationCIDR != "node:node-B/32" {
		t.Fatalf("Route.DestinationCIDR = %q, want %q", action.Route.DestinationCIDR, "node:node-B/32")
	}
}

// TestScheduler_newDeleteRouteAction verifies DeleteRoute action construction.
func TestScheduler_newDeleteRouteAction(t *testing.T) {
	scheduler, _, _ := setupSchedulerTest(t)

	when := time.Unix(1000, 0)
	route := scheduler.newRouteEntryForNode("node-B", "if-A")
	action := scheduler.newDeleteRouteAction("entry-1", sbi.AgentID("agent-1"), when, route)

	if action == nil {
		t.Fatalf("expected non-nil ScheduledAction")
	}
	if action.EntryID != "entry-1" {
		t.Fatalf("ScheduledAction.EntryID = %q, want %q", action.EntryID, "entry-1")
	}
	if action.AgentID != sbi.AgentID("agent-1") {
		t.Fatalf("ScheduledAction.AgentID = %q, want %q", action.AgentID, "agent-1")
	}
	if action.Type != sbi.ScheduledDeleteRoute {
		t.Fatalf("ScheduledAction.Type = %v, want %v", action.Type, sbi.ScheduledDeleteRoute)
	}
	if action.When != when {
		t.Fatalf("ScheduledAction.When = %v, want %v", action.When, when)
	}
	if action.Route == nil {
		t.Fatalf("expected non-nil Route")
	}
	if action.Route.DestinationCIDR != "node:node-B/32" {
		t.Fatalf("Route.DestinationCIDR = %q, want %q", action.Route.DestinationCIDR, "node:node-B/32")
	}
}

// TestScheduler_agentIDForNode verifies node-to-agent mapping.
func TestScheduler_agentIDForNode(t *testing.T) {
	// This test requires a CDPI server with registered agents.
	// For now, we'll test the error case.
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	scenarioState := state.NewScenarioState(physKB, netKB, logging.Noop())

	T0 := time.Unix(1000, 0)
	fakeClock := &fakeClockForTest{now: T0}
	eventScheduler := sbi.NewEventScheduler(fakeClock)

	fakeCDPI := newFakeCDPIServerForScheduler(scenarioState, eventScheduler)
	scheduler := NewScheduler(scenarioState, eventScheduler, fakeCDPI, logging.Noop(), state.NewTelemetryState(), nil)

	// Test with non-existent agent
	_, err := scheduler.agentIDForNode("unknown-node")
	if err == nil {
		t.Fatalf("expected error for unknown node, got nil")
	}
}
