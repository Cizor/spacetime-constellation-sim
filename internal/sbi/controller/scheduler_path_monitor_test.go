package controller

import (
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
)

func TestCheckPathHealth(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	scheduler := &Scheduler{
		Clock: sbi.NewFakeEventScheduler(now),
		contactWindows: map[string][]ContactWindow{
			"link-1": {{
				LinkID:    "link-1",
				StartTime: now.Add(-time.Minute),
				EndTime:   now.Add(time.Minute),
			}},
		},
		activePaths: make(map[string]*ActivePath),
	}

	path := &Path{
		Hops: []PathHop{{
			LinkID:    "link-1",
			StartTime: now.Add(-time.Second),
			EndTime:   now.Add(time.Second),
		}},
	}
	if got := scheduler.CheckPathHealth(path, now); got != HealthHealthy {
		t.Fatalf("expected healthy path, got %v", got)
	}

	if got := scheduler.CheckPathHealth(path, now.Add(2*time.Minute)); got != HealthBroken {
		t.Fatalf("expected broken path when time passes, got %v", got)
	}

	path.Hops[0].LinkID = "missing"
	if got := scheduler.CheckPathHealth(path, now); got != HealthDegraded {
		t.Fatalf("expected degraded path when windows missing, got %v", got)
	}
}

func TestRecordActivePath(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	scheduler := &Scheduler{
		Clock: sbi.NewFakeEventScheduler(now),
		contactWindows: map[string][]ContactWindow{
			"link-1": {{
				LinkID:    "link-1",
				StartTime: now.Add(-time.Minute),
				EndTime:   now.Add(time.Minute),
			}},
		},
		activePaths: make(map[string]*ActivePath),
	}

	path := &Path{
		Hops: []PathHop{{
			LinkID:    "link-1",
			StartTime: now,
			EndTime:   now.Add(time.Minute),
		}},
	}
	entries := []scheduledEntryRef{
		{entryID: "entry-1", agentID: "node-A"},
	}
	hopEntries := map[int][]scheduledEntryRef{
		0: entries,
	}
	scheduler.recordActivePath("sr-1", path, entries, hopEntries, nil)
	ap, ok := scheduler.activePaths["sr-1"]
	if !ok {
		t.Fatalf("active path not recorded")
	}
	if ap.Health != HealthHealthy {
		t.Fatalf("expected healthy active path, got %v", ap.Health)
	}
	if len(ap.ScheduledActions) != 1 || ap.ScheduledActions[0] != "entry-1" {
		t.Fatalf("scheduled actions not tracked: %v", ap.ScheduledActions)
	}
}
