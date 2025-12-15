package controller

import (
	"context"
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestComputePathsParallel(t *testing.T) {
	now := time.Now()
	scheduler := &Scheduler{
		Clock: &fakeEventScheduler{now: now},
		pathFinder: func(ctx context.Context, srcNodeID, dstNodeID string, start time.Time, horizon time.Duration) (*Path, error) {
			return &Path{
				Hops: []PathHop{
					{LinkID: srcNodeID + "->" + dstNodeID, StartTime: start, EndTime: start.Add(time.Minute)},
				},
			}, nil
		},
		log:             logging.Noop(),
		pathWorkerCount: 2,
	}
	srs := []*model.ServiceRequest{
		{ID: "sr1", SrcNodeID: "a", DstNodeID: "b"},
		{ID: "sr2", SrcNodeID: "c", DstNodeID: "d"},
	}
	paths := scheduler.ComputePathsParallel(context.Background(), srs)
	if len(paths) != len(srs) {
		t.Fatalf("expected %d paths, got %d", len(srs), len(paths))
	}
	for i, path := range paths {
		if path == nil || len(path.Hops) == 0 {
			t.Fatalf("path %d is nil or empty", i)
		}
	}
}
