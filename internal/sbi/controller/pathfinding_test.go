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
	"github.com/signalsfoundry/constellation-simulator/model"
)

func setupPathfindingScheduler(t *testing.T) (*Scheduler, string, time.Time) {
	t.Helper()

	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)

	trx := &core.TransceiverModel{
		ID: "trx-path",
		Band: core.FrequencyBand{
			MinGHz: 12.0,
			MaxGHz: 18.0,
		},
		MaxRangeKm: 1e6,
	}
	if err := netKB.AddTransceiverModel(trx); err != nil {
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
	if err := scenarioState.CreateNode(nodeA, []*core.NetworkInterface{ifaceA}); err != nil {
		t.Fatalf("CreateNode node-A failed: %v", err)
	}

	nodeB := &model.NetworkNode{ID: "node-B"}
	ifaceB := &core.NetworkInterface{
		ID:            "node-B/if0",
		ParentNodeID:  nodeB.ID,
		Medium:        core.MediumWireless,
		TransceiverID: trx.ID,
		IsOperational: true,
	}
	if err := scenarioState.CreateNode(nodeB, []*core.NetworkInterface{ifaceB}); err != nil {
		t.Fatalf("CreateNode node-B failed: %v", err)
	}

	link := &core.NetworkLink{
		ID:         "link-ab",
		InterfaceA: ifaceA.ID,
		InterfaceB: ifaceB.ID,
		Medium:     core.MediumWireless,
		Status:     core.LinkStatusPotential,
	}
	if err := scenarioState.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	now := time.Unix(1_000, 0)
	clock := sbi.NewFakeEventScheduler(now)
	scheduler := NewScheduler(scenarioState, clock, newFakeCDPIServerForScheduler(scenarioState, clock), log)
	return scheduler, link.ID, now
}

func TestBuildTimeExpandedGraph(t *testing.T) {
	scheduler, linkID, now := setupPathfindingScheduler(t)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkID: {
			{
				StartTime: now.Add(1 * time.Minute),
				EndTime:   now.Add(5 * time.Minute),
				Quality:   0,
			},
		},
	}

	graph, err := scheduler.BuildTimeExpandedGraph(context.Background(), "node-A", "node-B", now, now.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("BuildTimeExpandedGraph failed: %v", err)
	}

	if len(graph.Nodes) < 4 {
		t.Fatalf("expected nodes for both endpoints at start and end, got %d", len(graph.Nodes))
	}

	linkEdges := 0
	waitEdges := 0
	for _, edge := range graph.Edges {
		if edge.LinkID == "" {
			waitEdges++
			continue
		}
		if edge.LinkID != linkID {
			t.Fatalf("unexpected link ID %s", edge.LinkID)
		}
		if edge.Cost <= 0 {
			t.Fatalf("link edge cost must be positive")
		}
		linkEdges++
	}

	if linkEdges != 2 {
		t.Fatalf("expected 2 link edges (one per direction), got %d", linkEdges)
	}
	if waitEdges < 2 {
		t.Fatalf("expected wait edges for each node, got %d", waitEdges)
	}
}
