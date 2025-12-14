package controller

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

func setupContactWindowScheduler(t *testing.T) (*Scheduler, string, time.Time) {
	t.Helper()

	phys := kb.NewKnowledgeBase()
	net := core.NewKnowledgeBase()
	log := logging.Noop()
	state := state.NewScenarioState(phys, net, log)

	trx := &core.TransceiverModel{
		ID: "trx-test",
		Band: core.FrequencyBand{
			MinGHz: 12.0,
			MaxGHz: 18.0,
		},
		MaxRangeKm: 1e6,
	}
	if err := net.AddTransceiverModel(trx); err != nil {
		t.Fatalf("AddTransceiverModel failed: %v", err)
	}

	nodeA := &model.NetworkNode{ID: "node-A"}
	ifaceA := &core.NetworkInterface{
		ID:            "node-A/if0",
		ParentNodeID:  nodeA.ID,
		Medium:        core.MediumWireless,
		TransceiverID: trx.ID,
		IsOperational: true,
	}
	if err := state.CreateNode(nodeA, []*core.NetworkInterface{ifaceA}); err != nil {
		t.Fatalf("CreateNode(node-A) failed: %v", err)
	}

	nodeB := &model.NetworkNode{ID: "node-B"}
	ifaceB := &core.NetworkInterface{
		ID:            "node-B/if0",
		ParentNodeID:  nodeB.ID,
		Medium:        core.MediumWireless,
		TransceiverID: trx.ID,
		IsOperational: true,
	}
	if err := state.CreateNode(nodeB, []*core.NetworkInterface{ifaceB}); err != nil {
		t.Fatalf("CreateNode(node-B) failed: %v", err)
	}

	link := &core.NetworkLink{
		ID:         "link-ab",
		InterfaceA: ifaceA.ID,
		InterfaceB: ifaceB.ID,
		Medium:     core.MediumWireless,
		Status:     core.LinkStatusPotential,
		SNRdB:      12.3,
	}
	if err := state.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	now := time.Unix(1_000, 0)
	scheduler := &Scheduler{
		State:          state,
		Clock:          sbi.NewFakeEventScheduler(now),
		log:            logging.Noop(),
		contactWindows: make(map[string][]ContactWindow),
	}

	return scheduler, link.ID, now
}

func TestScheduler_GetContactPlanHorizonFiltering(t *testing.T) {
	scheduler, linkID, now := setupContactWindowScheduler(t)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkID: {
			{StartTime: now.Add(10 * time.Minute), EndTime: now.Add(20 * time.Minute), Quality: 4.2},
			{StartTime: now.Add(2 * time.Hour), EndTime: now.Add(3 * time.Hour), Quality: 1.1},
		},
	}

	plan := scheduler.GetContactPlan(linkID, 30*time.Minute)
	if len(plan) != 1 {
		t.Fatalf("expected 1 window within horizon, got %d", len(plan))
	}
	if plan[0].Quality != 4.2 {
		t.Fatalf("expected quality 4.2, got %v", plan[0].Quality)
	}

	allPlan := scheduler.GetContactPlan(linkID, 0)
	if len(allPlan) != 2 {
		t.Fatalf("expected 2 windows when horizon is 0, got %d", len(allPlan))
	}
}

func TestScheduler_GetContactPlanSortsByStartTime(t *testing.T) {
	scheduler, linkID, now := setupContactWindowScheduler(t)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkID: {
			{StartTime: now.Add(2 * time.Hour), EndTime: now.Add(3 * time.Hour), Quality: 2.1},
			{StartTime: now.Add(5 * time.Minute), EndTime: now.Add(15 * time.Minute), Quality: 7.7},
		},
	}

	plan := scheduler.GetContactPlan(linkID, 0)
	if len(plan) != 2 {
		t.Fatalf("expected 2 windows, got %d", len(plan))
	}
	if plan[0].StartTime.After(plan[1].StartTime) {
		t.Fatalf("expected plan sorted by StartTime, got %v then %v", plan[0].StartTime, plan[1].StartTime)
	}
	if plan[0].Quality != 7.7 {
		t.Fatalf("expected first window quality 7.7, got %v", plan[0].Quality)
	}
}

func TestScheduler_GetContactPlansForNode(t *testing.T) {
	scheduler, linkID, now := setupContactWindowScheduler(t)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkID: {
			{StartTime: now.Add(10 * time.Minute), EndTime: now.Add(20 * time.Minute), Quality: 3.3},
		},
	}

	plans := scheduler.GetContactPlansForNode("node-A", 0)
	if len(plans) != 1 {
		t.Fatalf("expected plan for node-A, got %d entries", len(plans))
	}
	plan, ok := plans[linkID]
	if !ok {
		t.Fatalf("expected plan for link %s", linkID)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 window in plan, got %d", len(plan))
	}

	shortPlans := scheduler.GetContactPlansForNode("node-A", 5*time.Minute)
	if len(shortPlans) != 0 {
		t.Fatalf("expected no plan when horizon shorter than window, got %d entries", len(shortPlans))
	}
}
