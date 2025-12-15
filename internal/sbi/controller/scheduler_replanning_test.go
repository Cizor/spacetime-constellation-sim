package controller

import (
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/model"
)

func prepareReplanTest(t *testing.T) (*Scheduler, time.Time, *model.ServiceRequest) {
	t.Helper()
	scheduler, _, now := setupSchedulerTest(t)
	scheduler.SetReplanInterval(0)

	sr := &model.ServiceRequest{
		ID:        "sr-replan",
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

	return scheduler, now, sr
}

func TestShouldReplanBrokenPath(t *testing.T) {
	scheduler, now, sr := prepareReplanTest(t)

	hop := PathHop{
		FromNodeID: "node-A",
		ToNodeID:   "node-B",
		LinkID:     "link-ab",
		StartTime:  now,
		EndTime:    now.Add(45 * time.Second),
	}
	path := &Path{
		Hops:       []PathHop{hop},
		ValidFrom:  hop.StartTime,
		ValidUntil: hop.EndTime,
	}
	scheduler.activePaths[sr.ID] = &ActivePath{
		ServiceRequestID: sr.ID,
		Path:             path,
		LastUpdated:      now,
		Health:           HealthHealthy,
	}

	scheduler.contactWindows = map[string][]ContactWindow{
		"link-ab": {{
			LinkID:    "link-ab",
			StartTime: now.Add(-time.Minute),
			EndTime:   now.Add(30 * time.Second),
			Quality:   20,
		}},
	}

	if !scheduler.ShouldReplan(sr.ID, now.Add(1*time.Minute)) {
		t.Fatalf("expected ShouldReplan true for broken path")
	}
}

func TestShouldReplanBetterWindow(t *testing.T) {
	scheduler, now, sr := prepareReplanTest(t)

	hop := PathHop{
		FromNodeID: "node-A",
		ToNodeID:   "node-B",
		LinkID:     "link-ab",
		StartTime:  now,
		EndTime:    now.Add(45 * time.Second),
	}
	path := &Path{
		Hops:       []PathHop{hop},
		ValidFrom:  hop.StartTime,
		ValidUntil: hop.EndTime,
	}
	scheduler.activePaths[sr.ID] = &ActivePath{
		ServiceRequestID: sr.ID,
		Path:             path,
		LastUpdated:      now,
		Health:           HealthHealthy,
	}

	scheduler.contactWindows = map[string][]ContactWindow{
		"link-ab": {
			{
				LinkID:    "link-ab",
				StartTime: now.Add(-time.Minute),
				EndTime:   now.Add(45 * time.Second),
				Quality:   20,
			},
			{
				LinkID:    "link-ab",
				StartTime: now.Add(1 * time.Minute),
				EndTime:   now.Add(2 * time.Minute),
				Quality:   20,
			},
		},
	}

	if !scheduler.ShouldReplan(sr.ID, now.Add(10*time.Second)) {
		t.Fatalf("expected ShouldReplan true when a longer window opens")
	}
}

func TestShouldReplanFrequencyLimit(t *testing.T) {
	scheduler, now, sr := prepareReplanTest(t)
	scheduler.SetReplanInterval(30 * time.Second)

	hop := PathHop{
		FromNodeID: "node-A",
		ToNodeID:   "node-B",
		LinkID:     "link-ab",
		StartTime:  now,
		EndTime:    now.Add(45 * time.Second),
	}
	path := &Path{
		Hops:       []PathHop{hop},
		ValidFrom:  hop.StartTime,
		ValidUntil: hop.EndTime,
	}
	scheduler.activePaths[sr.ID] = &ActivePath{
		ServiceRequestID: sr.ID,
		Path:             path,
		LastUpdated:      now,
		Health:           HealthHealthy,
	}

	scheduler.contactWindows = map[string][]ContactWindow{
		"link-ab": {
			{
				LinkID:    "link-ab",
				StartTime: now.Add(-time.Minute),
				EndTime:   now.Add(45 * time.Second),
				Quality:   20,
			},
			{
				LinkID:    "link-ab",
				StartTime: now.Add(1 * time.Minute),
				EndTime:   now.Add(2 * time.Minute),
				Quality:   20,
			},
		},
	}

	if !scheduler.ShouldReplan(sr.ID, now) {
		t.Fatalf("expected initial ShouldReplan true")
	}
	if scheduler.ShouldReplan(sr.ID, now.Add(10*time.Second)) {
		t.Fatalf("expected ShouldReplan false while interval not passed")
	}
	if !scheduler.ShouldReplan(sr.ID, now.Add(40*time.Second)) {
		t.Fatalf("expected ShouldReplan true after interval")
	}
}

func TestShouldReplanHigherPriorityConflict(t *testing.T) {
	scheduler, now, sr := prepareReplanTest(t)

	hop := PathHop{
		FromNodeID: "node-A",
		ToNodeID:   "node-B",
		LinkID:     "link-ab",
		StartTime:  now,
		EndTime:    now.Add(45 * time.Second),
	}
	path := &Path{
		Hops:       []PathHop{hop},
		ValidFrom:  hop.StartTime,
		ValidUntil: hop.EndTime,
	}
	scheduler.activePaths[sr.ID] = &ActivePath{
		ServiceRequestID: sr.ID,
		Path:             path,
		LastUpdated:      now,
		Health:           HealthHealthy,
	}

	high := &model.ServiceRequest{
		ID:               "sr-high",
		SrcNodeID:        "node-A",
		DstNodeID:        "node-B",
		Priority:         10,
		IsProvisionedNow: true,
	}
	if err := scheduler.State.CreateServiceRequest(high); err != nil {
		t.Fatalf("CreateServiceRequest failed: %v", err)
	}
	if err := scheduler.State.UpdateServiceRequest(high); err != nil {
		t.Fatalf("UpdateServiceRequest failed: %v", err)
	}

	scheduler.bandwidthReservations[sr.ID] = map[string]uint64{"link-ab": 1000000}
	scheduler.bandwidthReservations[high.ID] = map[string]uint64{"link-ab": 1000000}

	scheduler.contactWindows = map[string][]ContactWindow{
		"link-ab": {{
			LinkID:    "link-ab",
			StartTime: now.Add(-time.Minute),
			EndTime:   now.Add(45 * time.Second),
			Quality:   20,
		}},
	}

	if !scheduler.ShouldReplan(sr.ID, now.Add(5*time.Second)) {
		t.Fatalf("expected ShouldReplan true when higher priority SR shares links")
	}
}
