package state

import (
	"errors"
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestScenarioStatePlatformCRUD(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net)

	p := &model.PlatformDefinition{ID: "p1", Name: "one"}
	if err := s.CreatePlatform(p); err != nil {
		t.Fatalf("CreatePlatform error: %v", err)
	}

	if err := s.CreatePlatform(p); !errors.Is(err, ErrPlatformExists) {
		t.Fatalf("CreatePlatform duplicate error = %v, want ErrPlatformExists", err)
	}

	if got, err := s.GetPlatform("p1"); err != nil || got.Name != "one" {
		t.Fatalf("GetPlatform got (%#v, %v), want name=one", got, err)
	}

	if list := s.ListPlatforms(); len(list) != 1 || list[0].ID != "p1" {
		t.Fatalf("ListPlatforms = %#v, want single p1", list)
	}

	updated := &model.PlatformDefinition{ID: "p1", Name: "updated"}
	if err := s.UpdatePlatform(updated); err != nil {
		t.Fatalf("UpdatePlatform error: %v", err)
	}
	if got, err := s.GetPlatform("p1"); err != nil || got.Name != "updated" {
		t.Fatalf("GetPlatform after update got (%#v, %v), want name=updated", got, err)
	}

	if err := s.DeletePlatform("p1"); err != nil {
		t.Fatalf("DeletePlatform error: %v", err)
	}

	if _, err := s.GetPlatform("p1"); !errors.Is(err, ErrPlatformNotFound) {
		t.Fatalf("GetPlatform after delete error = %v, want ErrPlatformNotFound", err)
	}

	if err := s.DeletePlatform("p1"); !errors.Is(err, ErrPlatformNotFound) {
		t.Fatalf("DeletePlatform missing error = %v, want ErrPlatformNotFound", err)
	}
}

func TestDeletePlatformFailsWhenNodesPresent(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net)

	platformID := "plat-in-use"
	if err := s.CreatePlatform(&model.PlatformDefinition{ID: platformID, Name: platformID}); err != nil {
		t.Fatalf("CreatePlatform error: %v", err)
	}

	nodeID := "node-on-platform"
	if err := s.CreateNode(&model.NetworkNode{ID: nodeID, PlatformID: platformID}, []*network.NetworkInterface{
		{ID: nodeID + "/if0", ParentNodeID: nodeID, Medium: network.MediumWired},
	}); err != nil {
		t.Fatalf("CreateNode error: %v", err)
	}

	if err := s.DeletePlatform(platformID); !errors.Is(err, ErrPlatformInUse) {
		t.Fatalf("DeletePlatform error = %v, want ErrPlatformInUse", err)
	}

	if got, err := s.GetPlatform(platformID); err != nil || got == nil {
		t.Fatalf("platform should remain after failed delete, got (%+v, %v)", got, err)
	}
	if got := s.physKB.GetNetworkNode(nodeID); got == nil {
		t.Fatalf("node should remain after failed platform delete; got nil")
	}
}
