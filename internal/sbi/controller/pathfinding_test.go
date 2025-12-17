package controller

import (
	"context"
	"fmt"
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
	telemetryState := state.NewTelemetryState()

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
	scheduler := NewScheduler(scenarioState, clock, newFakeCDPIServerForScheduler(scenarioState, clock), log, telemetryState, nil)
	return scheduler, link.ID, now
}

func setupLinearScheduler(t *testing.T, nodeCount int, storage map[int]uint64) (*Scheduler, []string, time.Time) {
	t.Helper()
	if nodeCount < 2 {
		t.Fatalf("nodeCount must be >= 2")
	}

	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	telemetryState := state.NewTelemetryState()

	trx := &core.TransceiverModel{
		ID: "trx-linear",
		Band: core.FrequencyBand{
			MinGHz: 12.0,
			MaxGHz: 18.0,
		},
		MaxRangeKm: 1e6,
	}
	if err := netKB.AddTransceiverModel(trx); err != nil {
		t.Fatalf("AddTransceiverModel failed: %v", err)
	}

	interfaces := make([]*core.NetworkInterface, 0, nodeCount)
	for idx := 0; idx < nodeCount; idx++ {
		id := fmt.Sprintf("node-%d", idx)
		node := &model.NetworkNode{ID: id}
		if cap, ok := storage[idx]; ok && cap > 0 {
			node.StorageCapacityBytes = float64(cap)
		}
		iface := &core.NetworkInterface{
			ID:            fmt.Sprintf("%s/if0", id),
			ParentNodeID:  id,
			Medium:        core.MediumWireless,
			TransceiverID: trx.ID,
			IsOperational: true,
		}
		if err := scenarioState.CreateNode(node, []*core.NetworkInterface{iface}); err != nil {
			t.Fatalf("CreateNode %s failed: %v", id, err)
		}
		interfaces = append(interfaces, iface)
	}

	linkIDs := make([]string, 0, nodeCount-1)
	for idx := 0; idx < len(interfaces)-1; idx++ {
		linkID := fmt.Sprintf("link-%d-%d", idx, idx+1)
		link := &core.NetworkLink{
			ID:         linkID,
			InterfaceA: interfaces[idx].ID,
			InterfaceB: interfaces[idx+1].ID,
			Medium:     core.MediumWireless,
			Status:     core.LinkStatusPotential,
		}
		if err := scenarioState.CreateLink(link); err != nil {
			t.Fatalf("CreateLink %s failed: %v", linkID, err)
		}
		linkIDs = append(linkIDs, linkID)
	}

	now := time.Unix(1_000, 0)
	clock := sbi.NewFakeEventScheduler(now)
	scheduler := NewScheduler(scenarioState, clock, newFakeCDPIServerForScheduler(scenarioState, clock), log, telemetryState, nil)
	return scheduler, linkIDs, now
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

func TestFindMultiHopPath(t *testing.T) {
	scheduler, linkID, now := setupPathfindingScheduler(t)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkID: {
			{
				StartTime: now.Add(1 * time.Minute),
				EndTime:   now.Add(4 * time.Minute),
				Quality:   0,
			},
		},
	}

	path, err := scheduler.FindMultiHopPath(context.Background(), "node-A", "node-B", now, 10*time.Minute)
	if err != nil {
		t.Fatalf("FindMultiHopPath failed: %v", err)
	}
	if path == nil || len(path.Hops) != 1 {
		t.Fatalf("expected single hop path, got %+v", path)
	}
	if path.TotalLatency != 3*time.Minute {
		t.Fatalf("expected latency 3m, got %v", path.TotalLatency)
	}
	hop := path.Hops[0]
	if hop.FromNodeID != "node-A" || hop.ToNodeID != "node-B" {
		t.Fatalf("unexpected hop %+v", hop)
	}
	if hop.LinkID != linkID {
		t.Fatalf("unexpected link ID %s", hop.LinkID)
	}
}

func TestFindMultiHopPathNoPath(t *testing.T) {
	scheduler, _, now := setupPathfindingScheduler(t)
	scheduler.contactWindows = map[string][]ContactWindow{}
	_, err := scheduler.FindMultiHopPath(context.Background(), "node-A", "node-B", now, 5*time.Minute)
	if err == nil {
		t.Fatalf("expected error when no path exists")
	}
}

func TestFindDTNPathDirect(t *testing.T) {
	scheduler, linkID, now := setupPathfindingScheduler(t)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkID: {
			{
				StartTime: now.Add(1 * time.Minute),
				EndTime:   now.Add(4 * time.Minute),
				Quality:   0,
			},
		},
	}

	path, err := scheduler.FindDTNPath(context.Background(), "node-A", "node-B", 100, now)
	if err != nil {
		t.Fatalf("FindDTNPath failed: %v", err)
	}
	if path == nil || len(path.Hops) != 1 {
		t.Fatalf("expected single hop path, got %+v", path)
	}
	if len(path.StorageNodes) != 0 {
		t.Fatalf("expected no storage nodes, got %v", path.StorageNodes)
	}
	hop := path.Hops[0]
	if hop.FromNodeID != "node-A" || hop.ToNodeID != "node-B" {
		t.Fatalf("unexpected hop %+v", hop)
	}
	if hop.StorageAt != "" {
		t.Fatalf("expected no storage for direct hop, got %s", hop.StorageAt)
	}
}

func TestFindDTNPathStoreAndForward(t *testing.T) {
	scheduler, linkIDs, now := setupThreeNodeScheduler(t, 500)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkIDs["linkAB"]: {
			{
				StartTime: now.Add(1 * time.Minute),
				EndTime:   now.Add(2 * time.Minute),
				Quality:   0,
			},
		},
		linkIDs["linkBC"]: {
			{
				StartTime: now.Add(5 * time.Minute),
				EndTime:   now.Add(6 * time.Minute),
				Quality:   0,
			},
		},
	}

	path, err := scheduler.FindDTNPath(context.Background(), "node-A", "node-C", 100, now)
	if err != nil {
		t.Fatalf("FindDTNPath store-and-forward failed: %v", err)
	}
	if len(path.Hops) != 2 {
		t.Fatalf("expected two hops, got %+v", path.Hops)
	}
	if len(path.StorageNodes) != 1 || path.StorageNodes[0] != "node-B" {
		t.Fatalf("expected storage at node-B, got %v", path.StorageNodes)
	}
	secondHop := path.Hops[1]
	if secondHop.StorageAt != "node-B" {
		t.Fatalf("expected storage before second hop, got %s", secondHop.StorageAt)
	}
	if secondHop.StorageDuration < 2*time.Minute {
		t.Fatalf("expected wait at node-B, got %v", secondHop.StorageDuration)
	}
	if path.TotalDelay < 5*time.Minute {
		t.Fatalf("expected total delay at least 5m, got %v", path.TotalDelay)
	}
}

func TestFindMultiHopPathFiveHops(t *testing.T) {
	scheduler, linkIDs, now := setupLinearScheduler(t, 6, nil)
	scheduler.contactWindows = make(map[string][]ContactWindow, len(linkIDs))
	for idx, linkID := range linkIDs {
		start := now.Add(time.Duration(idx*3+1) * time.Minute)
		scheduler.contactWindows[linkID] = []ContactWindow{
			{
				StartTime: start,
				EndTime:   start.Add(2 * time.Minute),
				Quality:   0,
			},
		}
	}

	horizon := time.Duration(len(linkIDs))*3*time.Minute + 2*time.Minute
	path, err := scheduler.FindMultiHopPath(context.Background(), "node-0", "node-5", now, horizon)
	if err != nil {
		t.Fatalf("FindMultiHopPath failed: %v", err)
	}
	if len(path.Hops) != len(linkIDs) {
		t.Fatalf("expected %d hops, got %d", len(linkIDs), len(path.Hops))
	}
	expectedLatency := time.Duration(len(linkIDs)) * 2 * time.Minute
	if path.TotalLatency != expectedLatency {
		t.Fatalf("expected latency %v, got %v", expectedLatency, path.TotalLatency)
	}
}

func TestFindDTNPathMultipleStorageHops(t *testing.T) {
	storage := map[int]uint64{1: 1024, 2: 1024}
	scheduler, linkIDs, now := setupLinearScheduler(t, 4, storage)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkIDs[0]: {
			{StartTime: now.Add(1 * time.Minute), EndTime: now.Add(2 * time.Minute), Quality: 0},
		},
		linkIDs[1]: {
			{StartTime: now.Add(5 * time.Minute), EndTime: now.Add(6 * time.Minute), Quality: 0},
		},
		linkIDs[2]: {
			{StartTime: now.Add(10 * time.Minute), EndTime: now.Add(11 * time.Minute), Quality: 0},
		},
	}

	path, err := scheduler.FindDTNPath(context.Background(), "node-0", "node-3", 100, now)
	if err != nil {
		t.Fatalf("FindDTNPath with multiple storage hops failed: %v", err)
	}
	if len(path.Hops) != 3 {
		t.Fatalf("expected 3 hops, got %d", len(path.Hops))
	}
	if len(path.StorageNodes) != 2 {
		t.Fatalf("expected storage at 2 nodes, got %v", path.StorageNodes)
	}
	expectedNodes := map[string]struct{}{"node-1": {}, "node-2": {}}
	for _, node := range path.StorageNodes {
		delete(expectedNodes, node)
	}
	if len(expectedNodes) != 0 {
		t.Fatalf("storage nodes missing: %v", expectedNodes)
	}
	if path.TotalDelay < 10*time.Minute {
		t.Fatalf("expected delay >= 10m, got %v", path.TotalDelay)
	}
}

func TestFindDTNPathInsufficientStorage(t *testing.T) {
	scheduler, linkIDs, now := setupThreeNodeScheduler(t, 50)
	scheduler.contactWindows = map[string][]ContactWindow{
		linkIDs["linkAB"]: {
			{
				StartTime: now.Add(1 * time.Minute),
				EndTime:   now.Add(2 * time.Minute),
				Quality:   0,
			},
		},
		linkIDs["linkBC"]: {
			{
				StartTime: now.Add(5 * time.Minute),
				EndTime:   now.Add(6 * time.Minute),
				Quality:   0,
			},
		},
	}

	_, err := scheduler.FindDTNPath(context.Background(), "node-A", "node-C", 100, now)
	if err == nil {
		t.Fatalf("expected error when storage capacity insufficient")
	}
}

func setupThreeNodeScheduler(t *testing.T, storageCapacity uint64) (*Scheduler, map[string]string, time.Time) {
	t.Helper()

	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	telemetryState := state.NewTelemetryState()

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

	nodeB := &model.NetworkNode{ID: "node-B", StorageCapacityBytes: float64(storageCapacity)}
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

	nodeC := &model.NetworkNode{ID: "node-C"}
	ifaceC := &core.NetworkInterface{
		ID:            "node-C/if0",
		ParentNodeID:  nodeC.ID,
		Medium:        core.MediumWireless,
		TransceiverID: trx.ID,
		IsOperational: true,
	}
	if err := scenarioState.CreateNode(nodeC, []*core.NetworkInterface{ifaceC}); err != nil {
		t.Fatalf("CreateNode node-C failed: %v", err)
	}

	linkAB := &core.NetworkLink{
		ID:         "link-ab",
		InterfaceA: ifaceA.ID,
		InterfaceB: ifaceB.ID,
		Medium:     core.MediumWireless,
		Status:     core.LinkStatusPotential,
	}
	if err := scenarioState.CreateLink(linkAB); err != nil {
		t.Fatalf("CreateLink AB failed: %v", err)
	}

	linkBC := &core.NetworkLink{
		ID:         "link-bc",
		InterfaceA: ifaceB.ID,
		InterfaceB: ifaceC.ID,
		Medium:     core.MediumWireless,
		Status:     core.LinkStatusPotential,
	}
	if err := scenarioState.CreateLink(linkBC); err != nil {
		t.Fatalf("CreateLink BC failed: %v", err)
	}

	now := time.Unix(1_000, 0)
	clock := sbi.NewFakeEventScheduler(now)
	scheduler := NewScheduler(scenarioState, clock, newFakeCDPIServerForScheduler(scenarioState, clock), log, telemetryState, nil)
	return scheduler, map[string]string{
		"linkAB": linkAB.ID,
		"linkBC": linkBC.ID,
	}, now
}
