package state

import (
	"errors"
	"testing"

	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestScenarioStateDomainCRUD(t *testing.T) {
	state := newTestStateWithPlatform(t)
	ensureNode(t, state, "node-a")
	ensureNode(t, state, "node-b")

	domain := &model.SchedulingDomain{
		DomainID:           "dom-1",
		Name:               "Test Domain",
		Nodes:              []string{"node-a", "node-b"},
		Capabilities:       map[string]interface{}{"region": "alpha"},
		FederationEndpoint: "https://federation.example.com",
	}
	if err := state.CreateDomain(domain); err != nil {
		t.Fatalf("CreateDomain() error = %v", err)
	}

	got, err := state.GetDomain("dom-1")
	if err != nil {
		t.Fatalf("GetDomain() error = %v", err)
	}
	if got.DomainID != domain.DomainID {
		t.Fatalf("GetDomain() = %v, want %v", got.DomainID, domain.DomainID)
	}

	if domainID, err := state.GetDomainForNode("node-a"); err != nil || domainID != "dom-1" {
		t.Fatalf("GetDomainForNode() = %q, %v, want dom-1, nil", domainID, err)
	}

	list := state.ListDomains()
	if len(list) != 1 {
		t.Fatalf("ListDomains() len = %d, want 1", len(list))
	}

	list[0].Nodes = nil
	list[0].Capabilities["region"] = "beta"
	rechecked, _ := state.GetDomain("dom-1")
	if len(rechecked.Nodes) != 2 {
		t.Fatalf("GetDomain() nodes mutated via list result")
	}
	if rechecked.Capabilities["region"] != "alpha" {
		t.Fatalf("GetDomain() capabilities mutated via list result")
	}

	if err := state.DeleteDomain("dom-1"); err != nil {
		t.Fatalf("DeleteDomain() error = %v", err)
	}

	if _, err := state.GetDomain("dom-1"); !errors.Is(err, ErrDomainNotFound) {
		t.Fatalf("GetDomain() after delete error = %v, want ErrDomainNotFound", err)
	}
	if _, err := state.GetDomainForNode("node-a"); !errors.Is(err, ErrDomainNotFound) {
		t.Fatalf("GetDomainForNode() after delete error = %v, want ErrDomainNotFound", err)
	}
}

func TestScenarioStateDomainValidation(t *testing.T) {
	state := newTestStateWithPlatform(t)
	ensureNode(t, state, "node-x")

	if err := state.CreateDomain(&model.SchedulingDomain{
		DomainID: "dom-2",
		Nodes:    []string{"missing"},
	}); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("CreateDomain() missing node error = %v, want ErrNodeNotFound", err)
	}

	if err := state.CreateDomain(&model.SchedulingDomain{
		DomainID:           "dom-2",
		Nodes:              []string{"node-x"},
		FederationEndpoint: "://invalid",
	}); !errors.Is(err, ErrDomainInvalid) {
		t.Fatalf("CreateDomain() invalid endpoint error = %v, want ErrDomainInvalid", err)
	}

	if err := state.CreateDomain(&model.SchedulingDomain{
		DomainID: "dom-3",
		Nodes:    []string{"node-x"},
	}); err != nil {
		t.Fatalf("CreateDomain() error = %v", err)
	}

	if err := state.CreateDomain(&model.SchedulingDomain{
		DomainID: "dom-4",
		Nodes:    []string{"node-x"},
	}); !errors.Is(err, ErrDomainInvalid) {
		t.Fatalf("CreateDomain() duplicate node error = %v, want ErrDomainInvalid", err)
	}

	if _, err := state.GetDomainForNode("unknown"); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("GetDomainForNode() unknown node error = %v, want ErrNodeNotFound", err)
	}
}

func TestScenarioStateCrossDomainServiceRequest(t *testing.T) {
	state := newTestStateWithPlatform(t)
	ensureNode(t, state, "node-left")
	ensureNode(t, state, "node-right")

	domainLeft := &model.SchedulingDomain{
		DomainID: "dom-left",
		Nodes:    []string{"node-left"},
	}
	domainRight := &model.SchedulingDomain{
		DomainID: "dom-right",
		Nodes:    []string{"node-right"},
	}
	if err := state.CreateDomain(domainLeft); err != nil {
		t.Fatalf("CreateDomain(left) error = %v", err)
	}
	if err := state.CreateDomain(domainRight); err != nil {
		t.Fatalf("CreateDomain(right) error = %v", err)
	}

	sr := &model.ServiceRequest{
		ID:              "sr-cross",
		SrcNodeID:       "node-left",
		DstNodeID:       "node-right",
		FederationToken: "tok",
	}
	if err := state.CreateServiceRequest(sr); err != nil {
		t.Fatalf("CreateServiceRequest(cross) error = %v", err)
	}
	if !sr.CrossDomain {
		t.Fatalf("CreateServiceRequest cross-domain flag = false, want true")
	}
	if sr.SourceDomain != domainLeft.DomainID {
		t.Fatalf("SourceDomain = %s, want %s", sr.SourceDomain, domainLeft.DomainID)
	}
	if sr.DestDomain != domainRight.DomainID {
		t.Fatalf("DestDomain = %s, want %s", sr.DestDomain, domainRight.DomainID)
	}
}

func TestScenarioStateCrossDomainTokenRequired(t *testing.T) {
	state := newTestStateWithPlatform(t)
	ensureNode(t, state, "node-left")
	ensureNode(t, state, "node-right")

	domainLeft := &model.SchedulingDomain{
		DomainID: "dom-left",
		Nodes:    []string{"node-left"},
	}
	domainRight := &model.SchedulingDomain{
		DomainID: "dom-right",
		Nodes:    []string{"node-right"},
	}
	if err := state.CreateDomain(domainLeft); err != nil {
		t.Fatalf("CreateDomain(left) error = %v", err)
	}
	if err := state.CreateDomain(domainRight); err != nil {
		t.Fatalf("CreateDomain(right) error = %v", err)
	}

	sr := &model.ServiceRequest{
		ID:        "sr-missing-token",
		SrcNodeID: "node-left",
		DstNodeID: "node-right",
	}
	sr.SourceDomain = "dom-left"
	sr.DestDomain = "dom-right"

	if err := state.CreateServiceRequest(sr); err == nil || !errors.Is(err, ErrDomainInvalid) {
		t.Fatalf("CreateServiceRequest missing token error = %v, want %v", err, ErrDomainInvalid)
	}
}

func TestScenarioStateDomainReassignment(t *testing.T) {
	state := newTestStateWithPlatform(t)
	ensureNode(t, state, "node-reassign")

	domain1 := &model.SchedulingDomain{
		DomainID: "dom-first",
		Nodes:    []string{"node-reassign"},
	}
	if err := state.CreateDomain(domain1); err != nil {
		t.Fatalf("CreateDomain(domain1) error = %v", err)
	}

	if _, err := state.GetDomainForNode("node-reassign"); err != nil {
		t.Fatalf("GetDomainForNode() after first domain = %v", err)
	}

	if err := state.DeleteDomain("dom-first"); err != nil {
		t.Fatalf("DeleteDomain(dom-first) error = %v", err)
	}

	domain2 := &model.SchedulingDomain{
		DomainID: "dom-second",
		Nodes:    []string{"node-reassign"},
	}
	if err := state.CreateDomain(domain2); err != nil {
		t.Fatalf("CreateDomain(domain2) error = %v", err)
	}
	if domainID, err := state.GetDomainForNode("node-reassign"); err != nil || domainID != domain2.DomainID {
		t.Fatalf("GetDomainForNode() after reassignment = %q, %v, want %q, nil", domainID, err, domain2.DomainID)
	}
}
