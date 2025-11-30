package kb

import (
	"fmt"
	"sync"
	"testing"

	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestAddAndGetPlatform(t *testing.T) {
	store := NewKnowledgeBase()
	p := &model.PlatformDefinition{
		ID:   "p1",
		Name: "Platform1",
	}
	if err := store.AddPlatform(p); err != nil {
		t.Fatalf("AddPlatform error: %v", err)
	}
	got := store.GetPlatform("p1")
	if got == nil || got.Name != "Platform1" {
		t.Fatalf("GetPlatform returned %#v, want name Platform1", got)
	}
}

func TestAddPlatformDuplicate(t *testing.T) {
	store := NewKnowledgeBase()
	p := &model.PlatformDefinition{ID: "p1"}
	if err := store.AddPlatform(p); err != nil {
		t.Fatalf("first AddPlatform error: %v", err)
	}
	if err := store.AddPlatform(&model.PlatformDefinition{ID: "p1"}); err == nil {
		t.Fatalf("expected duplicate AddPlatform to fail")
	}
}

func TestAddNetworkNodePlatformValidation(t *testing.T) {
	store := NewKnowledgeBase()
	n := &model.NetworkNode{ID: "n1", PlatformID: "missing"}
	if err := store.AddNetworkNode(n); err == nil {
		t.Fatalf("expected error when platform does not exist")
	}

	p := &model.PlatformDefinition{ID: "p1"}
	if err := store.AddPlatform(p); err != nil {
		t.Fatalf("AddPlatform error: %v", err)
	}
	n.PlatformID = "p1"
	if err := store.AddNetworkNode(n); err != nil {
		t.Fatalf("AddNetworkNode error: %v", err)
	}
}

func TestListPlatformsAndNodes(t *testing.T) {
	store := NewKnowledgeBase()
	for i := range 3 {
		pid := fmt.Sprintf("p-%d", i)
		nid := fmt.Sprintf("n-%d", i)

		if err := store.AddPlatform(&model.PlatformDefinition{ID: pid}); err != nil {
			t.Fatalf("AddPlatform error: %v", err)
		}
		if err := store.AddNetworkNode(&model.NetworkNode{ID: nid}); err != nil {
			t.Fatalf("AddNetworkNode error: %v", err)
		}
	}

	if got := len(store.ListPlatforms()); got != 3 {
		t.Fatalf("ListPlatforms len=%d, want 3", got)
	}
	if got := len(store.ListNetworkNodes()); got != 3 {
		t.Fatalf("ListNetworkNodes len=%d, want 3", got)
	}
}

func TestUpdatePlatformPositionAndSubscribe(t *testing.T) {
	store := NewKnowledgeBase()
	p := &model.PlatformDefinition{ID: "p1"}
	if err := store.AddPlatform(p); err != nil {
		t.Fatalf("AddPlatform error: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var got Event
	store.Subscribe(func(e Event) {
		got = e
		wg.Done()
	})

	pos := model.Motion{X: 1, Y: 2, Z: 3}
	if err := store.UpdatePlatformPosition("p1", pos); err != nil {
		t.Fatalf("UpdatePlatformPosition error: %v", err)
	}

	wg.Wait()
	if got.Type != EventPlatformUpdated {
		t.Fatalf("got event type %v, want EventPlatformUpdated", got.Type)
	}
	if got.Platform.Coordinates != pos {
		t.Fatalf("event platform position = %#v, want %#v", got.Platform.Coordinates, pos)
	}
}

func TestConcurrentAccess(t *testing.T) {
	store := NewKnowledgeBase()
	p := &model.PlatformDefinition{ID: "p1"}
	if err := store.AddPlatform(p); err != nil {
		t.Fatalf("AddPlatform error: %v", err)
	}

	var wg sync.WaitGroup
	// Concurrent readers/writers
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = store.GetPlatform("p1")
			_ = store.ListPlatforms()
		}()
		go func() {
			defer wg.Done()
			_ = store.UpdatePlatformPosition("p1", model.Motion{X: float64(i)})
		}()
	}
	wg.Wait()
}
