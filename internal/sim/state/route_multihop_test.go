package state

import (
	"testing"
	"time"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func newRouteStateForTest() *ScenarioState {
	return NewScenarioState(kb.NewKnowledgeBase(), network.NewKnowledgeBase(), logging.Noop())
}

func TestInstallMultiHopRouteAndGetPath(t *testing.T) {
	state := newRouteStateForTest()

	nodeA := &model.NetworkNode{ID: "nodeA"}
	nodeB := &model.NetworkNode{ID: "nodeB"}
	nodeC := &model.NetworkNode{ID: "nodeC"}
	if err := state.CreateNode(nodeA, nil); err != nil {
		t.Fatalf("CreateNode nodeA: %v", err)
	}
	if err := state.CreateNode(nodeB, nil); err != nil {
		t.Fatalf("CreateNode nodeB: %v", err)
	}
	if err := state.CreateNode(nodeC, nil); err != nil {
		t.Fatalf("CreateNode nodeC: %v", err)
	}

	now := time.Now()
	route := model.RouteEntry{
		DestinationCIDR: "10.0.0.0/24",
		NextHopNodeID:   "nodeB",
		OutInterfaceID:  "eth0",
		Path:            []string{"nodeA", "nodeB", "nodeC"},
		Cost:            3,
		ValidUntil:      now.Add(time.Minute),
	}

	if err := state.InstallMultiHopRoute("nodeA", route); err != nil {
		t.Fatalf("InstallMultiHopRoute failed: %v", err)
	}

	path, err := state.GetRoutePath("nodeA", "nodeC")
	if err != nil {
		t.Fatalf("GetRoutePath failed: %v", err)
	}
	expected := []string{"nodeA", "nodeB", "nodeC"}
	for i := range expected {
		if path[i] != expected[i] {
			t.Fatalf("path mismatch: got %v want %v", path, expected)
		}
	}
}

func TestInvalidateExpiredRoutes(t *testing.T) {
	state := newRouteStateForTest()

	node := &model.NetworkNode{ID: "nodeX"}
	if err := state.CreateNode(node, nil); err != nil {
		t.Fatalf("CreateNode nodeX: %v", err)
	}

	routeFresh := model.RouteEntry{
		DestinationCIDR: "192.0.2.0/24",
		NextHopNodeID:   "nodeX",
		OutInterfaceID:  "eth1",
		Path:            []string{"nodeX"},
		Cost:            1,
		ValidUntil:      time.Now().Add(5 * time.Minute),
	}
	if err := state.InstallRoute("nodeX", routeFresh); err != nil {
		t.Fatalf("InstallRoute fresh: %v", err)
	}

	routeExpired := model.RouteEntry{
		DestinationCIDR: "198.51.100.0/24",
		NextHopNodeID:   "nodeX",
		OutInterfaceID:  "eth2",
		Path:            []string{"nodeX"},
		Cost:            2,
		ValidUntil:      time.Now().Add(-time.Minute),
	}
	if err := state.InstallRoute("nodeX", routeExpired); err != nil {
		t.Fatalf("InstallRoute expired: %v", err)
	}

	if err := state.InvalidateExpiredRoutes(time.Now()); err != nil {
		t.Fatalf("InvalidateExpiredRoutes failed: %v", err)
	}

	routes, err := state.GetRoutes("nodeX")
	if err != nil {
		t.Fatalf("GetRoutes failed: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route after invalidation, got %d", len(routes))
	}
	if routes[0].DestinationCIDR != routeFresh.DestinationCIDR {
		t.Fatalf("unexpected remaining route: %+v", routes[0])
	}
}
