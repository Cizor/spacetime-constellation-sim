package controller

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
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
	contactWindows map[string][]ContactWindow
	// srEntries tracks the entries created for each ServiceRequest to support cleanup.
	srEntries map[string][]scheduledEntryRef
	// linkEntries tracks entries created per link for cleanup.
	linkEntries map[string][]scheduledEntryRef
	// bandwidthReservations tracks how much bandwidth each SR reserved per link.
	bandwidthReservations map[string]map[string]uint64
	// preemptionRecords stores which SRs were preempted, why, and when.
	preemptionRecords map[string]preemptionRecord
	// powerAllocations tracks which entries have power reserved per interface.
	powerAllocations map[string]string
	// activePaths tracks currently monitored paths.
	activePaths map[string]*ActivePath
	// lastReplanAt records when ShouldReplan last triggered per SR.
	lastReplanAt map[string]time.Time
	// minReplanInterval throttles ShouldReplan to avoid thrashing.
	minReplanInterval time.Duration
	// pathFinder allows injecting a path computation function.
	pathFinder func(ctx context.Context, srcNodeID, dstNodeID string, start time.Time, horizon time.Duration) (*Path, error)
	// replanHooks centralizes calls within the re-planning loop for easier testing.
	replanHooks replanHooks
	// replanRequests signals the re-planning loop to wake up outside of the ticker.
	replanRequests chan struct{}
}

type preemptionRecord struct {
	PreemptedAt time.Time
	PreemptedBy string
	LinkIDs     []string
	Reason      string
}

type PriorityQueue struct {
	mu    sync.Mutex
	items []*model.ServiceRequest
}

func newPriorityQueue() *PriorityQueue {
	return &PriorityQueue{}
}

func (pq *PriorityQueue) Len() int {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return len(pq.items)
}

func (pq *PriorityQueue) Push(sr *model.ServiceRequest) {
	if sr == nil {
		return
	}
	pq.mu.Lock()
	defer pq.mu.Unlock()
	pq.items = append(pq.items, sr)
}

func (pq *PriorityQueue) Pop() *model.ServiceRequest {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if len(pq.items) == 0 {
		return nil
	}
	pq.sortLocked()
	sr := pq.items[0]
	pq.items = pq.items[1:]
	return sr
}

func (pq *PriorityQueue) Peek() *model.ServiceRequest {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if len(pq.items) == 0 {
		return nil
	}
	pq.sortLocked()
	return pq.items[0]
}

func (pq *PriorityQueue) SortByPriority() {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	pq.sortLocked()
}

func (pq *PriorityQueue) sortLocked() {
	sort.SliceStable(pq.items, func(i, j int) bool {
		return pq.items[i].Priority > pq.items[j].Priority
	})
}

// scheduledEntryRef captures a CDPI entry that we may need to clean up later.
type scheduledEntryRef struct {
	entryID string
	agentID string
	hopIdx  int
}

// PathHealth describes the health of an active path.
type PathHealth string

const (
	HealthHealthy  PathHealth = "healthy"
	HealthDegraded PathHealth = "degraded"
	HealthBroken   PathHealth = "broken"
)

// ActivePath captures monitoring data for a provisioned service request.
type ActivePath struct {
	ServiceRequestID string
	Path             *Path
	ScheduledActions []string
	LastUpdated      time.Time
	Health           PathHealth
	HopEntries       map[int][]scheduledEntryRef
}

// NewScheduler creates a new Scheduler with the given dependencies.
func NewScheduler(state *state.ScenarioState, clock sbi.EventScheduler, cdpi cdpiClient, log logging.Logger) *Scheduler {
	if log == nil {
		log = logging.Noop()
	}
	scheduler := &Scheduler{
		State:                 state,
		Clock:                 clock,
		CDPI:                  cdpi,
		log:                   log,
		scheduledEntryIDs:     make(map[string]bool),
		storageReservations:   make(map[string]float64),
		contactWindows:        make(map[string][]ContactWindow),
		srEntries:             make(map[string][]scheduledEntryRef),
		linkEntries:           make(map[string][]scheduledEntryRef),
		bandwidthReservations: make(map[string]map[string]uint64),
		preemptionRecords:     make(map[string]preemptionRecord),
		powerAllocations:      make(map[string]string),
		activePaths:           make(map[string]*ActivePath),
		lastReplanAt:          make(map[string]time.Time),
		minReplanInterval:     defaultReplanInterval,
	}
	scheduler.pathFinder = scheduler.FindMultiHopPath
	scheduler.replanHooks = replanHooks{
		RecomputeContactWindows: scheduler.RecomputeContactWindows,
		ScheduleLinkBeams:       scheduler.ScheduleLinkBeams,
		ScheduleLinkRoutes:      scheduler.ScheduleLinkRoutes,
		ScheduleServiceRequests: scheduler.ScheduleServiceRequests,
		ReplanActivePaths:       scheduler.replanActivePaths,
	}
	scheduler.replanRequests = make(chan struct{}, 1)
	return scheduler
}

// SetReplanInterval configures the minimum interval between ShouldReplan triggers.
func (s *Scheduler) SetReplanInterval(interval time.Duration) {
	if s == nil {
		return
	}
	if interval < 0 {
		interval = 0
	}
	s.minReplanInterval = interval
}

const (
	ContactHorizon         = 1 * time.Hour
	defaultActiveWindow    = 45 * time.Minute
	defaultPotentialWindow = 20 * time.Minute
	defaultDtnHold         = 30 * time.Second
	defaultReplanInterval  = 15 * time.Second
	betterWindowExtension  = 15 * time.Second
)

type replanHooks struct {
	RecomputeContactWindows func(ctx context.Context, now, horizon time.Time)
	ScheduleLinkBeams       func(ctx context.Context) error
	ScheduleLinkRoutes      func(ctx context.Context) error
	ScheduleServiceRequests func(ctx context.Context) error
	ReplanActivePaths       func(ctx context.Context, now time.Time)
}

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

func (s *Scheduler) computeContactWindows(now, horizon time.Time) map[string][]ContactWindow {
	windows := make(map[string][]ContactWindow)
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

		windows[link.ID] = append(windows[link.ID], ContactWindow{
			StartTime: now,
			EndTime:   end,
			Quality:   s.linkSNR(link.ID),
		})
	}
	return windows
}

// PrecomputeContactWindows samples connectivity windows over the planning horizon.
func (s *Scheduler) PrecomputeContactWindows(ctx context.Context, now, horizon time.Time) (map[string][]ContactWindow, error) {
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

func (s *Scheduler) contactWindowsForLink(linkID string) []ContactWindow {
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
func (s *Scheduler) scheduleBeamForLink(ctx context.Context, link *core.NetworkLink, windows []ContactWindow) error {
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
		if !window.EndTime.After(window.StartTime) {
			continue
		}

		onTime := window.StartTime.Add(-defaultBeamLeadTime)
		if onTime.Before(s.Clock.Now()) {
			onTime = s.Clock.Now()
		}

		entryIDOn := fmt.Sprintf("link:%s:on:%d", link.ID, window.StartTime.UnixNano())
		if err := s.sendBeamEntry(link.ID, agentID, link.InterfaceA, entryIDOn, sbi.ScheduledUpdateBeam, onTime, beamSpec); err != nil {
			return err
		}

		entryIDOff := fmt.Sprintf("link:%s:off:%d", link.ID, window.EndTime.UnixNano())
		if err := s.sendBeamEntry(link.ID, agentID, link.InterfaceA, entryIDOff, sbi.ScheduledDeleteBeam, window.EndTime, beamSpec); err != nil {
			return err
		}
	}

	start := windows[0].StartTime
	end := windows[len(windows)-1].EndTime
	s.log.Debug(ctx, "Scheduled beam actions for link",
		logging.String("link_id", link.ID),
		logging.String("agent_id", agentID),
		logging.String("window_start", start.Format(time.RFC3339)),
		logging.String("window_end", end.Format(time.RFC3339)),
	)

	return nil
}

func (s *Scheduler) sendBeamEntry(linkID, agentID, interfaceID, entryID string, actionType sbi.ScheduledActionType, when time.Time, beam *sbi.BeamSpec) error {
	if entryID == "" || agentID == "" || beam == nil {
		return fmt.Errorf("invalid beam entry parameters")
	}

	if s.scheduledEntryIDs[entryID] {
		return nil
	}
	var allocated bool
	if actionType == sbi.ScheduledUpdateBeam {
		powerWatts := s.beamPowerWatts(interfaceID)
		if err := s.allocatePowerForEntry(entryID, interfaceID, powerWatts); err != nil {
			return fmt.Errorf("allocate power for entry %s failed: %w", entryID, err)
		}
		allocated = true
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
		if allocated {
			s.releasePowerForEntry(entryID)
		}
		return fmt.Errorf("failed to send %s: %w", actionType.String(), err)
	}

	s.scheduledEntryIDs[entryID] = true
	s.recordLinkEntry(linkID, scheduledEntryRef{
		entryID: entryID,
		agentID: agentID,
	})
	return nil
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
			if !window.StartTime.After(now) && window.EndTime.After(now) {
				return true
			}
			continue
		}
		if window.EndTime.After(now) {
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

func (s *Scheduler) pickContactWindow(now time.Time, windows []ContactWindow) (time.Time, time.Time) {
	if now.IsZero() {
		now = time.Now()
	}
	if len(windows) == 0 {
		return now, now.Add(ContactHorizon)
	}

	sorted := append([]ContactWindow(nil), windows...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartTime.Before(sorted[j].StartTime)
	})

	for _, window := range sorted {
		if !window.EndTime.After(now) {
			continue
		}
		start := window.StartTime
		if start.Before(now) {
			start = now
		}
		return start, window.EndTime
	}

	last := sorted[len(sorted)-1]
	start := now
	if last.StartTime.After(now) {
		start = last.StartTime
	}
	end := last.EndTime
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

	_, dbw := s.beamPowerMetrics(link.InterfaceA)
	beamSpec := &sbi.BeamSpec{
		NodeID:       ifaceA.ParentNodeID,
		InterfaceID:  localInterfaceID(link.InterfaceA),
		TargetNodeID: ifaceB.ParentNodeID,
		TargetIfID:   localInterfaceID(link.InterfaceB),
		// RF parameters can be filled from transceiver models if needed
		FrequencyHz: 0,
		BandwidthHz: 0,
		PowerDBw:    dbw,
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
func (s *Scheduler) scheduleRoutesForLink(ctx context.Context, link *core.NetworkLink, windows []ContactWindow) error {
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
		if !window.EndTime.After(window.StartTime) {
			continue
		}

		entryIDAOn := fmt.Sprintf("route:%s:A->B:on:%d", link.ID, window.StartTime.UnixNano())
		if !s.scheduledEntryIDs[entryIDAOn] {
			routeA := s.newRouteEntryForNode(nodeBID, link.InterfaceA)
			actionAOn := s.newSetRouteAction(entryIDAOn, sbi.AgentID(agentAID), window.StartTime, routeA)
			if err := s.CDPI.SendCreateEntry(agentAID, actionAOn); err != nil {
				return fmt.Errorf("failed to send SetRoute for node A: %w", err)
			}
			s.recordLinkEntry(link.ID, scheduledEntryRef{
				entryID: entryIDAOn,
				agentID: agentAID,
			})
			s.scheduledEntryIDs[entryIDAOn] = true
		}

		entryIDBOn := fmt.Sprintf("route:%s:B->A:on:%d", link.ID, window.StartTime.UnixNano())
		if !s.scheduledEntryIDs[entryIDBOn] {
			routeB := s.newRouteEntryForNode(nodeAID, link.InterfaceB)
			actionBOn := s.newSetRouteAction(entryIDBOn, sbi.AgentID(agentBID), window.StartTime, routeB)
			if err := s.CDPI.SendCreateEntry(agentBID, actionBOn); err != nil {
				return fmt.Errorf("failed to send SetRoute for node B: %w", err)
			}
			s.recordLinkEntry(link.ID, scheduledEntryRef{
				entryID: entryIDBOn,
				agentID: agentBID,
			})
			s.scheduledEntryIDs[entryIDBOn] = true
		}

		entryIDAOff := fmt.Sprintf("route:%s:A->B:off:%d", link.ID, window.EndTime.UnixNano())
		if !s.scheduledEntryIDs[entryIDAOff] {
			routeA := s.newRouteEntryForNode(nodeBID, link.InterfaceA)
			actionAOff := s.newDeleteRouteAction(entryIDAOff, sbi.AgentID(agentAID), window.EndTime, routeA)
			if err := s.CDPI.SendCreateEntry(agentAID, actionAOff); err != nil {
				return fmt.Errorf("failed to send DeleteRoute for node A: %w", err)
			}
			s.recordLinkEntry(link.ID, scheduledEntryRef{
				entryID: entryIDAOff,
				agentID: agentAID,
			})
			s.scheduledEntryIDs[entryIDAOff] = true
		}

		entryIDBOff := fmt.Sprintf("route:%s:B->A:off:%d", link.ID, window.EndTime.UnixNano())
		if !s.scheduledEntryIDs[entryIDBOff] {
			routeB := s.newRouteEntryForNode(nodeAID, link.InterfaceB)
			actionBOff := s.newDeleteRouteAction(entryIDBOff, sbi.AgentID(agentBID), window.EndTime, routeB)
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

	start := windows[0].StartTime
	end := windows[len(windows)-1].EndTime
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

	pq := newPriorityQueue()
	for _, sr := range serviceRequests {
		pq.Push(sr)
	}

	s.log.Debug(ctx, "Scheduling service requests",
		logging.Int("sr_count", len(serviceRequests)),
	)

	s.ensureContactWindows(ctx)

	// For each service request, find a path and schedule actions
	for pq.Len() > 0 {
		sr := pq.Pop()
		if sr != nil {
			s.removeActivePath(sr.ID)
		}
		s.ReleasePathCapacity(sr.ID)
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

		path, err := s.FindMultiHopPath(ctx, sr.SrcNodeID, sr.DstNodeID, s.Clock.Now(), ContactHorizon)
		if err != nil {
			s.log.Debug(ctx, "No path found for service request",
				logging.String("sr_id", sr.ID),
				logging.String("src", sr.SrcNodeID),
				logging.String("dst", sr.DstNodeID),
				logging.String("error", err.Error()),
			)
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

		if err := s.validatePath(ctx, path); err != nil {
			s.log.Warn(ctx, "Invalid path for service request",
				logging.String("sr_id", sr.ID),
				logging.String("error", err.Error()),
			)
			if err := s.updateServiceRequestStatus(ctx, sr.ID, false, nil); err != nil {
				s.log.Warn(ctx, "Failed to update service request status (invalid path)",
					logging.String("sr_id", sr.ID),
					logging.String("error", err.Error()),
				)
			}
			s.cleanupServiceRequestEntries(ctx, sr.ID, prevEntries)
			continue
		}

		pathNodes := pathNodeSequence(path)
		if len(pathNodes) < 2 {
			s.log.Warn(ctx, "Invalid path node sequence",
				logging.String("sr_id", sr.ID),
			)
			if err := s.updateServiceRequestStatus(ctx, sr.ID, false, nil); err != nil {
				s.log.Warn(ctx, "Failed to update service request status (invalid path)",
					logging.String("sr_id", sr.ID),
					logging.String("error", err.Error()),
				)
			}
			s.cleanupServiceRequestEntries(ctx, sr.ID, prevEntries)
			continue
		}

		// Schedule actions along the path
		requiredBps := s.requiredBandwidthForSR(sr)
		hasCapacity, constrainedLinks := s.CheckPathCapacity(pathNodes, requiredBps)
		if !hasCapacity {
			preempted, err := s.preemptConflictingSRs(ctx, sr, pathNodes, requiredBps, constrainedLinks)
			if err != nil {
				s.log.Warn(ctx, "Failed to preempt lower-priority service requests",
					logging.String("sr_id", sr.ID),
					logging.String("error", err.Error()),
				)
			} else if preempted {
				s.log.Debug(ctx, "Preempted lower-priority service requests",
					logging.String("sr_id", sr.ID),
				)
			}
			if preempted {
				hasCapacity, constrainedLinks = s.CheckPathCapacity(pathNodes, requiredBps)
			}
		}
		if !hasCapacity {
			s.log.Warn(ctx, "Insufficient bandwidth for service request",
				logging.String("sr_id", sr.ID),
				logging.String("constrained_links", strings.Join(constrainedLinks, ",")),
			)
			if err := s.updateServiceRequestStatus(ctx, sr.ID, false, nil); err != nil {
				s.log.Warn(ctx, "Failed to update service request status (bandwidth)", logging.String("sr_id", sr.ID), logging.String("error", err.Error()))
			}
			s.cleanupServiceRequestEntries(ctx, sr.ID, prevEntries)
			continue
		}

		if err := s.AllocatePathCapacity(pathNodes, sr.ID, requiredBps); err != nil {
			s.log.Warn(ctx, "Failed to reserve bandwidth for service request",
				logging.String("sr_id", sr.ID),
				logging.String("error", err.Error()),
			)
			if err := s.updateServiceRequestStatus(ctx, sr.ID, false, nil); err != nil {
				s.log.Warn(ctx, "Failed to update service request status (bandwidth alloc)", logging.String("sr_id", sr.ID), logging.String("error", err.Error()))
			}
			s.cleanupServiceRequestEntries(ctx, sr.ID, prevEntries)
			continue
		}

		interval, entries, hopEntries, err := s.schedulePathHops(ctx, sr, path)
		if err != nil {
			s.log.Warn(ctx, "Failed to schedule actions for service request path",
				logging.String("sr_id", sr.ID),
				logging.String("error", err.Error()),
			)
			// Release allocated bandwidth before abandoning
			s.ReleasePathCapacity(sr.ID)
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
			s.recordActivePath(sr.ID, path, entries, hopEntries)
		} else {
			delete(s.srEntries, sr.ID)
			s.removeActivePath(sr.ID)
		}

		provisionedInterval := interval
		if provisionedInterval == nil {
			now := s.Clock.Now()
			provisionedInterval = &model.TimeInterval{
				StartTime: now,
				EndTime:   now.Add(ContactHorizon),
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
			logging.Int("path_length", len(path.Hops)),
		)
	}

	return nil
}

// RequestReplan signals the re-planning loop to wake up immediately.
func (s *Scheduler) RequestReplan() {
	if s == nil || s.replanRequests == nil {
		return
	}
	select {
	case s.replanRequests <- struct{}{}:
	default:
	}
}

// RunReplanningLoop runs the periodic re-planning cycle, including beam and route refreshes
// as well as active path evaluations. The loop exits when the provided context is canceled.
func (s *Scheduler) RunReplanningLoop(ctx context.Context, interval time.Duration) {
	if s == nil || ctx == nil {
		return
	}
	if interval <= 0 {
		interval = defaultReplanInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.replanTick(ctx)
		case <-s.replanRequests:
			s.drainReplanRequests()
			s.replanTick(ctx)
		}
	}
}

func (s *Scheduler) replanTick(ctx context.Context) {
	if s == nil {
		return
	}
	now := s.Clock.Now()
	horizon := now.Add(ContactHorizon)

	if s.replanHooks.RecomputeContactWindows != nil {
		s.replanHooks.RecomputeContactWindows(ctx, now, horizon)
	}
	if err := s.replanHooks.ScheduleLinkBeams(ctx); err != nil {
		s.log.Warn(ctx, "Replanning failed to schedule link beams",
			logging.String("error", err.Error()),
		)
	}
	if err := s.replanHooks.ScheduleLinkRoutes(ctx); err != nil {
		s.log.Warn(ctx, "Replanning failed to schedule link routes",
			logging.String("error", err.Error()),
		)
	}
	if err := s.replanHooks.ScheduleServiceRequests(ctx); err != nil {
		s.log.Warn(ctx, "Replanning failed to schedule service requests",
			logging.String("error", err.Error()),
		)
	}
	if s.replanHooks.ReplanActivePaths != nil {
		s.replanHooks.ReplanActivePaths(ctx, now)
	}
}

func (s *Scheduler) drainReplanRequests() {
	if s == nil || s.replanRequests == nil {
		return
	}
	for {
		select {
		case <-s.replanRequests:
		default:
			return
		}
	}
}

func (s *Scheduler) replanActivePaths(ctx context.Context, now time.Time) {
	if s == nil || len(s.activePaths) == 0 {
		return
	}
	for srID := range s.activePaths {
		if !s.ShouldReplan(srID, now) {
			continue
		}
		s.replanActivePath(ctx, srID, now)
	}
}

func (s *Scheduler) replanActivePath(ctx context.Context, srID string, now time.Time) {
	if srID == "" {
		return
	}
	ap := s.activePaths[srID]
	if ap == nil || ap.Path == nil {
		return
	}
	sr, err := s.State.GetServiceRequest(srID)
	if err != nil || sr == nil || !sr.IsProvisionedNow {
		return
	}
	if sr.SrcNodeID == "" || sr.DstNodeID == "" {
		return
	}

	newPath, err := s.pathFinder(ctx, sr.SrcNodeID, sr.DstNodeID, now, ContactHorizon)
	if err != nil {
		s.log.Warn(ctx, "Replanning failed to compute a candidate path",
			logging.String("sr_id", srID),
			logging.String("error", err.Error()),
		)
		return
	}
	if newPath == nil {
		s.log.Warn(ctx, "Replanning found no path to apply",
			logging.String("sr_id", srID),
		)
		return
	}
	if s.pathsEqual(ap.Path, newPath) {
		return
	}
	if err := s.UpdatePath(ctx, srID, newPath); err != nil {
		s.log.Warn(ctx, "Failed to apply replanned path",
			logging.String("sr_id", srID),
			logging.String("error", err.Error()),
		)
		return
	}
	s.log.Info(ctx, "Replanned service request path",
		logging.String("sr_id", srID),
	)
}

func pathNodeSequence(path *Path) []string {
	if path == nil || len(path.Hops) == 0 {
		return nil
	}
	nodes := make([]string, 0, len(path.Hops)+1)
	nodes = append(nodes, path.Hops[0].FromNodeID)
	for _, hop := range path.Hops {
		nodes = append(nodes, hop.ToNodeID)
	}
	return nodes
}

func (s *Scheduler) validatePath(ctx context.Context, path *Path) error {
	if path == nil || len(path.Hops) == 0 {
		return fmt.Errorf("path is empty")
	}

	for _, hop := range path.Hops {
		if hop.FromNodeID == "" || hop.ToNodeID == "" || hop.LinkID == "" {
			return fmt.Errorf("path hop missing identifiers")
		}
		if hop.EndTime.Before(hop.StartTime) || hop.EndTime.Equal(hop.StartTime) {
			return fmt.Errorf("invalid hop time window for link %s", hop.LinkID)
		}
		if _, _, err := s.State.GetNode(hop.FromNodeID); err != nil {
			return fmt.Errorf("from node %s missing: %w", hop.FromNodeID, err)
		}
		if _, _, err := s.State.GetNode(hop.ToNodeID); err != nil {
			return fmt.Errorf("to node %s missing: %w", hop.ToNodeID, err)
		}
		if _, err := s.State.GetLink(hop.LinkID); err != nil {
			return fmt.Errorf("link %s missing: %w", hop.LinkID, err)
		}
		windowValid := false
		for _, window := range s.contactWindowsForLink(hop.LinkID) {
			if !hop.StartTime.Before(window.StartTime) && !hop.EndTime.After(window.EndTime) {
				windowValid = true
				break
			}
		}
		if !windowValid {
			return fmt.Errorf("hop %s-%s outside of known contact windows", hop.FromNodeID, hop.ToNodeID)
		}
	}
	return nil
}

func (s *Scheduler) scheduleHopActions(ctx context.Context, sr *model.ServiceRequest, hop PathHop, hopIdx int) ([]scheduledEntryRef, time.Time, time.Time, error) {
	link, ifaceA, _, err := s.findLinkBetweenNodes(hop.FromNodeID, hop.ToNodeID)
	if err != nil {
		return nil, time.Time{}, time.Time{}, fmt.Errorf("unable to find link for hop: %w", err)
	}

	agentID, err := s.agentIDForNode(hop.FromNodeID)
	if err != nil {
		return nil, time.Time{}, time.Time{}, fmt.Errorf("unable to resolve agent for hop: %w", err)
	}

	beamSpec, err := s.beamSpecFromLink(link)
	if err != nil {
		return nil, time.Time{}, time.Time{}, fmt.Errorf("failed to build beam spec for hop: %w", err)
	}

	entries := make([]scheduledEntryRef, 0, 4)

	entryIDOn := fmt.Sprintf("sr:%s:hop:%d:beam:%s->%s:%d", sr.ID, hopIdx, hop.FromNodeID, hop.ToNodeID, hop.StartTime.UnixNano())
	if err := s.sendBeamEntry(link.ID, agentID, ifaceA.ID, entryIDOn, sbi.ScheduledUpdateBeam, hop.StartTime, beamSpec); err != nil {
		return nil, time.Time{}, time.Time{}, err
	}
	entries = append(entries, scheduledEntryRef{entryID: entryIDOn, agentID: agentID, hopIdx: hopIdx})

	entryIDOff := fmt.Sprintf("sr:%s:hop:%d:beam:%s->%s:off:%d", sr.ID, hopIdx, hop.FromNodeID, hop.ToNodeID, hop.EndTime.UnixNano())
	if err := s.sendBeamEntry(link.ID, agentID, ifaceA.ID, entryIDOff, sbi.ScheduledDeleteBeam, hop.EndTime, beamSpec); err != nil {
		return nil, time.Time{}, time.Time{}, err
	}
	entries = append(entries, scheduledEntryRef{entryID: entryIDOff, agentID: agentID, hopIdx: hopIdx})

	routeEntry := s.newRouteEntryForNode(hop.ToNodeID, ifaceA.ID)
	entryIDRoute := fmt.Sprintf("sr:%s:hop:%d:route:%s->%s:%d", sr.ID, hopIdx, hop.FromNodeID, hop.ToNodeID, hop.StartTime.UnixNano())
	action := s.newSetRouteAction(entryIDRoute, sbi.AgentID(agentID), hop.StartTime, routeEntry)
	if err := s.CDPI.SendCreateEntry(agentID, action); err != nil {
		return nil, time.Time{}, time.Time{}, fmt.Errorf("failed to send SetRoute for hop %d: %w", hopIdx, err)
	}
	entries = append(entries, scheduledEntryRef{entryID: entryIDRoute, agentID: agentID, hopIdx: hopIdx})

	entryIDRouteOff := fmt.Sprintf("sr:%s:hop:%d:route:%s->%s:off:%d", sr.ID, hopIdx, hop.FromNodeID, hop.ToNodeID, hop.EndTime.UnixNano())
	action = s.newDeleteRouteAction(entryIDRouteOff, sbi.AgentID(agentID), hop.EndTime, routeEntry)
	if err := s.CDPI.SendCreateEntry(agentID, action); err != nil {
		return nil, time.Time{}, time.Time{}, fmt.Errorf("failed to send DeleteRoute for hop %d: %w", hopIdx, err)
	}
	entries = append(entries, scheduledEntryRef{entryID: entryIDRouteOff, agentID: agentID, hopIdx: hopIdx})

	return entries, hop.StartTime, hop.EndTime, nil
}

func (s *Scheduler) schedulePathHops(ctx context.Context, sr *model.ServiceRequest, path *Path) (*model.TimeInterval, []scheduledEntryRef, map[int][]scheduledEntryRef, error) {
	if sr == nil || path == nil || len(path.Hops) == 0 {
		return nil, nil, nil, fmt.Errorf("invalid path for scheduling")
	}

	var earliest time.Time
	var latest time.Time
	entries := []scheduledEntryRef{}
	hopEntries := make(map[int][]scheduledEntryRef)

	for i, hop := range path.Hops {
		link, ifaceA, _, err := s.findLinkBetweenNodes(hop.FromNodeID, hop.ToNodeID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to find link for hop %d: %w", i, err)
		}

		agentID, err := s.agentIDForNode(hop.FromNodeID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to resolve agent for hop %d: %w", i, err)
		}

		beamSpec, err := s.beamSpecFromLink(link)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to build beam spec for hop %d: %w", i, err)
		}

		hopEntries[i] = []scheduledEntryRef{}

		entryIDOn := fmt.Sprintf("sr:%s:hop:%d:beam:%s->%s:%d", sr.ID, i, hop.FromNodeID, hop.ToNodeID, hop.StartTime.UnixNano())
		if err := s.sendBeamEntry(link.ID, agentID, ifaceA.ID, entryIDOn, sbi.ScheduledUpdateBeam, hop.StartTime, beamSpec); err != nil {
			return nil, nil, nil, err
		}
		ref := scheduledEntryRef{entryID: entryIDOn, agentID: agentID, hopIdx: i}
		entries = append(entries, ref)
		hopEntries[i] = append(hopEntries[i], ref)

		entryIDOff := fmt.Sprintf("sr:%s:hop:%d:beam:%s->%s:off:%d", sr.ID, i, hop.FromNodeID, hop.ToNodeID, hop.EndTime.UnixNano())
		if err := s.sendBeamEntry(link.ID, agentID, ifaceA.ID, entryIDOff, sbi.ScheduledDeleteBeam, hop.EndTime, beamSpec); err != nil {
			return nil, nil, nil, err
		}
		ref = scheduledEntryRef{entryID: entryIDOff, agentID: agentID, hopIdx: i}
		entries = append(entries, ref)
		hopEntries[i] = append(hopEntries[i], ref)

		routeEntry := s.newRouteEntryForNode(hop.ToNodeID, ifaceA.ID)
		entryIDRoute := fmt.Sprintf("sr:%s:hop:%d:route:%s->%s:%d", sr.ID, i, hop.FromNodeID, hop.ToNodeID, hop.StartTime.UnixNano())
		action := s.newSetRouteAction(entryIDRoute, sbi.AgentID(agentID), hop.StartTime, routeEntry)
		if err := s.CDPI.SendCreateEntry(agentID, action); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to send SetRoute for hop %d: %w", i, err)
		}
		ref = scheduledEntryRef{entryID: entryIDRoute, agentID: agentID, hopIdx: i}
		entries = append(entries, ref)
		hopEntries[i] = append(hopEntries[i], ref)

		entryIDRouteOff := fmt.Sprintf("sr:%s:hop:%d:route:%s->%s:off:%d", sr.ID, i, hop.FromNodeID, hop.ToNodeID, hop.EndTime.UnixNano())
		action = s.newDeleteRouteAction(entryIDRouteOff, sbi.AgentID(agentID), hop.EndTime, routeEntry)
		if err := s.CDPI.SendCreateEntry(agentID, action); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to send DeleteRoute for hop %d: %w", i, err)
		}
		ref = scheduledEntryRef{entryID: entryIDRouteOff, agentID: agentID, hopIdx: i}
		entries = append(entries, ref)
		hopEntries[i] = append(hopEntries[i], ref)

		if earliest.IsZero() || hop.StartTime.Before(earliest) {
			earliest = hop.StartTime
		}
		if latest.IsZero() || hop.EndTime.After(latest) {
			latest = hop.EndTime
		}
	}

	if earliest.IsZero() {
		earliest = s.Clock.Now()
	}
	if latest.IsZero() {
		latest = earliest.Add(ContactHorizon)
	}

	return &model.TimeInterval{StartTime: earliest, EndTime: latest}, entries, hopEntries, nil
}

func (s *Scheduler) recordActivePath(srID string, path *Path, entries []scheduledEntryRef, hopEntries map[int][]scheduledEntryRef) {
	if srID == "" || path == nil {
		return
	}
	actionIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.entryID != "" {
			actionIDs = append(actionIDs, entry.entryID)
		}
	}
	s.activePaths[srID] = &ActivePath{
		ServiceRequestID: srID,
		Path:             path,
		ScheduledActions: actionIDs,
		LastUpdated:      s.Clock.Now(),
		Health:           s.CheckPathHealth(path, s.Clock.Now()),
		HopEntries:       hopEntries,
	}
}

func (s *Scheduler) removeActivePath(srID string) {
	if srID == "" {
		return
	}
	delete(s.activePaths, srID)
}

func (s *Scheduler) hopEqual(a, b PathHop) bool {
	return a.LinkID != "" && a.LinkID == b.LinkID &&
		a.FromNodeID == b.FromNodeID &&
		a.ToNodeID == b.ToNodeID &&
		a.StartTime.Equal(b.StartTime) &&
		a.EndTime.Equal(b.EndTime)
}

func (s *Scheduler) pathsEqual(a, b *Path) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.Hops) != len(b.Hops) {
		return false
	}
	for i := range a.Hops {
		if !s.hopEqual(a.Hops[i], b.Hops[i]) {
			return false
		}
	}
	return true
}

// ComputePathDiff identifies shared, removed, and added hops between two paths.
func (s *Scheduler) ComputePathDiff(oldPath, newPath *Path) PathDiff {
	if oldPath == nil || newPath == nil {
		return PathDiff{}
	}

	minLen := len(oldPath.Hops)
	if len(newPath.Hops) < minLen {
		minLen = len(newPath.Hops)
	}
	shared := 0
	for shared < minLen && s.hopEqual(oldPath.Hops[shared], newPath.Hops[shared]) {
		shared++
	}

	return PathDiff{
		SharedHops:  append([]PathHop(nil), oldPath.Hops[:shared]...),
		RemovedHops: append([]PathHop(nil), oldPath.Hops[shared:]...),
		AddedHops:   append([]PathHop(nil), newPath.Hops[shared:]...),
	}
}

// UpdatePath adjusts the scheduled actions for a service request when a new path
// becomes available. It only schedules the added hops, removes the obsolete hops,
// and updates the active path state accordingly.
func (s *Scheduler) UpdatePath(ctx context.Context, srID string, newPath *Path) error {
	if s == nil {
		return fmt.Errorf("scheduler is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if srID == "" {
		return fmt.Errorf("service request ID is empty")
	}
	if newPath == nil {
		return fmt.Errorf("new path is nil")
	}

	sr, err := s.State.GetServiceRequest(srID)
	if err != nil {
		return fmt.Errorf("get service request: %w", err)
	}
	if sr == nil {
		return fmt.Errorf("service request %s not found", srID)
	}
	if err := s.validatePath(ctx, newPath); err != nil {
		return fmt.Errorf("validate new path: %w", err)
	}
	ap := s.activePaths[srID]
	if ap == nil || ap.Path == nil {
		return fmt.Errorf("no active path to update for service request %s", srID)
	}
	if !sr.IsProvisionedNow {
		return fmt.Errorf("service request %s is not provisioned", srID)
	}

	diff := s.ComputePathDiff(ap.Path, newPath)
	shared := len(diff.SharedHops)
	requiredBps := s.requiredBandwidthForSR(sr)
	newHopEntries := make(map[int][]scheduledEntryRef)
	scheduled := make([]int, 0, len(newPath.Hops))

	for idx := shared; idx < len(newPath.Hops); idx++ {
		hop := newPath.Hops[idx]
		if err := s.reserveHopCapacity(srID, hop, requiredBps); err != nil {
			s.cleanupNewHopReservations(ctx, srID, newPath, scheduled, newHopEntries)
			return fmt.Errorf("reserve bandwidth for hop %d: %w", idx, err)
		}
		entries, _, _, err := s.scheduleHopActions(ctx, sr, hop, idx)
		if err != nil {
			s.cleanupHopEntries(ctx, srID, entries)
			s.releaseHopCapacity(ctx, srID, hop)
			s.cleanupNewHopReservations(ctx, srID, newPath, scheduled, newHopEntries)
			return fmt.Errorf("schedule hop %d: %w", idx, err)
		}
		newHopEntries[idx] = entries
		scheduled = append(scheduled, idx)
	}

	for idx := shared; idx < len(ap.Path.Hops); idx++ {
		entries := ap.HopEntries[idx]
		s.cleanupHopEntries(ctx, srID, entries)
		s.releaseHopCapacity(ctx, srID, ap.Path.Hops[idx])
		delete(ap.HopEntries, idx)
	}

	for idx, entries := range newHopEntries {
		ap.HopEntries[idx] = entries
	}
	ap.Path = newPath
	ap.ScheduledActions = s.collectActionIDs(ap)
	ap.LastUpdated = s.Clock.Now()
	ap.Health = s.CheckPathHealth(newPath, s.Clock.Now())
	s.syncActivePathEntries(srID)

	interval := pathInterval(newPath)
	if err := s.updateServiceRequestStatus(ctx, srID, true, interval); err != nil {
		return fmt.Errorf("update service request status: %w", err)
	}

	return nil
}

func (s *Scheduler) reserveHopCapacity(srID string, hop PathHop, bps uint64) error {
	if hop.LinkID == "" || bps == 0 {
		return nil
	}
	if _, ok := s.bandwidthReservations[srID]; !ok {
		s.bandwidthReservations[srID] = make(map[string]uint64)
	}
	if _, exists := s.bandwidthReservations[srID][hop.LinkID]; exists {
		return nil
	}
	if err := s.State.ReserveBandwidth(hop.LinkID, bps); err != nil {
		return err
	}
	s.bandwidthReservations[srID][hop.LinkID] = bps
	return nil
}

func (s *Scheduler) releaseHopCapacity(ctx context.Context, srID string, hop PathHop) {
	if hop.LinkID == "" {
		return
	}
	reserved, ok := s.bandwidthReservations[srID]
	if !ok {
		return
	}
	bps, ok := reserved[hop.LinkID]
	if !ok {
		return
	}
	if err := s.State.ReleaseBandwidth(hop.LinkID, bps); err != nil {
		s.log.Warn(ctx, "Release bandwidth failed for hop", logging.String("link_id", hop.LinkID),
			logging.String("sr_id", srID), logging.String("error", err.Error()))
	}
	delete(reserved, hop.LinkID)
	if len(reserved) == 0 {
		delete(s.bandwidthReservations, srID)
	}
}

func pathInterval(path *Path) *model.TimeInterval {
	if path == nil || len(path.Hops) == 0 {
		return nil
	}
	return &model.TimeInterval{
		StartTime: path.Hops[0].StartTime,
		EndTime:   path.Hops[len(path.Hops)-1].EndTime,
	}
}

func (s *Scheduler) cleanupNewHopReservations(ctx context.Context, srID string, path *Path, hopIdxs []int, hopEntries map[int][]scheduledEntryRef) {
	if path == nil {
		return
	}
	for _, idx := range hopIdxs {
		s.cleanupHopEntries(ctx, srID, hopEntries[idx])
		s.releaseHopCapacity(ctx, srID, path.Hops[idx])
		delete(hopEntries, idx)
	}
}

func (s *Scheduler) collectActionIDs(ap *ActivePath) []string {
	if ap == nil || ap.Path == nil {
		return nil
	}
	actionIDs := make([]string, 0)
	for idx := 0; idx < len(ap.Path.Hops); idx++ {
		actionIDs = append(actionIDs, collectEntryIDs(ap.HopEntries[idx])...)
	}
	return actionIDs
}

func collectEntryIDs(entries []scheduledEntryRef) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.entryID != "" {
			ids = append(ids, entry.entryID)
		}
	}
	return ids
}

func (s *Scheduler) syncActivePathEntries(srID string) {
	ap := s.activePaths[srID]
	if ap == nil || ap.Path == nil {
		delete(s.srEntries, srID)
		return
	}
	srEntries := make([]scheduledEntryRef, 0)
	for idx := 0; idx < len(ap.Path.Hops); idx++ {
		srEntries = append(srEntries, ap.HopEntries[idx]...)
	}
	s.srEntries[srID] = srEntries
}

func (s *Scheduler) CheckPathHealth(path *Path, now time.Time) PathHealth {
	if path == nil || len(path.Hops) == 0 {
		return HealthBroken
	}
	if now.IsZero() {
		now = s.Clock.Now()
	}
	health := HealthHealthy

	for _, hop := range path.Hops {
		if hop.EndTime.IsZero() || now.After(hop.EndTime) {
			return HealthBroken
		}
		windows := s.contactWindowsForLink(hop.LinkID)
		if len(windows) == 0 {
			health = worseHealth(health, HealthDegraded)
			continue
		}
		valid := false
		for _, window := range windows {
			if !hop.StartTime.Before(window.StartTime) && !hop.EndTime.After(window.EndTime) {
				valid = true
				if now.After(window.EndTime) {
					return HealthBroken
				}
				break
			}
		}
		if !valid {
			health = worseHealth(health, HealthDegraded)
		}
	}

	return health
}

// ShouldReplan evaluates whether the given service request needs a re-planning pass.
func (s *Scheduler) ShouldReplan(srID string, now time.Time) bool {
	if s == nil || srID == "" {
		return false
	}
	if now.IsZero() {
		now = s.Clock.Now()
	}

	sr, err := s.State.GetServiceRequest(srID)
	if err != nil || sr == nil || !sr.IsProvisionedNow {
		return false
	}

	ap := s.activePaths[srID]
	if ap == nil || ap.Path == nil {
		if s.canReplan(srID, now, false) {
			s.markReplan(srID, now)
			return true
		}
		return false
	}

	health := s.CheckPathHealth(ap.Path, now)
	if health == HealthBroken {
		if s.canReplan(srID, now, true) {
			s.markReplan(srID, now)
			return true
		}
		return false
	}
	if health == HealthDegraded {
		if s.canReplan(srID, now, false) {
			s.markReplan(srID, now)
			return true
		}
		return false
	}

	if s.hasExtendedWindow(ap) {
		if s.canReplan(srID, now, false) {
			s.markReplan(srID, now)
			return true
		}
		return false
	}

	if s.isOverlappedByHigherPriority(sr, ap.Path) {
		if s.canReplan(srID, now, false) {
			s.markReplan(srID, now)
			return true
		}
		return false
	}

	return false
}

func (s *Scheduler) canReplan(srID string, now time.Time, bypassInterval bool) bool {
	if srID == "" {
		return false
	}
	if bypassInterval || s.minReplanInterval <= 0 {
		return true
	}
	last := s.lastReplanAt[srID]
	if last.IsZero() {
		return true
	}
	return now.Sub(last) >= s.minReplanInterval
}

func (s *Scheduler) markReplan(srID string, now time.Time) {
	if srID == "" {
		return
	}
	if s.lastReplanAt == nil {
		s.lastReplanAt = make(map[string]time.Time)
	}
	s.lastReplanAt[srID] = now
}

func (s *Scheduler) hasExtendedWindow(ap *ActivePath) bool {
	if ap == nil || ap.Path == nil {
		return false
	}
	validUntil := ap.Path.ValidUntil
	if validUntil.IsZero() && len(ap.Path.Hops) > 0 {
		validUntil = ap.Path.Hops[len(ap.Path.Hops)-1].EndTime
	}
	if validUntil.IsZero() {
		return false
	}

	for _, hop := range ap.Path.Hops {
		for _, window := range s.contactWindowsForLink(hop.LinkID) {
			if window.EndTime.Sub(validUntil) >= betterWindowExtension {
				return true
			}
		}
	}

	return false
}

func (s *Scheduler) isOverlappedByHigherPriority(sr *model.ServiceRequest, path *Path) bool {
	if sr == nil || path == nil {
		return false
	}
	linkSet := make(map[string]struct{})
	for _, hop := range path.Hops {
		if hop.LinkID != "" {
			linkSet[hop.LinkID] = struct{}{}
		}
	}
	if len(linkSet) == 0 {
		return false
	}
	for otherID, reserved := range s.bandwidthReservations {
		if otherID == sr.ID {
			continue
		}
		other, err := s.State.GetServiceRequest(otherID)
		if err != nil || other == nil {
			continue
		}
		if other.Priority <= sr.Priority {
			continue
		}
		for linkID := range reserved {
			if _, ok := linkSet[linkID]; ok {
				return true
			}
		}
	}
	return false
}

func worseHealth(current, candidate PathHealth) PathHealth {
	if current == HealthBroken || candidate == HealthBroken {
		return HealthBroken
	}
	if current == HealthDegraded || candidate == HealthDegraded {
		return HealthDegraded
	}
	return HealthHealthy
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
		s.releasePowerForEntry(entry.entryID)
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

func (s *Scheduler) cleanupHopEntries(ctx context.Context, srID string, entries []scheduledEntryRef) {
	if len(entries) == 0 {
		return
	}
	for _, entry := range entries {
		s.releasePowerForEntry(entry.entryID)
		if err := s.CDPI.SendDeleteEntry(entry.agentID, entry.entryID); err != nil {
			s.log.Warn(ctx, "Failed to delete hop entry",
				logging.String("sr_id", srID),
				logging.String("agent_id", entry.agentID),
				logging.String("entry_id", entry.entryID),
				logging.String("error", err.Error()),
			)
		}
		delete(s.scheduledEntryIDs, entry.entryID)
	}
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
		s.releasePowerForEntry(entry.entryID)
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

func (s *Scheduler) beamPowerMetrics(interfaceID string) (float64, float64) {
	if interfaceID == "" {
		return 1, 0
	}
	netKB := s.State.NetworkKB()
	if netKB == nil {
		return 1, 0
	}
	iface := netKB.GetNetworkInterface(interfaceID)
	if iface == nil {
		return 1, 0
	}
	trx := netKB.GetTransceiverModel(iface.TransceiverID)
	if trx == nil {
		return 1, 0
	}
	dbw := trx.TxPowerDBw
	watts := math.Pow(10, dbw/10)
	if math.IsNaN(watts) || watts <= 0 {
		watts = 1
	}
	return watts, dbw
}

func (s *Scheduler) beamPowerWatts(interfaceID string) float64 {
	watts, _ := s.beamPowerMetrics(interfaceID)
	return watts
}

func (s *Scheduler) allocatePowerForEntry(entryID, interfaceID string, powerWatts float64) error {
	if entryID == "" || interfaceID == "" {
		return fmt.Errorf("entry ID and interface ID are required")
	}
	if powerWatts < 0 {
		powerWatts = 0
	}
	if err := s.State.AllocatePower(interfaceID, entryID, powerWatts); err != nil {
		return err
	}
	s.powerAllocations[entryID] = interfaceID
	return nil
}

func (s *Scheduler) releasePowerForEntry(entryID string) {
	if entryID == "" {
		return
	}
	interfaceID, ok := s.powerAllocations[entryID]
	if !ok || interfaceID == "" {
		return
	}
	s.State.ReleasePower(interfaceID, entryID)
	delete(s.powerAllocations, entryID)
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

func (s *Scheduler) requiredBandwidthForSR(sr *model.ServiceRequest) uint64 {
	if sr == nil {
		return 1_000_000
	}
	var bps float64
	for _, fr := range sr.FlowRequirements {
		bps = math.Max(bps, fr.RequestedBandwidth)
	}
	if bps == 0 {
		for _, fr := range sr.FlowRequirements {
			bps = math.Max(bps, fr.MinBandwidth)
		}
	}
	if bps == 0 {
		bps = 1_000_000
	}
	return uint64(bps)
}

func (s *Scheduler) linkIDsForPath(path []string) ([]string, error) {
	if len(path) < 2 {
		return nil, fmt.Errorf("path must contain at least two nodes")
	}
	linkIDs := make([]string, 0, len(path)-1)
	for i := 0; i < len(path)-1; i++ {
		a, b := path[i], path[i+1]
		link, _, _, err := s.findLinkBetweenNodes(a, b)
		if err != nil {
			return nil, err
		}
		linkIDs = append(linkIDs, link.ID)
	}
	return linkIDs, nil
}

func (s *Scheduler) CheckPathCapacity(path []string, requiredBps uint64) (bool, []string) {
	linkIDs, err := s.linkIDsForPath(path)
	if err != nil {
		return false, nil
	}
	constrained := make([]string, 0)
	for _, linkID := range linkIDs {
		avail, err := s.State.GetAvailableBandwidth(linkID)
		if err != nil {
			return false, []string{linkID}
		}
		unlimited := false
		if avail == 0 {
			if linkInfo, err := s.State.GetLink(linkID); err == nil {
				if linkInfo.MaxBandwidthBps == 0 {
					unlimited = true
				}
			}
		}
		if !unlimited && avail < requiredBps {
			constrained = append(constrained, linkID)
		}
	}
	return len(constrained) == 0, constrained
}

func (s *Scheduler) AllocatePathCapacity(path []string, srID string, bps uint64) error {
	if len(path) < 2 || bps == 0 {
		return nil
	}
	linkIDs, err := s.linkIDsForPath(path)
	if err != nil {
		return err
	}
	reserved := make(map[string]uint64, len(linkIDs))
	for _, linkID := range linkIDs {
		if err := s.State.ReserveBandwidth(linkID, bps); err != nil {
			for rid, amount := range reserved {
				_ = s.State.ReleaseBandwidth(rid, amount)
			}
			return err
		}
		reserved[linkID] = bps
	}
	s.bandwidthReservations[srID] = reserved
	return nil
}

func (s *Scheduler) ReleasePathCapacity(srID string) {
	reserved, ok := s.bandwidthReservations[srID]
	if !ok {
		return
	}
	for linkID, bps := range reserved {
		if err := s.State.ReleaseBandwidth(linkID, bps); err != nil {
			s.log.Warn(context.Background(), "Release bandwidth failed",
				logging.String("link_id", linkID),
				logging.String("sr_id", srID),
				logging.String("error", err.Error()),
			)
		}
	}
	delete(s.bandwidthReservations, srID)
}

func (s *Scheduler) preemptConflictingSRs(ctx context.Context, incoming *model.ServiceRequest, path []string, requiredBps uint64, constrainedLinks []string) (bool, error) {
	if incoming == nil || len(constrainedLinks) == 0 {
		return false, nil
	}
	candidates := s.collectPreemptionCandidates(incoming.Priority, constrainedLinks)
	if len(candidates) == 0 {
		return false, nil
	}

	for _, candidate := range candidates {
		reason := fmt.Sprintf("preempted by service request %s", incoming.ID)
		if err := s.preemptServiceRequest(ctx, candidate, incoming.ID, constrainedLinks, reason); err != nil {
			return false, err
		}
		if ok, _ := s.CheckPathCapacity(path, requiredBps); ok {
			return true, nil
		}
	}

	ok, _ := s.CheckPathCapacity(path, requiredBps)
	return ok, nil
}

func (s *Scheduler) collectPreemptionCandidates(priority int32, constrainedLinks []string) []*model.ServiceRequest {
	if len(constrainedLinks) == 0 {
		return nil
	}
	linkSet := make(map[string]struct{}, len(constrainedLinks))
	for _, linkID := range constrainedLinks {
		linkSet[linkID] = struct{}{}
	}

	seen := make(map[string]struct{})
	var result []*model.ServiceRequest

	for srID, reserved := range s.bandwidthReservations {
		if _, ok := seen[srID]; ok {
			continue
		}
		intersects := false
		for linkID := range reserved {
			if _, ok := linkSet[linkID]; ok {
				intersects = true
				break
			}
		}
		if !intersects {
			continue
		}
		sr, err := s.State.GetServiceRequest(srID)
		if err != nil || sr == nil {
			continue
		}
		if sr.Priority >= priority {
			continue
		}
		seen[srID] = struct{}{}
		result = append(result, sr)
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Priority != result[j].Priority {
			return result[i].Priority < result[j].Priority
		}
		return result[i].ID < result[j].ID
	})

	return result
}

func (s *Scheduler) preemptServiceRequest(ctx context.Context, sr *model.ServiceRequest, preemptingID string, linkIDs []string, reason string) error {
	if sr == nil {
		return fmt.Errorf("service request is nil")
	}

	interval := s.currentProvisioningInterval(sr.ID)
	s.ReleasePathCapacity(sr.ID)
	entries := append([]scheduledEntryRef(nil), s.srEntries[sr.ID]...)
	s.cleanupServiceRequestEntries(ctx, sr.ID, entries)
	s.releaseStorageForSR(ctx, sr)
	if err := s.updateServiceRequestStatus(ctx, sr.ID, false, interval); err != nil {
		return err
	}

	s.recordPreemption(sr.ID, preemptionRecord{
		PreemptedAt: s.Clock.Now(),
		PreemptedBy: preemptingID,
		LinkIDs:     append([]string(nil), linkIDs...),
		Reason:      reason,
	})

	return nil
}

func (s *Scheduler) currentProvisioningInterval(srID string) *model.TimeInterval {
	status, err := s.State.GetServiceRequestStatus(srID)
	if err != nil {
		return nil
	}
	if status.CurrentInterval == nil {
		now := s.Clock.Now()
		return &model.TimeInterval{
			StartTime: now,
			EndTime:   now,
		}
	}
	interval := *status.CurrentInterval
	if interval.EndTime.IsZero() {
		interval.EndTime = s.Clock.Now()
	}
	return &interval
}

func (s *Scheduler) recordPreemption(srID string, record preemptionRecord) {
	if srID == "" {
		return
	}
	s.preemptionRecords[srID] = record
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
