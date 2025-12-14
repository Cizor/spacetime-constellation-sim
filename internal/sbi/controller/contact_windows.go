package controller

import (
	"context"
	"sort"
	"time"
)

// ContactWindow describes the visibility window for a link.
type ContactWindow struct {
	LinkID    string
	StartTime time.Time
	EndTime   time.Time
	Quality   float64 // e.g., SNR or link quality metric.
}

// ContactPlan groups windows by link ID.
type ContactPlan map[string][]ContactWindow

// GetContactPlan returns the contact windows for a link within the optional horizon.
// A horizon of zero returns all future windows.
func (s *Scheduler) GetContactPlan(linkID string, horizon time.Duration) []ContactWindow {
	if linkID == "" {
		return nil
	}
	s.ensureContactWindows(context.Background())
	now := s.Clock.Now()
	if horizon < 0 {
		horizon = 0
	}
	limit := now.Add(horizon)
	windows := s.contactWindowsForLink(linkID)
	out := make([]ContactWindow, 0, len(windows))
	for _, window := range windows {
		if horizon > 0 && window.StartTime.After(limit) {
			continue
		}
		if window.EndTime.Before(now) {
			continue
		}
		out = append(out, ContactWindow{
			LinkID:    linkID,
			StartTime: window.StartTime,
			EndTime:   window.EndTime,
			Quality:   window.Quality,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartTime.Before(out[j].StartTime)
	})
	return out
}

// GetContactPlansForNode returns contact plans for all links attached to nodeID.
func (s *Scheduler) GetContactPlansForNode(nodeID string, horizon time.Duration) ContactPlan {
	plans := make(ContactPlan)
	if nodeID == "" {
		return plans
	}
	interfaces := s.State.InterfacesByNode()
	for _, iface := range interfaces[nodeID] {
		if iface == nil {
			continue
		}
		for _, linkID := range iface.LinkIDs {
			if _, exists := plans[linkID]; exists {
				continue
			}
			if plan := s.GetContactPlan(linkID, horizon); len(plan) > 0 {
				plans[linkID] = plan
			}
		}
	}
	return plans
}

func (s *Scheduler) linkSNR(linkID string) float64 {
	link, err := s.State.GetLink(linkID)
	if err != nil || link == nil {
		return 0
	}
	return link.SNRdB
}
