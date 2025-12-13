package controller

import (
	"context"
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
)

func TestScheduler_PrecomputeContactWindowsPotentialLink(t *testing.T) {
	scheduler, _, _ := setupSchedulerTest(t)
	ctx := context.Background()
	now := scheduler.Clock.Now()
	horizon := now.Add(ContactHorizon)

	windows := scheduler.PrecomputeContactWindows(ctx, now, horizon)
	linkWindows, ok := windows["link-ab"]
	if !ok {
		t.Fatalf("expected windows for link-ab")
	}
	if len(linkWindows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(linkWindows))
	}
	duration := linkWindows[0].end.Sub(linkWindows[0].start)
	if duration != defaultPotentialWindow {
		t.Fatalf("expected potential window %v, got %v", defaultPotentialWindow, duration)
	}
}

func TestScheduler_PrecomputeContactWindowsActiveLink(t *testing.T) {
	scheduler, _, _ := setupSchedulerTest(t)
	ctx := context.Background()
	link, err := scheduler.State.GetLink("link-ab")
	if err != nil {
		t.Fatalf("GetLink failed: %v", err)
	}
	link.Status = core.LinkStatusActive
	if err := scheduler.State.UpdateLink(link); err != nil {
		t.Fatalf("UpdateLink failed: %v", err)
	}

	now := scheduler.Clock.Now()
	horizon := now.Add(ContactHorizon)
	windows := scheduler.PrecomputeContactWindows(ctx, now, horizon)
	linkWindows, ok := windows["link-ab"]
	if !ok {
		t.Fatalf("expected windows for link-ab")
	}
	if len(linkWindows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(linkWindows))
	}
	duration := linkWindows[0].end.Sub(linkWindows[0].start)
	if duration != defaultActiveWindow {
		t.Fatalf("expected active window %v, got %v", defaultActiveWindow, duration)
	}
}

func TestScheduler_PrecomputeContactWindowsClipsAtHorizon(t *testing.T) {
	scheduler, _, _ := setupSchedulerTest(t)
	ctx := context.Background()
	now := scheduler.Clock.Now()
	horizon := now.Add(10 * time.Minute)

	windows := scheduler.PrecomputeContactWindows(ctx, now, horizon)
	linkWindows, ok := windows["link-ab"]
	if !ok {
		t.Fatalf("expected windows for link-ab")
	}
	if len(linkWindows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(linkWindows))
	}
	if !linkWindows[0].end.Equal(horizon) {
		t.Fatalf("expected end == horizon %v, got %v", horizon, linkWindows[0].end)
	}
}
