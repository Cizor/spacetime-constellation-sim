package state

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

// TestSimLoopAndNBIConcurrency exercises the sim tick loop running alongside
// concurrent NBI-style CRUD operations to verify we stay race-free.
func TestSimLoopAndNBIConcurrency(t *testing.T) {
	state, phys, netKB := newScenarioStateForTest()
	motion := network.NewMotionModel(network.WithPositionUpdater(phys))
	connectivity := network.NewConnectivityService(netKB)

	addWirelessTransceiver(t, netKB, "trx-ku")

	// Seed a stable pair of platforms/nodes/interfaces so connectivity work has
	// something meaningful to chew on.
	platA := &model.PlatformDefinition{
		ID:          "p-stable-a",
		Coordinates: model.Motion{X: 6371000, Y: 0, Z: 0},
	}
	platB := &model.PlatformDefinition{
		ID:          "p-stable-b",
		Coordinates: model.Motion{X: 6371000, Y: 100000, Z: 0},
	}
	if err := state.CreatePlatform(platA); err != nil {
		t.Fatalf("CreatePlatform platA: %v", err)
	}
	if err := state.CreatePlatform(platB); err != nil {
		t.Fatalf("CreatePlatform platB: %v", err)
	}

	if err := state.CreateNode(&model.NetworkNode{
		ID:         "node-stable-a",
		PlatformID: platA.ID,
	}, []*network.NetworkInterface{{
		ID:            "node-stable-a/if0",
		ParentNodeID:  "node-stable-a",
		Medium:        network.MediumWireless,
		TransceiverID: "trx-ku",
		IsOperational: true,
	}}); err != nil {
		t.Fatalf("CreateNode node-stable-a: %v", err)
	}
	if err := state.CreateNode(&model.NetworkNode{
		ID:         "node-stable-b",
		PlatformID: platB.ID,
	}, []*network.NetworkInterface{{
		ID:            "node-stable-b/if0",
		ParentNodeID:  "node-stable-b",
		Medium:        network.MediumWireless,
		TransceiverID: "trx-ku",
		IsOperational: true,
	}}); err != nil {
		t.Fatalf("CreateNode node-stable-b: %v", err)
	}

	if err := state.CreateLink(&network.NetworkLink{
		ID:         "link-stable",
		InterfaceA: "node-stable-a/if0",
		InterfaceB: "node-stable-b/if0",
		Medium:     network.MediumWireless,
	}); err != nil {
		t.Fatalf("CreateLink link-stable: %v", err)
	}

	// Initial node positions derived from platform coordinates.
	pushNodePositions(phys, netKB)

	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()

	var tickWG sync.WaitGroup
	tickWG.Add(1)
	go func() {
		defer tickWG.Done()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if err := state.RunSimTick(now, motion, connectivity, func() {
					pushNodePositions(phys, netKB)
				}); err != nil {
					// Non-fatal so we continue exercising concurrency.
					t.Logf("RunSimTick error: %v", err)
				}
			}
		}
	}()

	var workers sync.WaitGroup
	runWorker(ctx, &workers, func(iter int) { exercisePlatforms(state, iter) })
	runWorker(ctx, &workers, func(iter int) { exerciseNodes(state, platA.ID, iter) })
	runWorker(ctx, &workers, func(iter int) { exerciseLinks(state, iter) })
	runWorker(ctx, &workers, func(iter int) { exerciseServiceRequests(state, iter) })

	workers.Wait()
	tickWG.Wait()
}

func addWirelessTransceiver(t *testing.T, kb *network.KnowledgeBase, id string) {
	t.Helper()
	trx := &network.TransceiverModel{
		ID:   id,
		Band: network.FrequencyBand{MinGHz: 10.7, MaxGHz: 12.75},
		// Large range to keep links up during the test regardless of small motions.
		MaxRangeKm: 80000,
	}
	if err := kb.AddTransceiverModel(trx); err != nil {
		t.Fatalf("AddTransceiverModel(%q) failed: %v", id, err)
	}
}

func pushNodePositions(phys *kb.KnowledgeBase, netKB *network.KnowledgeBase) {
	platforms := phys.ListPlatforms()
	platformByID := make(map[string]*model.PlatformDefinition, len(platforms))
	for _, p := range platforms {
		if p == nil {
			continue
		}
		platformByID[p.ID] = p
	}

	for _, node := range phys.ListNetworkNodes() {
		if node == nil {
			continue
		}
		if p := platformByID[node.PlatformID]; p != nil {
			netKB.SetNodeECEFPosition(node.ID, network.Vec3{
				X: p.Coordinates.X / 1000.0,
				Y: p.Coordinates.Y / 1000.0,
				Z: p.Coordinates.Z / 1000.0,
			})
		}
	}
}

func runWorker(ctx context.Context, wg *sync.WaitGroup, fn func(iter int)) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		for iter := 0; ; iter++ {
			select {
			case <-ctx.Done():
				return
			default:
				fn(iter)
				time.Sleep(time.Duration(rand.Intn(5)+1) * time.Millisecond)
			}
		}
	}()
}

var platformSeq uint64

func exercisePlatforms(state *ScenarioState, iter int) {
	id := fmt.Sprintf("p-dyn-%d", atomic.AddUint64(&platformSeq, 1))
	pd := &model.PlatformDefinition{
		ID:          id,
		Name:        "dynamic",
		Coordinates: model.Motion{X: float64(iter), Y: float64(iter)},
	}
	if err := state.CreatePlatform(pd); err == nil {
		_ = state.UpdatePlatform(&model.PlatformDefinition{
			ID:          id,
			Name:        "dynamic-updated",
			Coordinates: model.Motion{X: float64(iter + 1), Y: float64(iter + 2)},
		})
		_ = state.DeletePlatform(id)
	}
}

var nodeSeq uint64

func exerciseNodes(state *ScenarioState, platformID string, iter int) {
	id := fmt.Sprintf("node-dyn-%d", atomic.AddUint64(&nodeSeq, 1))
	ifaceID := id + "/if0"

	node := &model.NetworkNode{
		ID:         id,
		Name:       "dynamic-node",
		PlatformID: platformID,
	}
	iface := &network.NetworkInterface{
		ID:            ifaceID,
		ParentNodeID:  id,
		Medium:        network.MediumWired,
		IsOperational: true,
	}

	if err := state.CreateNode(node, []*network.NetworkInterface{iface}); err == nil {
		iface.LinkIDs = nil // keep replace safe
		_ = state.UpdateNode(&model.NetworkNode{
			ID:         id,
			Name:       "dynamic-node-updated",
			PlatformID: platformID,
		}, []*network.NetworkInterface{iface})
		_ = state.DeleteNode(id)
	}
}

var linkSeq uint64

func exerciseLinks(state *ScenarioState, iter int) {
	id := fmt.Sprintf("link-dyn-%d", atomic.AddUint64(&linkSeq, 1))
	link := &network.NetworkLink{
		ID:         id,
		InterfaceA: "node-stable-a/if0",
		InterfaceB: "node-stable-b/if0",
		Medium:     network.MediumWireless,
		IsImpaired: iter%2 == 0,
	}
	if err := state.CreateLink(link); err == nil {
		link.IsImpaired = !link.IsImpaired
		_ = state.UpdateLink(link)
		_ = state.DeleteLink(id)
	}
}

var srSeq uint64

func exerciseServiceRequests(state *ScenarioState, iter int) {
	id := fmt.Sprintf("sr-%d", atomic.AddUint64(&srSeq, 1))
	sr := &model.ServiceRequest{
		ID:        id,
		SrcNodeID: "node-stable-a",
		DstNodeID: "node-stable-b",
		Priority:  int32(iter % 3),
	}
	if err := state.CreateServiceRequest(sr); err == nil {
		sr.Priority = int32((iter + 1) % 5)
		_ = state.UpdateServiceRequest(sr)
		_ = state.DeleteServiceRequest(id)
	}
}
