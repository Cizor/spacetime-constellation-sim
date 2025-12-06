package state

import (
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

type metricsSnapshot struct {
	platforms       int
	nodes           int
	links           int
	serviceRequests int
}

type stubMetricsRecorder struct {
	records []metricsSnapshot
}

func (r *stubMetricsRecorder) SetScenarioCounts(platforms, nodes, links, serviceRequests int) {
	r.records = append(r.records, metricsSnapshot{
		platforms:       platforms,
		nodes:           nodes,
		links:           links,
		serviceRequests: serviceRequests,
	})
}

func (r *stubMetricsRecorder) last() metricsSnapshot {
	if len(r.records) == 0 {
		return metricsSnapshot{}
	}
	return r.records[len(r.records)-1]
}

func TestScenarioStateMetricsRecorder(t *testing.T) {
	recorder := &stubMetricsRecorder{}
	state := NewScenarioState(kb.NewKnowledgeBase(), network.NewKnowledgeBase(), logging.Noop(), WithMetricsRecorder(recorder))

	assertCounts(t, recorder.last(), metricsSnapshot{platforms: 0, nodes: 0, links: 0, serviceRequests: 0})

	if err := state.CreatePlatform(&model.PlatformDefinition{ID: "plat-1"}); err != nil {
		t.Fatalf("CreatePlatform: %v", err)
	}
	assertCounts(t, recorder.last(), metricsSnapshot{platforms: 1, nodes: 0, links: 0, serviceRequests: 0})

	if err := state.CreateNode(&model.NetworkNode{ID: "node-a", PlatformID: "plat-1"}, []*network.NetworkInterface{{
		ID:            "node-a/if0",
		ParentNodeID:  "node-a",
		Medium:        network.MediumWired,
		IsOperational: true,
	}}); err != nil {
		t.Fatalf("CreateNode node-a: %v", err)
	}
	assertCounts(t, recorder.last(), metricsSnapshot{platforms: 1, nodes: 1, links: 0, serviceRequests: 0})

	if err := state.CreateNode(&model.NetworkNode{ID: "node-b", PlatformID: "plat-1"}, []*network.NetworkInterface{{
		ID:            "node-b/if0",
		ParentNodeID:  "node-b",
		Medium:        network.MediumWired,
		IsOperational: true,
	}}); err != nil {
		t.Fatalf("CreateNode node-b: %v", err)
	}
	assertCounts(t, recorder.last(), metricsSnapshot{platforms: 1, nodes: 2, links: 0, serviceRequests: 0})

	if err := state.CreateLink(&network.NetworkLink{
		ID:         "link-1",
		InterfaceA: "node-a/if0",
		InterfaceB: "node-b/if0",
		Medium:     network.MediumWired,
	}); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	assertCounts(t, recorder.last(), metricsSnapshot{platforms: 1, nodes: 2, links: 1, serviceRequests: 0})

	if err := state.CreateServiceRequest(&model.ServiceRequest{
		ID:        "sr-1",
		SrcNodeID: "node-a",
		DstNodeID: "node-b",
	}); err != nil {
		t.Fatalf("CreateServiceRequest: %v", err)
	}
	assertCounts(t, recorder.last(), metricsSnapshot{platforms: 1, nodes: 2, links: 1, serviceRequests: 1})

	if err := state.DeleteServiceRequest("sr-1"); err != nil {
		t.Fatalf("DeleteServiceRequest: %v", err)
	}
	assertCounts(t, recorder.last(), metricsSnapshot{platforms: 1, nodes: 2, links: 1, serviceRequests: 0})

	if err := state.DeleteLink("link-1"); err != nil {
		t.Fatalf("DeleteLink: %v", err)
	}
	assertCounts(t, recorder.last(), metricsSnapshot{platforms: 1, nodes: 2, links: 0, serviceRequests: 0})

	if err := state.DeleteNode("node-b"); err != nil {
		t.Fatalf("DeleteNode node-b: %v", err)
	}
	assertCounts(t, recorder.last(), metricsSnapshot{platforms: 1, nodes: 1, links: 0, serviceRequests: 0})

	if err := state.DeleteNode("node-a"); err != nil {
		t.Fatalf("DeleteNode node-a: %v", err)
	}
	assertCounts(t, recorder.last(), metricsSnapshot{platforms: 1, nodes: 0, links: 0, serviceRequests: 0})

	if err := state.DeletePlatform("plat-1"); err != nil {
		t.Fatalf("DeletePlatform: %v", err)
	}
	assertCounts(t, recorder.last(), metricsSnapshot{platforms: 0, nodes: 0, links: 0, serviceRequests: 0})
}

func assertCounts(t *testing.T, got, want metricsSnapshot) {
	t.Helper()
	if got != want {
		t.Fatalf("metrics = %+v, want %+v", got, want)
	}
}
