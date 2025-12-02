package state

import (
	"errors"
	"testing"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestScenarioStateLinkCRUD(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net)

	if err := net.AddInterface(&network.NetworkInterface{
		ID:            "ifA",
		Name:          "If-A",
		Medium:        network.MediumWireless,
		ParentNodeID:  "nodeA",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifA) failed: %v", err)
	}
	if err := net.AddInterface(&network.NetworkInterface{
		ID:            "ifB",
		Name:          "If-B",
		Medium:        network.MediumWireless,
		ParentNodeID:  "nodeB",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifB) failed: %v", err)
	}

	link := &network.NetworkLink{
		ID:         "link-1",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     network.MediumWired,
		IsUp:       true,
	}

	if err := s.CreateLink(link); err != nil {
		t.Fatalf("CreateLink error: %v", err)
	}

	got, err := s.GetLink("link-1")
	if err != nil || got == nil {
		t.Fatalf("GetLink got (%+v, %v), want link", got, err)
	}

	links := s.ListLinks()
	if len(links) != 1 || links[0].ID != "link-1" {
		t.Fatalf("ListLinks = %+v, want link-1", links)
	}

	ifA := net.GetNetworkInterface("ifA")
	ifB := net.GetNetworkInterface("ifB")
	if !containsID(ifA.LinkIDs, "link-1") {
		t.Fatalf("ifA.LinkIDs missing link-1, got %v", ifA.LinkIDs)
	}
	if !containsID(ifB.LinkIDs, "link-1") {
		t.Fatalf("ifB.LinkIDs missing link-1, got %v", ifB.LinkIDs)
	}

	if err := s.DeleteLink("link-1"); err != nil {
		t.Fatalf("DeleteLink error: %v", err)
	}
	if _, err := s.GetLink("link-1"); !errors.Is(err, ErrLinkNotFound) {
		t.Fatalf("GetLink after delete error = %v, want ErrLinkNotFound", err)
	}
	if err := s.DeleteLink("link-1"); !errors.Is(err, ErrLinkNotFound) {
		t.Fatalf("DeleteLink missing error = %v, want ErrLinkNotFound", err)
	}

	if len(ifA.LinkIDs) != 0 {
		t.Fatalf("ifA.LinkIDs should be empty after delete, got %v", ifA.LinkIDs)
	}
	if len(ifB.LinkIDs) != 0 {
		t.Fatalf("ifB.LinkIDs should be empty after delete, got %v", ifB.LinkIDs)
	}
}

func TestScenarioStateUpdateLink(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net)

	if err := net.AddInterface(&network.NetworkInterface{
		ID:            "ifA",
		Name:          "If-A",
		Medium:        network.MediumWireless,
		ParentNodeID:  "nodeA",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifA) failed: %v", err)
	}
	if err := net.AddInterface(&network.NetworkInterface{
		ID:            "ifB",
		Name:          "If-B",
		Medium:        network.MediumWireless,
		ParentNodeID:  "nodeB",
		IsOperational: true,
	}); err != nil {
		t.Fatalf("AddInterface(ifB) failed: %v", err)
	}

	link := &network.NetworkLink{
		ID:         "link-1",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     network.MediumWired,
		IsUp:       true,
	}

	if err := s.CreateLink(link); err != nil {
		t.Fatalf("CreateLink error: %v", err)
	}

	got, err := s.GetLink("link-1")
	if err != nil || got == nil {
		t.Fatalf("GetLink got (%+v, %v), want link", got, err)
	}

	links := s.ListLinks()
	if len(links) != 1 || links[0].ID != "link-1" {
		t.Fatalf("ListLinks = %+v, want link-1", links)
	}

	// Flip IsUp and verify UpdateLink works.
	link.IsUp = false
	if err := s.UpdateLink(link); err != nil {
		t.Fatalf("UpdateLink error: %v", err)
	}

	got, err = s.GetLink(link.ID)
	if err != nil {
		t.Fatalf("GetLink after UpdateLink error: %v", err)
	}
	if got.IsUp {
		t.Fatalf("GetLink after UpdateLink IsUp = %v, want false", got.IsUp)
	}
}

func TestScenarioStateServiceRequestCRUD(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net)

	sr := &model.ServiceRequest{
		ID:        "sr-1",
		Type:      "video",
		SrcNodeID: "nodeA",
		DstNodeID: "nodeB",
		Priority:  1,
	}

	if err := s.CreateServiceRequest(sr); err != nil {
		t.Fatalf("CreateServiceRequest error: %v", err)
	}
	if err := s.CreateServiceRequest(sr); !errors.Is(err, ErrServiceRequestExists) {
		t.Fatalf("CreateServiceRequest duplicate error = %v, want ErrServiceRequestExists", err)
	}

	got, err := s.GetServiceRequest("sr-1")
	if err != nil || got == nil || got.Type != "video" {
		t.Fatalf("GetServiceRequest got (%+v, %v), want Type=video", got, err)
	}

	all := s.ListServiceRequests()
	if len(all) != 1 || all[0].ID != "sr-1" {
		t.Fatalf("ListServiceRequests = %+v, want sr-1", all)
	}

	updated := &model.ServiceRequest{
		ID:        "sr-1",
		Type:      "backhaul",
		SrcNodeID: "nodeA",
		DstNodeID: "nodeB",
		Priority:  2,
	}
	if err := s.UpdateServiceRequest(updated); err != nil {
		t.Fatalf("UpdateServiceRequest error: %v", err)
	}
	if got, err := s.GetServiceRequest("sr-1"); err != nil || got.Type != "backhaul" || got.Priority != 2 {
		t.Fatalf("GetServiceRequest after update got (%+v, %v), want Type=backhaul Priority=2", got, err)
	}

	if err := s.DeleteServiceRequest("sr-1"); err != nil {
		t.Fatalf("DeleteServiceRequest error: %v", err)
	}
	if _, err := s.GetServiceRequest("sr-1"); !errors.Is(err, ErrServiceRequestNotFound) {
		t.Fatalf("GetServiceRequest after delete error = %v, want ErrServiceRequestNotFound", err)
	}
	if err := s.DeleteServiceRequest("sr-1"); !errors.Is(err, ErrServiceRequestNotFound) {
		t.Fatalf("DeleteServiceRequest missing error = %v, want ErrServiceRequestNotFound", err)
	}

	if list := s.ListServiceRequests(); len(list) != 0 {
		t.Fatalf("ListServiceRequests after delete = %+v, want empty", list)
	}
}
