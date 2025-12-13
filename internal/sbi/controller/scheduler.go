package controller

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/model"
)

type cdpiClient interface {
	SendCreateEntry(agentID string, action *sbi.ScheduledAction) error
	SendDeleteEntry(agentID, entryID string) error
	hasAgent(agentID string) bool
}

// Scheduler implements the controller-side scheduling logic for Scope 4.
// It coordinates between ScenarioState (links, nodes), EventScheduler (time),
// and CDPIServer (sending actions to agents).
type Scheduler struct {
	State *state.ScenarioState
	Clock sbi.EventScheduler
	CDPI  cdpiClient
	log   logging.Logger

	// scheduledEntryIDs tracks entry IDs we've already scheduled to avoid duplicates.
	// This provides idempotency for ScheduleLinkBeams.
	scheduledEntryIDs map[string]bool
	// storageReservations tracks DTN storage allocations per service request.
	storageReservations map[string]float64
	// contactWindows stores sampled visibility windows per link.
	contactWindows map[string][]contactWindow
	// srEntries tracks the entries created for each ServiceRequest to support cleanup.
	srEntries map[string][]scheduledEntryRef
	// linkEntries tracks entries created per link for cleanup.
	linkEntries map[string][]scheduledEntryRef
}

// scheduledEntryRef captures a CDPI entry that we may need to clean up later.
type scheduledEntryRef struct {
	entryID string
	agentID string
}

// NewScheduler creates a new Scheduler with the given dependencies.
func NewScheduler(state *state.ScenarioState, clock sbi.EventScheduler, cdpi cdpiClient, log logging.Logger) *Scheduler {
	if log == nil {
		log = logging.Noop()
	}
	return &Scheduler{
		State:               state,
		Clock:               clock,
		CDPI:                cdpi,
		log:                 log,
		scheduledEntryIDs:   make(map[string]bool),
		storageReservations: make(map[string]float64),
		contactWindows:      make(map[string][]contactWindow),
		srEntries:           make(map[string][]scheduledEntryRef),
		linkEntries:         make(map[string][]scheduledEntryRef),
	}
}

const (
	ContactHorizon         = 1 * time.Hour
	defaultActiveWindow    = 45 * time.Minute
	defaultPotentialWindow = 20 * time.Minute
	defaultDtnHold         = 30 * time.Second
)

// RunInitialSchedule runs the initial scheduling pass, including link-driven
// beam scheduling and route scheduling. This should be called once at scenario startup after
// agents are connected.
func (s *Scheduler) RunInitialSchedule(ctx context.Context) error {
	// 1. Link-driven beam schedule
	if err := s.ScheduleLinkBeams(ctx); err != nil {
		return fmt.Errorf("link-driven beam scheduling failed: %w", err)
	}

	// 2. Route scheduling for single-hop links
	if err := s.ScheduleLinkRoutes(ctx); err != nil {
		return fmt.Errorf("link route scheduling failed: %w", err)
	}

	// 3. ServiceRequest-aware scheduling
	if err := s.ScheduleServiceRequests(ctx); err != nil {
		return fmt.Errorf("service request scheduling failed: %w", err)
	}

	return nil
}

// ScheduleLinkBeams implements link-driven beam scheduling:
// - For each potential link, determine visibility intervals [T_on, T_off]
// - Schedule ScheduledUpdateBeam at T_on and ScheduledDeleteBeam at T_off
// - Send actions to the appropriate agent via CDPI
func (s *Scheduler) ScheduleLinkBeams(ctx context.Context) error {
	now := s.Clock.Now()
	horizon := now.Add(ContactHorizon)

	// Get all potential links
	potentialLinks := s.getPotentialLinks()
	if len(potentialLinks) == 0 {
		s.log.Debug(ctx, "No potential links found for scheduling")
		return nil
	}

	s.log.Debug(ctx, "Scheduling beams for potential links",
		logging.Int("link_count", len(potentialLinks)),
		logging.String("horizon", horizon.Format(time.RFC3339)),
	)

	windows, _ := s.PrecomputeContactWindows(ctx, now, horizon)

	// For each potential link, determine visibility windows and schedule actions
	for _, link := range potentialLinks {
		if err := s.scheduleBeamForLink(ctx, link, windows[link.ID]); err != nil {
			s.log.Warn(ctx, "Failed to schedule beam for link",
				logging.String("link_id", link.ID),
				logging.String("error", err.Error()),
			)
			// Continue with other links even if one fails
			continue
		}
	}

	return nil
}

// getPotentialLinks returns all links with Status == LinkStatusPotential.
// These are links that are geometrically possible but not yet activated.
func (s *Scheduler) getPotentialLinks() []*core.NetworkLink {
	allLinks := s.State.ListLinks()
	potential := make([]*core.NetworkLink, 0, len(allLinks))
	for _, link := range allLinks {
		if link != nil && link.Status == core.LinkStatusPotential {
			potential = append(potential, link)
		}
	}
	return potential
}

func (s *Scheduler) computeContactWindows(now, horizon time.Time) map[string][]contactWindow {
	windows := make(map[string][]contactWindow)
	for _, link := range s.State.ListLinks() {
		if link == nil {
			continue
		}
		if link.Status != core.LinkStatusPotential && link.Status != core.LinkStatusActive {
			continue
		}

		end := now.Add(s.linkWindowDuration(link))
		if end.After(horizon) {
			end = horizon
		}
		if !end.After(now) {
			continue
		}

		windows[link.ID] = append(windows[link.ID], contactWindow{
			start: now,
			end:   end,
		})
	}
	return windows
}

// PrecomputeContactWindows samples connectivity windows over the planning horizon.
func (s *Scheduler) PrecomputeContactWindows(ctx context.Context, now, horizon time.Time) (map[string][]contactWindow, error) {
	windows, err := s.sampleLinkWindows(ctx, now, horizon)
	if err != nil {
		s.log.Warn(ctx, "Contact window sampling failed, falling back to heuristic windows",
			logging.String("error", err.Error()),
		)
		windows = s.computeContactWindows(now, horizon)
	}
	s.contactWindows = windows
	s.log.Debug(ctx, "Precomputed contact windows",
		logging.Int("window_count", len(windows)),
		logging.String("horizon", horizon.Format(time.RFC3339)),
	)
	return windows, err
}

// RecomputeContactWindows triggers the contact window precomputation without emitting the map.
func (s *Scheduler) RecomputeContactWindows(ctx context.Context, now, horizon time.Time) {
	if _, err := s.PrecomputeContactWindows(ctx, now, horizon); err != nil {
		s.log.Info(ctx, "RecomputeContactWindows encountered an error", logging.String("error", err.Error()))
	}
}

func (s *Scheduler) contactWindowsForLink(linkID string) []contactWindow {
	if s == nil || linkID == "" {
		return nil
	}
	return s.contactWindows[linkID]
}

func (s *Scheduler) linkWindowDuration(link *core.NetworkLink) time.Duration {
	if link != nil && link.Status == core.LinkStatusActive {
		return defaultActiveWindow
	}
	return defaultPotentialWindow
}

// scheduleBeamForLink schedules UpdateBeam and DeleteBeam actions for a single link.
func (s *Scheduler) scheduleBeamForLink(ctx context.Context, link *core.NetworkLink, windows []contactWindow) error {
	if len(windows) == 0 {
		return nil
	}

	s.cleanupLinkEntries(ctx, link.ID)

	beamSpec, err := s.beamSpecFromLink(link)
	if err != nil {
		return fmt.Errorf("failed to construct BeamSpec: %w", err)
	}

	agentID, err := s.agentIDForLink(link)
	if err != nil {
		return fmt.Errorf("failed to resolve agent for link: %w", err)
	}

	const defaultBeamLeadTime = 0
	for _, window := range windows {
		if !window.end.After(window.start) {
			continue
		}

		onTime := window.start.Add(-defaultBeamLeadTime)
		if onTime.Before(s.Clock.Now()) {
			onTime = s.Clock.Now()
		}

		entryIDOn := fmt.Sprintf("link:%s:on:%d", link.ID, window.start.UnixNano())
		if err := s.sendBeamEntry(link.ID, agentID, entryIDOn, sbi.ScheduledUpdateBeam, onTime, beamSpec); err != nil {
			return err
		}

		entryIDOff := fmt.Sprintf("link:%s:off:%d", link.ID, window.end.UnixNano())
		if err := s.sendBeamEntry(link.ID, agentID, entryIDOff, sbi.ScheduledDeleteBeam, window.end, beamSpec); err != nil {
			return err
		}
	}

	start := windows[0].start
	end := windows[len(windows)-1].end
	s.log.Debug(ctx, "Scheduled beam actions for link",
		logging.String("link_id", link.ID),
		logging.String("agent_id", agentID),
		logging.String("window_start", start.Format(time.RFC3339)),
		logging.String("window_end", end.Format(time.RFC3339)),
	)

	return nil
}

func (s *Scheduler) sendBeamEntry(linkID, agentID, entryID string, actionType sbi.ScheduledActionType, when time.Time, beam *sbi.BeamSpec) error {
	if entryID == "" || agentID == "" || beam == nil {
		return fmt.Errorf("invalid beam entry parameters")
	}

	if s.scheduledEntryIDs[entryID] {
		return nil
	}

	action := &sbi.ScheduledAction{
		EntryID:   entryID,
		AgentID:   sbi.AgentID(agentID),
		Type:      actionType,
		When:      when,
		Beam:      beam,
		RequestID: "",
		SeqNo:     0,
		Token:     "",
	}

	if err := s.CDPI.SendCreateEntry(agentID, action); err != nil {
		return fmt.Errorf("failed to send %s: %w", actionType.String(), err)
	}

	s.scheduledEntryIDs[entryID] = true
	s.recordLinkEntry(linkID, scheduledEntryRef{
		entryID: entryID,
		agentID: agentID,
	})
	return nil
}

type contactWindow struct {
	start time.Time
	end   time.Time
}

func (s *Scheduler) ensureContactWindows(ctx context.Context) {
	if len(s.contactWindows) > 0 {
		return
	}
	now := s.Clock.Now()
	horizon := now.Add(ContactHorizon)
	if _, err := s.PrecomputeContactWindows(ctx, now, horizon); err != nil {
		s.log.Warn(ctx, "Failed to compute contact windows for service requests",
			logging.String("error", err.Error()),
		)
	}
}

func (s *Scheduler) linkHasWindow(linkID string, now time.Time, requireCurrent bool) bool {
	if linkID == "" {
		return false
	}
	windows := s.contactWindowsForLink(linkID)
	if len(windows) == 0 {
		return !requireCurrent
	}
	for _, window := range windows {
		if requireCurrent {
			if !window.start.After(now) && window.end.After(now) {
				return true
			}
			continue
		}
		if window.end.After(now) {
			return true
		}
	}
	return false
}

func (s *Scheduler) linkHasWindowBetween(nodeA, nodeB string, requireCurrent bool) bool {
	now := s.Clock.Now()
	link, _, _, err := s.findLinkBetweenNodes(nodeA, nodeB)
	if err != nil || link == nil {
		return false
	}
	return s.linkHasWindow(link.ID, now, requireCurrent)
}

func (s *Scheduler) pickContactWindow(now time.Time, windows []contactWindow) (time.Time, time.Time) {
	if now.IsZero() {
		now = time.Now()
	}
	if len(windows) == 0 {
		return now, now.Add(ContactHorizon)
	}

	sorted := append([]contactWindow(nil), windows...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].start.Before(sorted[j].start)
	})

	for _, window := range sorted {
		if !window.end.After(now) {
			continue
		}
		start := window.start
		if start.Before(now) {
			start = now
		}
		return start, window.end
	}

	last := sorted[len(sorted)-1]
	start := now
	if last.start.After(now) {
		start = last.start
	}
	end := last.end
	if !end.After(start) {
		end = start.Add(ContactHorizon)
	}
	return start, end
}

// beamSpecFromLink constructs a BeamSpec from a NetworkLink.
func (s *Scheduler) beamSpecFromLink(link *core.NetworkLink) (*sbi.BeamSpec, error) {
	if link == nil {
		return nil, fmt.Errorf("link is nil")
	}

	// Get interface details to determine node IDs
	// Use InterfacesByNode to get all interfaces, then find the ones we need
	interfacesByNode := s.State.InterfacesByNode()
	ifaceA := findInterfaceByRef(interfacesByNode, link.InterfaceA)
	ifaceB := findInterfaceByRef(interfacesByNode, link.InterfaceB)
	if ifaceA == nil || ifaceB == nil {
		return nil, fmt.Errorf("interface not found: ifaceA=%v, ifaceB=%v", ifaceA != nil, ifaceB != nil)
	}

	beamSpec := &sbi.BeamSpec{
		NodeID:       ifaceA.ParentNodeID,
		InterfaceID:  localInterfaceID(link.InterfaceA),
		TargetNodeID: ifaceB.ParentNodeID,
		TargetIfID:   localInterfaceID(link.InterfaceB),
		// RF parameters can be filled from transceiver models if needed
		FrequencyHz: 0,
		BandwidthHz: 0,
		PowerDBw:    0,
	}

	return beamSpec, nil
}

// agentIDForLink determines which agent should receive beam actions for a link.
// For Scope 4, we use a simple mapping: agent_id equals node_id.
// The beam is controlled by the source node (InterfaceA's parent).
func (s *Scheduler) agentIDForLink(link *core.NetworkLink) (string, error) {
	if link == nil {
		return "", fmt.Errorf("link is nil")
	}

	// Get the source interface to find its parent node
	// Use InterfacesByNode to get all interfaces, then find the one we need
	interfacesByNode := s.State.InterfacesByNode()
	ifaceA := findInterfaceByRef(interfacesByNode, link.InterfaceA)
	if ifaceA == nil {
		return "", fmt.Errorf("interface not found: %s", link.InterfaceA)
	}

	nodeID := ifaceA.ParentNodeID
	if nodeID == "" {
		return "", fmt.Errorf("interface %s has no parent node", link.InterfaceA)
	}

	// For Scope 4, agent_id equals node_id
	agentID := nodeID

	// Verify agent exists in CDPI
	if !s.CDPI.hasAgent(agentID) {
		return "", fmt.Errorf("agent %s not found in CDPI server", agentID)
	}

	return agentID, nil
}

// hasAgent checks if an agent is registered with the CDPI server.
// This is a helper method that accesses CDPI's internal state.
func (s *CDPIServer) hasAgent(agentID string) bool {
	s.agentsMu.RLock()
	defer s.agentsMu.RUnlock()
	_, exists := s.agents[agentID]
	return exists
}

// ScheduleLinkRoutes implements static single-hop route scheduling:
// - For each potential link, determine visibility intervals [T_on, T_off]
// - Schedule ScheduledSetRoute actions at T_on for both endpoints
// - Schedule ScheduledDeleteRoute actions at T_off for both endpoints
func (s *Scheduler) ScheduleLinkRoutes(ctx context.Context) error {
	now := s.Clock.Now()
	horizon := now.Add(ContactHorizon)

	// Get all potential links
	potentialLinks := s.getPotentialLinks()
	if len(potentialLinks) == 0 {
		s.log.Debug(ctx, "No potential links found for route scheduling")
		return nil
	}

	windows, _ := s.PrecomputeContactWindows(ctx, now, horizon)

	s.log.Debug(ctx, "Scheduling routes for potential links",
		logging.Int("link_count", len(potentialLinks)),
		logging.String("horizon", horizon.Format(time.RFC3339)),
	)

	// For each potential link, determine visibility windows and schedule route actions
	for _, link := range potentialLinks {
		if err := s.scheduleRoutesForLink(ctx, link, windows[link.ID]); err != nil {
			s.log.Warn(ctx, "Failed to schedule routes for link",
				logging.String("link_id", link.ID),
				logging.String("error", err.Error()),
			)
			// Continue with other links even if one fails
			continue
		}
	}

	return nil
}

// scheduleRoutesForLink schedules SetRoute and DeleteRoute actions for a single link.
// For each visibility interval [T_on, T_off]:
// - At T_on: schedule SetRoute on both endpoints (node A -> node B, node B -> node A)
// - At T_off: schedule DeleteRoute on both endpoints
func (s *Scheduler) scheduleRoutesForLink(ctx context.Context, link *core.NetworkLink, windows []contactWindow) error {
	if len(windows) == 0 {
		return nil
	}

	s.cleanupLinkEntries(ctx, link.ID)

	interfacesByNode := s.State.InterfacesByNode()
	ifaceA := findInterfaceByRef(interfacesByNode, link.InterfaceA)
	ifaceB := findInterfaceByRef(interfacesByNode, link.InterfaceB)
	if ifaceA == nil || ifaceB == nil {
		return fmt.Errorf("interface not found: ifaceA=%v, ifaceB=%v", ifaceA != nil, ifaceB != nil)
	}

	nodeAID := ifaceA.ParentNodeID
	nodeBID := ifaceB.ParentNodeID

	agentAID, err := s.agentIDForNode(nodeAID)
	if err != nil {
		return fmt.Errorf("failed to resolve agent for node A: %w", err)
	}
	agentBID, err := s.agentIDForNode(nodeBID)
	if err != nil {
		return fmt.Errorf("failed to resolve agent for node B: %w", err)
	}

	for _, window := range windows {
		if !window.end.After(window.start) {
			continue
		}

		entryIDAOn := fmt.Sprintf("route:%s:A->B:on:%d", link.ID, window.start.UnixNano())
		if !s.scheduledEntryIDs[entryIDAOn] {
			routeA := s.newRouteEntryForNode(nodeBID, link.InterfaceA)
			actionAOn := s.newSetRouteAction(entryIDAOn, sbi.AgentID(agentAID), window.start, routeA)
			if err := s.CDPI.SendCreateEntry(agentAID, actionAOn); err != nil {
				return fmt.Errorf("failed to send SetRoute for node A: %w", err)
			}
			s.recordLinkEntry(link.ID, scheduledEntryRef{
				entryID: entryIDAOn,
				agentID: agentAID,
			})
			s.scheduledEntryIDs[entryIDAOn] = true
		}

		entryIDBOn := fmt.Sprintf("route:%s:B->A:on:%d", link.ID, window.start.UnixNano())
		if !s.scheduledEntryIDs[entryIDBOn] {
			routeB := s.newRouteEntryForNode(nodeAID, link.InterfaceB)
			actionBOn := s.newSetRouteAction(entryIDBOn, sbi.AgentID(agentBID), window.start, routeB)
			if err := s.CDPI.SendCreateEntry(agentBID, actionBOn); err != nil {
				return fmt.Errorf("failed to send SetRoute for node B: %w", err)
			}
			s.recordLinkEntry(link.ID, scheduledEntryRef{
				entryID: entryIDBOn,
				agentID: agentBID,
			})
			s.scheduledEntryIDs[entryIDBOn] = true
		}

		entryIDAOff := fmt.Sprintf("route:%s:A->B:off:%d", link.ID, window.end.UnixNano())
		if !s.scheduledEntryIDs[entryIDAOff] {
			routeA := s.newRouteEntryForNode(nodeBID, link.InterfaceA)
			actionAOff := s.newDeleteRouteAction(entryIDAOff, sbi.AgentID(agentAID), window.end, routeA)
			if err := s.CDPI.SendCreateEntry(agentAID, actionAOff); err != nil {
				return fmt.Errorf("failed to send DeleteRoute for node A: %w", err)
			}
			s.recordLinkEntry(link.ID, scheduledEntryRef{
				entryID: entryIDAOff,
				agentID: agentAID,
			})
			s.scheduledEntryIDs[entryIDAOff] = true
		}

		entryIDBOff := fmt.Sprintf("route:%s:B->A:off:%d", link.ID, window.end.UnixNano())
		if !s.scheduledEntryIDs[entryIDBOff] {
			routeB := s.newRouteEntryForNode(nodeAID, link.InterfaceB)
			actionBOff := s.newDeleteRouteAction(entryIDBOff, sbi.AgentID(agentBID), window.end, routeB)
			if err := s.CDPI.SendCreateEntry(agentBID, actionBOff); err != nil {
				return fmt.Errorf("failed to send DeleteRoute for node B: %w", err)
			}
			s.recordLinkEntry(link.ID, scheduledEntryRef{
				entryID: entryIDBOff,
				agentID: agentBID,
			})
			s.scheduledEntryIDs[entryIDBOff] = true
		}
	}

	start := windows[0].start
	end := windows[len(windows)-1].end
	s.log.Debug(ctx, "Scheduled route actions for link",
		logging.String("link_id", link.ID),
		logging.String("node_a", nodeAID),
		logging.String("node_b", nodeBID),
		logging.String("window_start", start.Format(time.RFC3339)),
		logging.String("window_end", end.Format(time.RFC3339)),
	)

	return nil
}

// newRouteEntryForNode creates a RouteEntry for routing to a destination node.
// Uses a consistent DestinationCIDR scheme: "node:<nodeID>/32"
func (s *Scheduler) newRouteEntryForNode(destNodeID, outInterfaceID string) *model.RouteEntry {
	return &model.RouteEntry{
		DestinationCIDR: fmt.Sprintf("node:%s/32", destNodeID),
		NextHopNodeID:   destNodeID,
		OutInterfaceID:  outInterfaceID,
	}
}

// newSetRouteAction creates a ScheduledAction for SetRoute.
func (s *Scheduler) newSetRouteAction(entryID string, agentID sbi.AgentID, when time.Time, route *model.RouteEntry) *sbi.ScheduledAction {
	return &sbi.ScheduledAction{
		EntryID:   entryID,
		AgentID:   agentID,
		Type:      sbi.ScheduledSetRoute,
		When:      when,
		Route:     route,
		RequestID: "", // CDPI will fill this
		SeqNo:     0,  // CDPI will fill this
		Token:     "", // CDPI will fill this
	}
}

// newDeleteRouteAction creates a ScheduledAction for DeleteRoute.
func (s *Scheduler) newDeleteRouteAction(entryID string, agentID sbi.AgentID, when time.Time, route *model.RouteEntry) *sbi.ScheduledAction {
	return &sbi.ScheduledAction{
		EntryID:   entryID,
		AgentID:   agentID,
		Type:      sbi.ScheduledDeleteRoute,
		When:      when,
		Route:     route,
		RequestID: "", // CDPI will fill this
		SeqNo:     0,  // CDPI will fill this
		Token:     "", // CDPI will fill this
	}
}

// agentIDForNode determines which agent should receive actions for a node.
// For Scope 4, we use a simple mapping: agent_id equals node_id.
func (s *Scheduler) agentIDForNode(nodeID string) (string, error) {
	if nodeID == "" {
		return "", fmt.Errorf("nodeID is empty")
	}

	// For Scope 4, agent_id equals node_id
	agentID := nodeID

	// Verify agent exists in CDPI
	if !s.CDPI.hasAgent(agentID) {
		return "", fmt.Errorf("agent %s not found in CDPI server", agentID)
	}

	return agentID, nil
}

// ScheduleServiceRequests implements minimal ServiceRequest-aware scheduling:
// - For each active ServiceRequest, find a path between src and dst
// - Schedule UpdateBeam and SetRoute actions along the path
func (s *Scheduler) ScheduleServiceRequests(ctx context.Context) error {
	serviceRequests := s.State.ListServiceRequests()
	if len(serviceRequests) == 0 {
		s.log.Debug(ctx, "No service requests found for scheduling")
		return nil
	}

	sort.Slice(serviceRequests, func(i, j int) bool {
		return serviceRequests[i].Priority > serviceRequests[j].Priority
	})

	s.log.Debug(ctx, "Scheduling service requests",
		logging.Int("sr_count", len(serviceRequests)),
	)

	s.ensureContactWindows(ctx)

	// Build connectivity graph from potential/active links
	graph := s.buildConnectivityGraph()

	// For each service request, find a path and schedule actions
	for _, sr := range serviceRequests {
		if sr == nil || sr.SrcNodeID == "" || sr.DstNodeID == "" {
			srID := "nil"
			if sr != nil {
				srID = sr.ID
			}
			s.log.Warn(ctx, "Skipping invalid service request",
				logging.String("sr_id", srID),
			)
			continue
		}

		prevEntries := append([]scheduledEntryRef(nil), s.srEntries[sr.ID]...)

		// Find a path from src to dst
		requireCurrent := !sr.IsDisruptionTolerant
		path := s.findAnyPath(graph, sr.SrcNodeID, sr.DstNodeID, requireCurrent)
		if path == nil {
			s.log.Debug(ctx, "No path found for service request",
				logging.String("sr_id", sr.ID),
				logging.String("src", sr.SrcNodeID),
				logging.String("dst", sr.DstNodeID),
			)
			// Mark as not provisioned if no path found
			if err := s.updateServiceRequestStatus(ctx, sr.ID, false, nil); err != nil {
				s.log.Warn(ctx, "Failed to update service request status (no path)",
					logging.String("sr_id", sr.ID),
					logging.String("error", err.Error()),
				)
			}
			if sr.IsDisruptionTolerant {
				s.reserveStorageForSR(ctx, sr)
			}
			s.cleanupServiceRequestEntries(ctx, sr.ID, prevEntries)
			continue
		}

		// Schedule actions along the path
		entrySeen := make(map[string]bool)
		interval, entries, err := s.scheduleActionsForPath(ctx, path, sr.ID, entrySeen)
		if err != nil {
			s.log.Warn(ctx, "Failed to schedule actions for service request path",
				logging.String("sr_id", sr.ID),
				logging.String("error", err.Error()),
			)
			// Mark as not provisioned if scheduling failed
			if err := s.updateServiceRequestStatus(ctx, sr.ID, false, nil); err != nil {
				s.log.Warn(ctx, "Failed to update service request status (scheduling failed)",
					logging.String("sr_id", sr.ID),
					logging.String("error", err.Error()),
				)
			}
			s.releaseStorageForSR(ctx, sr)
			// Continue with other service requests
			continue
		}

		// Clean up previous entries now that new ones are scheduled
		s.cleanupServiceRequestEntries(ctx, sr.ID, prevEntries)
		if len(entries) > 0 {
			s.srEntries[sr.ID] = entries
		} else {
			delete(s.srEntries, sr.ID)
		}

		provisionedInterval := interval
		if provisionedInterval == nil {
			now := s.Clock.Now()
			provisionedInterval = &model.TimeInterval{
				Start: now,
				End:   now.Add(ContactHorizon),
			}
		}
		if err := s.updateServiceRequestStatus(ctx, sr.ID, true, provisionedInterval); err != nil {
			s.log.Warn(ctx, "Failed to update service request status (provisioned)",
				logging.String("sr_id", sr.ID),
				logging.String("error", err.Error()),
			)
		}
		s.releaseStorageForSR(ctx, sr)

		s.log.Debug(ctx, "Scheduled actions for service request",
			logging.String("sr_id", sr.ID),
			logging.Int("path_length", len(path)),
		)
	}

	return nil
}

func (s *Scheduler) reserveStorageForSR(ctx context.Context, sr *model.ServiceRequest) {
	if sr == nil || sr.SrcNodeID == "" {
		return
	}
	if _, exists := s.storageReservations[sr.ID]; exists {
		return
	}

	bytes := s.storageRequirementBytes(sr)
	if bytes <= 0 {
		return
	}

	if err := s.State.ReserveStorage(sr.SrcNodeID, bytes); err != nil {
		s.log.Warn(ctx, "Failed to reserve DTN storage",
			logging.String("sr_id", sr.ID),
			logging.String("node_id", sr.SrcNodeID),
			logging.String("error", err.Error()),
		)
		return
	}

	s.storageReservations[sr.ID] = bytes
	s.log.Debug(ctx, "Reserved DTN storage for service request",
		logging.String("sr_id", sr.ID),
		logging.String("node_id", sr.SrcNodeID),
		logging.String("bytes", fmt.Sprintf("%.0f", bytes)),
	)
}

func (s *Scheduler) releaseStorageForSR(ctx context.Context, sr *model.ServiceRequest) {
	if sr == nil {
		return
	}
	bytes, ok := s.storageReservations[sr.ID]
	if !ok {
		return
	}

	s.State.ReleaseStorage(sr.SrcNodeID, bytes)
	delete(s.storageReservations, sr.ID)
	s.log.Debug(ctx, "Released DTN storage for service request",
		logging.String("sr_id", sr.ID),
		logging.String("node_id", sr.SrcNodeID),
		logging.String("bytes", fmt.Sprintf("%.0f", bytes)),
	)
}

func (s *Scheduler) storageRequirementBytes(sr *model.ServiceRequest) float64 {
	if sr == nil {
		return 0
	}

	var bw float64
	for _, fr := range sr.FlowRequirements {
		bw = math.Max(bw, fr.RequestedBandwidth)
	}
	if bw == 0 {
		for _, fr := range sr.FlowRequirements {
			bw = math.Max(bw, fr.MinBandwidth)
		}
	}
	if bw == 0 {
		bw = 1e6 // default 1 Mbps
	}

	return math.Max(0, bw*(defaultDtnHold.Seconds())/8.0)
}

func (s *Scheduler) cleanupServiceRequestEntries(ctx context.Context, srID string, entries []scheduledEntryRef) {
	if len(entries) == 0 {
		delete(s.srEntries, srID)
		return
	}

	for _, entry := range entries {
		if err := s.CDPI.SendDeleteEntry(entry.agentID, entry.entryID); err != nil {
			s.log.Warn(ctx, "Failed to delete previous scheduled entry",
				logging.String("sr_id", srID),
				logging.String("agent_id", entry.agentID),
				logging.String("entry_id", entry.entryID),
				logging.String("error", err.Error()),
			)
		}
		delete(s.scheduledEntryIDs, entry.entryID)
	}
	delete(s.srEntries, srID)
}

func (s *Scheduler) cleanupLinkEntries(ctx context.Context, linkID string) {
	if linkID == "" {
		return
	}
	entries := s.linkEntries[linkID]
	if len(entries) == 0 {
		return
	}
	for _, entry := range entries {
		if err := s.CDPI.SendDeleteEntry(entry.agentID, entry.entryID); err != nil {
			s.log.Warn(ctx, "Failed to delete link entry during replan",
				logging.String("link_id", linkID),
				logging.String("agent_id", entry.agentID),
				logging.String("entry_id", entry.entryID),
				logging.String("error", err.Error()),
			)
		}
		delete(s.scheduledEntryIDs, entry.entryID)
	}
	delete(s.linkEntries, linkID)
}

func (s *Scheduler) recordLinkEntry(linkID string, entry scheduledEntryRef) {
	if linkID == "" || entry.entryID == "" {
		return
	}
	s.linkEntries[linkID] = append(s.linkEntries[linkID], entry)
}

// connectivityGraph represents a simple undirected graph of node connectivity.
type connectivityGraph struct {
	adj map[string][]string // NodeID -> neighbor NodeIDs
}

// buildConnectivityGraph builds a connectivity graph from potential/active links.
func (s *Scheduler) buildConnectivityGraph() *connectivityGraph {
	graph := &connectivityGraph{
		adj: make(map[string][]string),
	}

	// Get all links (potential or active)
	allLinks := s.State.ListLinks()
	interfacesByNode := s.State.InterfacesByNode()
	for _, link := range allLinks {
		if link == nil {
			continue
		}

		// Only include links that are potential or active (usable)
		if link.Status != core.LinkStatusPotential && link.Status != core.LinkStatusActive {
			continue
		}

		// Get node IDs from interfaces
		ifaceA := findInterfaceByRef(interfacesByNode, link.InterfaceA)
		ifaceB := findInterfaceByRef(interfacesByNode, link.InterfaceB)
		if ifaceA == nil || ifaceB == nil {
			continue
		}
		nodeAID := ifaceA.ParentNodeID
		nodeBID := ifaceB.ParentNodeID

		if nodeAID == "" || nodeBID == "" {
			continue
		}

		// Add undirected edge
		graph.addEdge(nodeAID, nodeBID)
	}

	return graph
}

// addEdge adds an undirected edge between two nodes.
func (g *connectivityGraph) addEdge(nodeA, nodeB string) {
	if g.adj[nodeA] == nil {
		g.adj[nodeA] = make([]string, 0)
	}
	if g.adj[nodeB] == nil {
		g.adj[nodeB] = make([]string, 0)
	}

	// Check if edge already exists
	for _, neighbor := range g.adj[nodeA] {
		if neighbor == nodeB {
			return // Edge already exists
		}
	}

	g.adj[nodeA] = append(g.adj[nodeA], nodeB)
	g.adj[nodeB] = append(g.adj[nodeB], nodeA)
}

// findAnyPath performs BFS to find any path from src to dst that has upcoming contact windows.
// For non-DTN flows (requireCurrent=true) it ensures each link is active now.
// Returns a slice of node IDs [src, ..., dst], or nil if no path exists.
func (s *Scheduler) findAnyPath(graph *connectivityGraph, srcNodeID, dstNodeID string, requireCurrent bool) []string {
	if srcNodeID == dstNodeID {
		return []string{srcNodeID} // Self-loop
	}

	// BFS setup
	queue := []string{srcNodeID}
	visited := make(map[string]bool)
	prev := make(map[string]string)

	visited[srcNodeID] = true

	// BFS loop
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == dstNodeID {
			// Reconstruct path
			path := make([]string, 0)
			node := dstNodeID
			for node != "" {
				path = append(path, node)
				node = prev[node]
			}
			// Reverse path (currently [dst, ..., src])
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path
		}

		neighbors := graph.adj[current]
		for _, neighbor := range neighbors {
			if visited[neighbor] {
				continue
			}
			if !s.linkHasWindowBetween(current, neighbor, requireCurrent) {
				continue
			}
			visited[neighbor] = true
			prev[neighbor] = current
			queue = append(queue, neighbor)
		}
	}

	return nil // No path found
}

// scheduleActionsForPath schedules UpdateBeam, DeleteBeam, SetRoute, and DeleteRoute
// actions for each hop in the path and returns the provisioned interval plus the
// entries that were created, which can be cleaned up later.
func (s *Scheduler) scheduleActionsForPath(_ context.Context, path []string, srID string, entrySeen map[string]bool) (*model.TimeInterval, []scheduledEntryRef, error) {
	if len(path) < 2 {
		return nil, nil, fmt.Errorf("path must have at least 2 nodes, got %d", len(path))
	}

	now := s.Clock.Now()

	var earliestStart time.Time
	var latestEnd time.Time
	var entries []scheduledEntryRef

	// For each hop (path[i] -> path[i+1])
	for i := 0; i < len(path)-1; i++ {
		nodeAID := path[i]
		nodeBID := path[i+1]

		// Find the link between nodeA and nodeB
		link, ifaceA, _, err := s.findLinkBetweenNodes(nodeAID, nodeBID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to find link between %s and %s: %w", nodeAID, nodeBID, err)
		}

		// Determine agent ID for nodeA (controlling agent for this hop)
		agentAID, err := s.agentIDForNode(nodeAID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to resolve agent for node %s: %w", nodeAID, err)
		}

		// Schedule UpdateBeam action
		beamSpec, err := s.beamSpecFromLink(link)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to construct BeamSpec: %w", err)
		}

		onTime, offTime := s.pickContactWindow(now, s.contactWindowsForLink(link.ID))

		if earliestStart.IsZero() || onTime.Before(earliestStart) {
			earliestStart = onTime
		}
		if offTime.After(latestEnd) {
			latestEnd = offTime
		}

		entryIDBeam := fmt.Sprintf("sr:%s:hop:%d:beam:%s->%s:%d", srID, i, nodeAID, nodeBID, onTime.UnixNano())
		if entrySeen == nil {
			entrySeen = make(map[string]bool)
		}
		if !entrySeen[entryIDBeam] {
			entrySeen[entryIDBeam] = true

			actionBeam := &sbi.ScheduledAction{
				EntryID:   entryIDBeam,
				AgentID:   sbi.AgentID(agentAID),
				Type:      sbi.ScheduledUpdateBeam,
				When:      onTime,
				Beam:      beamSpec,
				RequestID: "",
				SeqNo:     0,
				Token:     "",
			}

			if err := s.CDPI.SendCreateEntry(agentAID, actionBeam); err != nil {
				return nil, nil, fmt.Errorf("failed to send UpdateBeam: %w", err)
			}
			entries = append(entries, scheduledEntryRef{entryID: entryIDBeam, agentID: agentAID})
		}

		entryIDBeamOff := fmt.Sprintf("sr:%s:hop:%d:beam:%s->%s:off:%d", srID, i, nodeAID, nodeBID, offTime.UnixNano())
		if !entrySeen[entryIDBeamOff] {
			entrySeen[entryIDBeamOff] = true
			actionBeamOff := &sbi.ScheduledAction{
				EntryID:   entryIDBeamOff,
				AgentID:   sbi.AgentID(agentAID),
				Type:      sbi.ScheduledDeleteBeam,
				When:      offTime,
				Beam:      beamSpec,
				RequestID: "",
				SeqNo:     0,
				Token:     "",
			}
			if err := s.CDPI.SendCreateEntry(agentAID, actionBeamOff); err != nil {
				return nil, nil, fmt.Errorf("failed to send DeleteBeam: %w", err)
			}
			entries = append(entries, scheduledEntryRef{entryID: entryIDBeamOff, agentID: agentAID})
		}

		// Schedule SetRoute action on nodeA to reach nodeB
		route := s.newRouteEntryForNode(nodeBID, ifaceA.ID)
		entryIDRoute := fmt.Sprintf("sr:%s:hop:%d:route:%s->%s:%d", srID, i, nodeAID, nodeBID, onTime.UnixNano())
		if entrySeen[entryIDRoute] {
			// already scheduled in this run
		} else {
			entrySeen[entryIDRoute] = true
			actionRoute := s.newSetRouteAction(entryIDRoute, sbi.AgentID(agentAID), onTime, route)
			if err := s.CDPI.SendCreateEntry(agentAID, actionRoute); err != nil {
				return nil, nil, fmt.Errorf("failed to send SetRoute: %w", err)
			}
			entries = append(entries, scheduledEntryRef{entryID: entryIDRoute, agentID: agentAID})
		}

		entryIDRouteOff := fmt.Sprintf("sr:%s:hop:%d:route:%s->%s:off:%d", srID, i, nodeAID, nodeBID, offTime.UnixNano())
		if !entrySeen[entryIDRouteOff] {
			entrySeen[entryIDRouteOff] = true
			actionRouteOff := s.newDeleteRouteAction(entryIDRouteOff, sbi.AgentID(agentAID), offTime, route)
			if err := s.CDPI.SendCreateEntry(agentAID, actionRouteOff); err != nil {
				return nil, nil, fmt.Errorf("failed to send DeleteRoute: %w", err)
			}
			entries = append(entries, scheduledEntryRef{entryID: entryIDRouteOff, agentID: agentAID})
		}
	}

	if earliestStart.IsZero() {
		earliestStart = now
	}
	if latestEnd.IsZero() {
		latestEnd = earliestStart.Add(ContactHorizon)
	}

	return &model.TimeInterval{
		Start: earliestStart,
		End:   latestEnd,
	}, entries, nil
}

// updateServiceRequestStatus updates the provisioned status of a ServiceRequest.
// If provisioned is true and interval is provided, it adds the interval to ProvisionedIntervals
// and sets IsProvisionedNow to true. If provisioned is false, it sets IsProvisionedNow to false.
func (s *Scheduler) updateServiceRequestStatus(ctx context.Context, srID string, provisioned bool, interval *model.TimeInterval) error {
	sr, err := s.State.GetServiceRequest(srID)
	if err != nil {
		return fmt.Errorf("failed to get service request %s: %w", srID, err)
	}

	// Create a copy to update
	updated := *sr
	updated.IsProvisionedNow = provisioned

	if provisioned && interval != nil {
		// Add the interval to the list (avoid duplicates by checking if it already exists)
		// For simplicity, we'll just append - in a production system you might want to merge overlapping intervals
		updated.ProvisionedIntervals = append(updated.ProvisionedIntervals, *interval)
	} else if !provisioned {
		// Clear provisioned intervals when not provisioned
		// In a more sophisticated implementation, we might keep history
		updated.ProvisionedIntervals = nil
	}

	// Update via ScenarioState
	if err := s.State.UpdateServiceRequest(&updated); err != nil {
		return fmt.Errorf("failed to update service request %s: %w", srID, err)
	}

	s.log.Debug(ctx, "Updated service request status",
		logging.String("sr_id", srID),
		logging.String("provisioned", fmt.Sprintf("%v", provisioned)),
	)

	return nil
}

// findLinkBetweenNodes finds the link and interfaces connecting two nodes.
// Returns (link, ifaceA, ifaceB, error).
func (s *Scheduler) findLinkBetweenNodes(nodeAID, nodeBID string) (*core.NetworkLink, *core.NetworkInterface, *core.NetworkInterface, error) {
	allLinks := s.State.ListLinks()
	interfacesByNode := s.State.InterfacesByNode()

	for _, link := range allLinks {
		if link == nil {
			continue
		}

		// Find interfaces for this link
		ifaceA := findInterfaceByRef(interfacesByNode, link.InterfaceA)
		ifaceB := findInterfaceByRef(interfacesByNode, link.InterfaceB)

		if ifaceA == nil || ifaceB == nil {
			continue
		}

		// Check if this link connects nodeA and nodeB
		if (ifaceA.ParentNodeID == nodeAID && ifaceB.ParentNodeID == nodeBID) ||
			(ifaceA.ParentNodeID == nodeBID && ifaceB.ParentNodeID == nodeAID) {
			// Ensure ifaceA is on nodeA and ifaceB is on nodeB
			if ifaceA.ParentNodeID != nodeAID {
				ifaceA, ifaceB = ifaceB, ifaceA
			}
			return link, ifaceA, ifaceB, nil
		}
	}

	return nil, nil, nil, fmt.Errorf("no link found between %s and %s", nodeAID, nodeBID)
}

func findInterfaceByRef(interfacesByNode map[string][]*core.NetworkInterface, ref string) *core.NetworkInterface {
	if ref == "" {
		return nil
	}

	if iface := findInterfaceByExactID(interfacesByNode, ref); iface != nil {
		return iface
	}

	parent, local := splitQualifiedInterfaceID(ref)
	if local == "" {
		return nil
	}

	var matches []*core.NetworkInterface
	for _, ifaces := range interfacesByNode {
		for _, iface := range ifaces {
			if localInterfaceID(iface.ID) != local {
				continue
			}
			if parent != "" && iface.ParentNodeID != parent {
				continue
			}
			matches = append(matches, iface)
		}
	}

	if parent != "" {
		if len(matches) > 0 {
			return matches[0]
		}
		return nil
	}

	if len(matches) == 1 {
		return matches[0]
	}
	return nil
}

func findInterfaceByExactID(interfacesByNode map[string][]*core.NetworkInterface, ref string) *core.NetworkInterface {
	for _, ifaces := range interfacesByNode {
		for _, iface := range ifaces {
			if iface.ID == ref {
				return iface
			}
		}
	}
	return nil
}

func splitQualifiedInterfaceID(ref string) (parent, local string) {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", ref
}

func localInterfaceID(ifID string) string {
	if ifID == "" {
		return ""
	}
	parts := strings.SplitN(ifID, "/", 2)
	if len(parts) == 2 && parts[1] != "" {
		return parts[1]
	}
	return ifID
}
