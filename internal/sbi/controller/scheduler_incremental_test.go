package controller

import (
	"context"
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/internal/logging"
)

type fakeEventScheduler struct {
	now time.Time
}

func (f *fakeEventScheduler) Schedule(time.Time, func()) string { return "" }
func (f *fakeEventScheduler) Cancel(string)                     {}
func (f *fakeEventScheduler) Now() time.Time {
	if f == nil {
		return time.Time{}
	}
	return f.now
}
func (f *fakeEventScheduler) RunDue() {}

func TestRecordActivePathUpdatesLinkMappings(t *testing.T) {
	now := time.Now()
	s := &Scheduler{
		Clock:                 &fakeEventScheduler{now: now},
		log:                   logging.Noop(),
		contactWindows:        map[string][]ContactWindow{"link-1": {{StartTime: now.Add(-time.Minute), EndTime: now.Add(time.Hour)}}},
		linkToServiceRequests: make(map[string]map[string]struct{}),
		srToLinks:             make(map[string]map[string]struct{}),
		activePaths:           make(map[string]*ActivePath),
		srEntries:             make(map[string][]scheduledEntryRef),
		linkEntries:           make(map[string][]scheduledEntryRef),
		bandwidthReservations: make(map[string]map[string]uint64),
		preemptionRecords:     make(map[string]preemptionRecord),
		powerAllocations:      make(map[string]string),
		minReplanInterval:     defaultReplanInterval,
		replanRequests:        make(chan struct{}, 1),
	}
	path := &Path{
		Hops: []PathHop{
			{LinkID: "link-1", StartTime: now, EndTime: now.Add(time.Minute)},
		},
	}
	entries := []scheduledEntryRef{{entryID: "entry"}}
	hopEntries := map[int][]scheduledEntryRef{0: entries}

	s.recordActivePath("sr-1", path, entries, hopEntries, nil)

	if _, ok := s.linkToServiceRequests["link-1"]["sr-1"]; !ok {
		t.Fatalf("expected sr-1 to be linked to link-1")
	}
	if _, ok := s.srToLinks["sr-1"]["link-1"]; !ok {
		t.Fatalf("expected sr-1 to track link-1")
	}

	s.removeActivePath("sr-1")
	if _, ok := s.linkToServiceRequests["link-1"]["sr-1"]; ok {
		t.Fatalf("expected link mapping to clear for sr-1")
	}
	if _, ok := s.srToLinks["sr-1"]; ok {
		t.Fatalf("expected sr-1 to be removed from srToLinks")
	}
}

func TestIncrementalUpdateReplansAffectedServiceRequests(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	s := &Scheduler{
		Clock: &fakeEventScheduler{now: now},
		log:   logging.Noop(),
		linkToServiceRequests: map[string]map[string]struct{}{
			"link-1": {"sr-1": {}, "sr-2": {}},
			"link-2": {"sr-3": {}},
		},
	}
	var called []string
	s.incrementalReplan = func(ctx context.Context, srID string) {
		called = append(called, srID)
	}

	if err := s.IncrementalUpdate(ctx, "link-1", "link_removed"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(called) != 2 {
		t.Fatalf("expected 2 replan calls, got %d", len(called))
	}
	found := map[string]struct{}{}
	for _, srID := range called {
		found[srID] = struct{}{}
	}
	if _, ok := found["sr-1"]; !ok {
		t.Fatalf("sr-1 not replanned")
	}
	if _, ok := found["sr-2"]; !ok {
		t.Fatalf("sr-2 not replanned")
	}

	if err := s.IncrementalUpdate(ctx, "", "link_removed"); err == nil {
		t.Fatalf("expected error for empty link id")
	}
}
