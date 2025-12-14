package state

import (
	"testing"
	"time"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func newScenarioStateForStatusTest() *ScenarioState {
	return NewScenarioState(kb.NewKnowledgeBase(), network.NewKnowledgeBase(), logging.Noop())
}

func TestUpdateServiceRequestStatusTracksProvisioning(t *testing.T) {
	state := newScenarioStateForStatusTest()
	sr := &model.ServiceRequest{
		ID: "sr-status",
	}
	if err := state.CreateServiceRequest(sr); err != nil {
		t.Fatalf("CreateServiceRequest failed: %v", err)
	}

	interval := &model.TimeInterval{
		StartTime: time.Unix(10, 0),
		EndTime:   time.Unix(20, 0),
	}
	if err := state.UpdateServiceRequestStatus(sr.ID, true, interval); err != nil {
		t.Fatalf("UpdateServiceRequestStatus failed: %v", err)
	}

	status, err := state.GetServiceRequestStatus(sr.ID)
	if err != nil {
		t.Fatalf("GetServiceRequestStatus failed: %v", err)
	}
	if !status.IsProvisionedNow {
		t.Fatalf("expected IsProvisionedNow true, got false")
	}
	if status.CurrentInterval == nil {
		t.Fatalf("expected current interval to be set")
	}
	if len(status.AllIntervals) != 1 {
		t.Fatalf("expected 1 interval, got %d", len(status.AllIntervals))
	}
	if !status.LastProvisionedAt.Equal(interval.StartTime) {
		t.Fatalf("expected LastProvisionedAt %v, got %v", interval.StartTime, status.LastProvisionedAt)
	}
	if len(sr.ProvisionedIntervals) != 1 {
		t.Fatalf("expected ServiceRequest ProvisionedIntervals to have 1 entry, got %d", len(sr.ProvisionedIntervals))
	}

	secondInterval := &model.TimeInterval{
		StartTime: time.Unix(30, 0),
		EndTime:   time.Unix(40, 0),
	}
	if err := state.UpdateServiceRequestStatus(sr.ID, false, secondInterval); err != nil {
		t.Fatalf("UpdateServiceRequestStatus (unprovision) failed: %v", err)
	}

	status, err = state.GetServiceRequestStatus(sr.ID)
	if err != nil {
		t.Fatalf("GetServiceRequestStatus failed: %v", err)
	}
	if status.IsProvisionedNow {
		t.Fatalf("expected IsProvisionedNow false after tear down")
	}
	if status.CurrentInterval != nil {
		t.Fatalf("expected current interval to be nil when not provisioned")
	}
	if !status.LastUnprovisionedAt.Equal(secondInterval.EndTime) {
		t.Fatalf("expected LastUnprovisionedAt %v, got %v", secondInterval.EndTime, status.LastUnprovisionedAt)
	}
}

func TestGetServiceRequestStatusReturnsCopy(t *testing.T) {
	state := newScenarioStateForStatusTest()
	sr := &model.ServiceRequest{ID: "sr-copy"}
	if err := state.CreateServiceRequest(sr); err != nil {
		t.Fatalf("CreateServiceRequest failed: %v", err)
	}
	interval := &model.TimeInterval{
		StartTime: time.Unix(50, 0),
		EndTime:   time.Unix(60, 0),
	}
	if err := state.UpdateServiceRequestStatus(sr.ID, true, interval); err != nil {
		t.Fatalf("UpdateServiceRequestStatus failed: %v", err)
	}

	first, err := state.GetServiceRequestStatus(sr.ID)
	if err != nil {
		t.Fatalf("GetServiceRequestStatus failed: %v", err)
	}
	first.AllIntervals[0].StartTime = time.Unix(0, 0)

	second, err := state.GetServiceRequestStatus(sr.ID)
	if err != nil {
		t.Fatalf("GetServiceRequestStatus failed: %v", err)
	}
	if second.AllIntervals[0].StartTime != interval.StartTime {
		t.Fatalf("expected copied intervals, got %v", second.AllIntervals[0].StartTime)
	}
}

func TestUpdateServiceRequestStatusNotFound(t *testing.T) {
	state := newScenarioStateForStatusTest()
	if err := state.UpdateServiceRequestStatus("missing", true, nil); err == nil {
		t.Fatalf("expected error for missing service request")
	}
	if _, err := state.GetServiceRequestStatus("missing"); err == nil {
		t.Fatalf("expected error fetching status for missing service request")
	}
}

func TestUpdateServiceRequestStatusClosesIntervals(t *testing.T) {
	state := newScenarioStateForStatusTest()
	sr := &model.ServiceRequest{ID: "sr-close"}
	if err := state.CreateServiceRequest(sr); err != nil {
		t.Fatalf("CreateServiceRequest failed: %v", err)
	}

	start := time.Unix(10, 0)
	interval := &model.TimeInterval{
		StartTime: start,
	}
	if err := state.UpdateServiceRequestStatus(sr.ID, true, interval); err != nil {
		t.Fatalf("UpdateServiceRequestStatus failed: %v", err)
	}

	closeInterval := &model.TimeInterval{
		StartTime: start,
		EndTime:   time.Unix(20, 0),
	}
	if err := state.UpdateServiceRequestStatus(sr.ID, false, closeInterval); err != nil {
		t.Fatalf("UpdateServiceRequestStatus (unprovision) failed: %v", err)
	}

	status, err := state.GetServiceRequestStatus(sr.ID)
	if err != nil {
		t.Fatalf("GetServiceRequestStatus failed: %v", err)
	}
	if len(status.AllIntervals) != 1 {
		t.Fatalf("expected 1 interval, got %d", len(status.AllIntervals))
	}
	if !status.AllIntervals[0].EndTime.Equal(closeInterval.EndTime) {
		t.Fatalf("expected interval end %v, got %v", closeInterval.EndTime, status.AllIntervals[0].EndTime)
	}
	if !status.LastUnprovisionedAt.Equal(closeInterval.EndTime) {
		t.Fatalf("expected LastUnprovisionedAt %v, got %v", closeInterval.EndTime, status.LastUnprovisionedAt)
	}

	sr, err = state.GetServiceRequest(sr.ID)
	if err != nil {
		t.Fatalf("GetServiceRequest failed: %v", err)
	}
	if len(sr.ProvisionedIntervals) != 1 {
		t.Fatalf("expected ServiceRequest to track 1 interval, got %d", len(sr.ProvisionedIntervals))
	}
	if !sr.ProvisionedIntervals[0].EndTime.Equal(closeInterval.EndTime) {
		t.Fatalf("expected last interval end %v, got %v", closeInterval.EndTime, sr.ProvisionedIntervals[0].EndTime)
	}
}
