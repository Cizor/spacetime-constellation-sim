package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestComputePathDiff(t *testing.T) {
	scheduler, _, _ := setupSchedulerTest(t)
	now := time.Unix(1000, 0)
	hopA := PathHop{
		FromNodeID: "node-A",
		ToNodeID:   "node-B",
		LinkID:     "link-ab",
		StartTime:  now,
		EndTime:    now.Add(30 * time.Second),
	}
	hopB := PathHop{
		FromNodeID: "node-B",
		ToNodeID:   "node-C",
		LinkID:     "link-bc",
		StartTime:  now.Add(31 * time.Second),
		EndTime:    now.Add(60 * time.Second),
	}
	hopD := PathHop{
		FromNodeID: "node-B",
		ToNodeID:   "node-D",
		LinkID:     "link-bd",
		StartTime:  now.Add(31 * time.Second),
		EndTime:    now.Add(65 * time.Second),
	}
	oldPath := &Path{Hops: []PathHop{hopA, hopB}}
	newPath := &Path{Hops: []PathHop{hopA, hopD}}

	diff := scheduler.ComputePathDiff(oldPath, newPath)
	if len(diff.SharedHops) != 1 {
		t.Fatalf("shared hops = %d, want 1", len(diff.SharedHops))
	}
	if len(diff.RemovedHops) != 1 || diff.RemovedHops[0] != hopB {
		t.Fatalf("removed hops = %v, want [%v]", diff.RemovedHops, hopB)
	}
	if len(diff.AddedHops) != 1 || diff.AddedHops[0] != hopD {
		t.Fatalf("added hops = %v, want [%v]", diff.AddedHops, hopD)
	}
}

func TestUpdatePathAppendsHop(t *testing.T) {
	scheduler, fakeCDPI, now := setupSchedulerTest(t)

	platformC := &model.PlatformDefinition{ID: "platform-C", Name: "Platform C"}
	if err := scheduler.State.CreatePlatform(platformC); err != nil {
		t.Fatalf("CreatePlatform(platform-C) failed: %v", err)
	}
	if err := scheduler.State.CreateNode(&model.NetworkNode{
		ID:         "node-C",
		Name:       "Node C",
		PlatformID: "platform-C",
	}, []*core.NetworkInterface{{
		ID:            "if-C",
		Name:          "Interface C",
		Medium:        core.MediumWireless,
		ParentNodeID:  "node-C",
		IsOperational: true,
		TransceiverID: "trx-A",
	}}); err != nil {
		t.Fatalf("CreateNode(node-C) failed: %v", err)
	}

	linkBC := &core.NetworkLink{
		ID:         "link-bc",
		InterfaceA: "if-B",
		InterfaceB: "if-C",
		Medium:     core.MediumWireless,
		Status:     core.LinkStatusPotential,
	}
	if err := scheduler.State.CreateLink(linkBC); err != nil {
		t.Fatalf("CreateLink(link-bc) failed: %v", err)
	}

	registerAgent(t, fakeCDPI, "node-A")
	registerAgent(t, fakeCDPI, "node-B")

	scheduler.contactWindows = map[string][]ContactWindow{
		"link-ab": {
			{LinkID: "link-ab", StartTime: now.Add(-time.Minute), EndTime: now.Add(2 * time.Minute), Quality: 20},
		},
		"link-bc": {
			{LinkID: "link-bc", StartTime: now.Add(-time.Minute), EndTime: now.Add(2 * time.Minute), Quality: 20},
		},
	}

	sr := &model.ServiceRequest{
		ID:        "sr-inc",
		SrcNodeID: "node-A",
		DstNodeID: "node-B",
		Priority:  1,
	}
	if err := scheduler.State.CreateServiceRequest(sr); err != nil {
		t.Fatalf("CreateServiceRequest failed: %v", err)
	}
	sr.IsProvisionedNow = true
	if err := scheduler.State.UpdateServiceRequest(sr); err != nil {
		t.Fatalf("UpdateServiceRequest failed: %v", err)
	}

	oldHop := PathHop{
		FromNodeID: "node-A",
		ToNodeID:   "node-B",
		LinkID:     "link-ab",
		StartTime:  now,
		EndTime:    now.Add(30 * time.Second),
	}
	oldPath := &Path{
		Hops:       []PathHop{oldHop},
		ValidFrom:  oldHop.StartTime,
		ValidUntil: oldHop.EndTime,
	}
	oldEntries := []scheduledEntryRef{
		{entryID: "sr-inc:hop:0:beam:node-A->node-B:0", agentID: "node-A", hopIdx: 0},
		{entryID: "sr-inc:hop:0:beam:node-A->node-B:off:1", agentID: "node-A", hopIdx: 0},
		{entryID: "sr-inc:hop:0:route:node-A->node-B:0", agentID: "node-A", hopIdx: 0},
		{entryID: "sr-inc:hop:0:route:node-A->node-B:off:1", agentID: "node-A", hopIdx: 0},
	}
	for _, entry := range oldEntries {
		scheduler.scheduledEntryIDs[entry.entryID] = true
	}
	scheduler.activePaths[sr.ID] = &ActivePath{
		ServiceRequestID: sr.ID,
		Path:             oldPath,
		HopEntries: map[int][]scheduledEntryRef{
			0: oldEntries,
		},
		ScheduledActions: oldEntriesIDs(oldEntries),
		LastUpdated:      now,
		Health:           HealthHealthy,
	}
	scheduler.srEntries[sr.ID] = append([]scheduledEntryRef(nil), oldEntries...)

	required := scheduler.requiredBandwidthForSR(sr)
	if err := scheduler.State.ReserveBandwidth("link-ab", required); err != nil {
		t.Fatalf("ReserveBandwidth failed: %v", err)
	}
	scheduler.bandwidthReservations[sr.ID] = map[string]uint64{"link-ab": required}

	newHop := PathHop{
		FromNodeID: "node-B",
		ToNodeID:   "node-C",
		LinkID:     "link-bc",
		StartTime:  now.Add(5 * time.Second),
		EndTime:    now.Add(40 * time.Second),
	}
	newPath := &Path{
		Hops:       []PathHop{oldHop, newHop},
		ValidFrom:  oldHop.StartTime,
		ValidUntil: newHop.EndTime,
	}

	if err := scheduler.UpdatePath(context.Background(), sr.ID, newPath); err != nil {
		t.Fatalf("UpdatePath failed: %v", err)
	}

	ap := scheduler.activePaths[sr.ID]
	if ap == nil {
		t.Fatalf("active path missing after update")
	}
	if len(ap.Path.Hops) != 2 {
		t.Fatalf("expected 2 hops, got %d", len(ap.Path.Hops))
	}
	if len(ap.HopEntries[1]) != 4 {
		t.Fatalf("expected 4 entries for new hop, got %d", len(ap.HopEntries[1]))
	}
	if _, ok := scheduler.bandwidthReservations[sr.ID]["link-bc"]; !ok {
		t.Fatalf("expected bandwidth reserved for link-bc")
	}
	if len(scheduler.srEntries[sr.ID]) != len(oldEntries)+4 {
		t.Fatalf("expected %d total entries, got %d", len(oldEntries)+4, len(scheduler.srEntries[sr.ID]))
	}

	expectedRoutes := map[string]struct{}{
		fmt.Sprintf("sr:sr-inc:hop:1:route:node-B->node-C:%d", newHop.StartTime.UnixNano()):   {},
		fmt.Sprintf("sr:sr-inc:hop:1:route:node-B->node-C:off:%d", newHop.EndTime.UnixNano()): {},
	}
	for _, action := range fakeCDPI.sentActions {
		if _, ok := expectedRoutes[action.action.EntryID]; ok {
			delete(expectedRoutes, action.action.EntryID)
		}
	}
	if len(expectedRoutes) != 0 {
		t.Fatalf("expected route actions recorded for %v, but missing %v", fakeCDPI.sentActions, expectedRoutes)
	}
}

func oldEntriesIDs(entries []scheduledEntryRef) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.entryID != "" {
			ids = append(ids, entry.entryID)
		}
	}
	return ids
}

func registerAgent(t *testing.T, fakeCDPI *fakeCDPIServerForScheduler, nodeID string) {
	t.Helper()
	handle := &AgentHandle{
		AgentID:  nodeID,
		NodeID:   nodeID,
		Stream:   &fakeStream{},
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 10),
	}
	fakeCDPI.agentsMu.Lock()
	fakeCDPI.agents[nodeID] = handle
	fakeCDPI.agentsMu.Unlock()
}
