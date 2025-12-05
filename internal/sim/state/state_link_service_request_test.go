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

func TestScenarioStateCreateLinksBatch(t *testing.T) {
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

	linkAB := &network.NetworkLink{
		ID:         "bidi|ifA->ifB",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     network.MediumWireless,
		IsUp:       true,
	}
	linkBA := &network.NetworkLink{
		ID:         "bidi|ifB->ifA",
		InterfaceA: "ifB",
		InterfaceB: "ifA",
		Medium:     network.MediumWireless,
		IsUp:       true,
	}

	if err := s.CreateLinks(linkAB, linkBA); err != nil {
		t.Fatalf("CreateLinks error: %v", err)
	}

	all := s.ListLinks()
	if len(all) != 2 {
		t.Fatalf("ListLinks len = %d, want 2", len(all))
	}
	ifA := net.GetNetworkInterface("ifA")
	ifB := net.GetNetworkInterface("ifB")
	if !containsID(ifA.LinkIDs, linkAB.ID) || !containsID(ifA.LinkIDs, linkBA.ID) {
		t.Fatalf("ifA.LinkIDs = %v, want both links", ifA.LinkIDs)
	}
	if !containsID(ifB.LinkIDs, linkAB.ID) || !containsID(ifB.LinkIDs, linkBA.ID) {
		t.Fatalf("ifB.LinkIDs = %v, want both links", ifB.LinkIDs)
	}
}

func TestScenarioStateCreateLinksRollback(t *testing.T) {
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

	linkAB := &network.NetworkLink{
		ID:         "bidi|ifA->ifB",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     network.MediumWireless,
		IsUp:       true,
	}
	badLink := &network.NetworkLink{
		ID:         "bidi|ifB->missing",
		InterfaceA: "ifB",
		InterfaceB: "missing",
		Medium:     network.MediumWireless,
		IsUp:       true,
	}

	if err := s.CreateLinks(linkAB, badLink); err == nil {
		t.Fatalf("CreateLinks expected error, got nil")
	}
	if links := s.ListLinks(); len(links) != 0 {
		t.Fatalf("ListLinks after rollback = %+v, want empty", links)
	}
	if ifA := net.GetNetworkInterface("ifA"); len(ifA.LinkIDs) != 0 {
		t.Fatalf("ifA.LinkIDs after rollback = %v, want empty", ifA.LinkIDs)
	}
	if ifB := net.GetNetworkInterface("ifB"); len(ifB.LinkIDs) != 0 {
		t.Fatalf("ifB.LinkIDs after rollback = %v, want empty", ifB.LinkIDs)
	}
}

func TestScenarioStateServiceRequestCRUD(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	net := network.NewKnowledgeBase()
	s := NewScenarioState(phys, net)

	sr := &model.ServiceRequest{
		ID:        "sr-1",
		SrcNodeID: "nodeA",
		DstNodeID: "nodeB",
		Priority:  1,
		FlowRequirements: []model.FlowRequirement{
			{RequestedBandwidth: 1_000_000},
		},
	}

	if err := s.CreateServiceRequest(sr); err != nil {
		t.Fatalf("CreateServiceRequest error: %v", err)
	}
	if err := s.CreateServiceRequest(sr); !errors.Is(err, ErrServiceRequestExists) {
		t.Fatalf("CreateServiceRequest duplicate error = %v, want ErrServiceRequestExists", err)
	}

	got, err := s.GetServiceRequest("sr-1")
	if err != nil || got == nil || got.Priority != 1 {
		t.Fatalf("GetServiceRequest got (%+v, %v), want Priority=1", got, err)
	}

	all := s.ListServiceRequests()
	if len(all) != 1 || all[0].ID != "sr-1" {
		t.Fatalf("ListServiceRequests = %+v, want sr-1", all)
	}

	updated := &model.ServiceRequest{
		ID:                    "sr-1",
		SrcNodeID:             "nodeA",
		DstNodeID:             "nodeB",
		Priority:              2,
		AllowPartnerResources: true,
	}
	if err := s.UpdateServiceRequest(updated); err != nil {
		t.Fatalf("UpdateServiceRequest error: %v", err)
	}
	if got, err := s.GetServiceRequest("sr-1"); err != nil || got.Priority != 2 || !got.AllowPartnerResources {
		t.Fatalf("GetServiceRequest after update got (%+v, %v), want Priority=2 AllowPartnerResources=true", got, err)
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
