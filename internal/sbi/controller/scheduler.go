package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/model"
)

// Scheduler implements the controller-side scheduling logic for Scope 4.
// It coordinates between ScenarioState (links, nodes), EventScheduler (time),
// and CDPIServer (sending actions to agents).
type Scheduler struct {
	State *state.ScenarioState
	Clock sbi.EventScheduler
	CDPI  *CDPIServer
	log   logging.Logger

	// scheduledEntryIDs tracks entry IDs we've already scheduled to avoid duplicates.
	// This provides idempotency for ScheduleLinkBeams.
	scheduledEntryIDs map[string]bool
}

// NewScheduler creates a new Scheduler with the given dependencies.
func NewScheduler(state *state.ScenarioState, clock sbi.EventScheduler, cdpi *CDPIServer, log logging.Logger) *Scheduler {
	if log == nil {
		log = logging.Noop()
	}
	return &Scheduler{
		State:             state,
		Clock:             clock,
		CDPI:              cdpi,
		log:               log,
		scheduledEntryIDs: make(map[string]bool),
	}
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

	// 3. (Later) ServiceRequest-aware scheduling, in subsequent issues.
	return nil
}

// ScheduleLinkBeams implements link-driven beam scheduling:
// - For each potential link, determine visibility intervals [T_on, T_off]
// - Schedule ScheduledUpdateBeam at T_on and ScheduledDeleteBeam at T_off
// - Send actions to the appropriate agent via CDPI
func (s *Scheduler) ScheduleLinkBeams(ctx context.Context) error {
	now := s.Clock.Now()
	horizon := now.Add(1 * time.Hour) // Fixed 1-hour planning horizon for now

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

	// For each potential link, determine visibility windows and schedule actions
	for _, link := range potentialLinks {
		if err := s.scheduleBeamForLink(ctx, link, now, horizon); err != nil {
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

// scheduleBeamForLink schedules UpdateBeam and DeleteBeam actions for a single link.
// For now, we use a simplified approach:
// - Assume the link is available from now until horizon
// - Schedule UpdateBeam at now (or T_on if we compute it)
// - Schedule DeleteBeam at horizon (or T_off if we compute it)
func (s *Scheduler) scheduleBeamForLink(ctx context.Context, link *core.NetworkLink, now, horizon time.Time) error {
	// For now, use a simple approach: assume link is available from now to horizon
	// TODO: In future, compute actual visibility windows by sampling connectivity
	T_on := now
	T_off := horizon

	// Compute onTime with optional lead time (clamped to now)
	const defaultBeamLeadTime = 0 // Start with zero lead time
	onTime := T_on.Add(-defaultBeamLeadTime)
	if onTime.Before(now) {
		onTime = now
	}

	// Construct BeamSpec from link
	beamSpec, err := s.beamSpecFromLink(link)
	if err != nil {
		return fmt.Errorf("failed to construct BeamSpec: %w", err)
	}

	// Determine which agent owns this beam (which node controls it)
	// For now, use the source node (InterfaceA's parent node)
	agentID, err := s.agentIDForLink(link)
	if err != nil {
		return fmt.Errorf("failed to resolve agent for link: %w", err)
	}

	// Create UpdateBeam action
	entryIDOn := fmt.Sprintf("link:%s:on:%d", link.ID, T_on.UnixNano())
	if s.scheduledEntryIDs[entryIDOn] {
		// Already scheduled, skip
		return nil
	}

	actionOn := &sbi.ScheduledAction{
		EntryID:  entryIDOn,
		AgentID:  sbi.AgentID(agentID),
		Type:     sbi.ScheduledUpdateBeam,
		When:     onTime,
		Beam:     beamSpec,
		RequestID: "", // CDPI will fill this
		SeqNo:    0,   // CDPI will fill this
		Token:    "",  // CDPI will fill this
	}

	if err := s.CDPI.SendCreateEntry(agentID, actionOn); err != nil {
		return fmt.Errorf("failed to send UpdateBeam: %w", err)
	}
	s.scheduledEntryIDs[entryIDOn] = true

	// Create DeleteBeam action
	entryIDOff := fmt.Sprintf("link:%s:off:%d", link.ID, T_off.UnixNano())
	if s.scheduledEntryIDs[entryIDOff] {
		// Already scheduled, skip
		return nil
	}

	actionOff := &sbi.ScheduledAction{
		EntryID:   entryIDOff,
		AgentID:   sbi.AgentID(agentID),
		Type:      sbi.ScheduledDeleteBeam,
		When:      T_off,
		Beam:      beamSpec,
		RequestID: "", // CDPI will fill this
		SeqNo:     0,  // CDPI will fill this
		Token:     "", // CDPI will fill this
	}

	if err := s.CDPI.SendCreateEntry(agentID, actionOff); err != nil {
		return fmt.Errorf("failed to send DeleteBeam: %w", err)
	}
	s.scheduledEntryIDs[entryIDOff] = true

	s.log.Debug(ctx, "Scheduled beam actions for link",
		logging.String("link_id", link.ID),
		logging.String("agent_id", agentID),
		logging.String("on_time", onTime.Format(time.RFC3339)),
		logging.String("off_time", T_off.Format(time.RFC3339)),
	)

	return nil
}

// beamSpecFromLink constructs a BeamSpec from a NetworkLink.
func (s *Scheduler) beamSpecFromLink(link *core.NetworkLink) (*sbi.BeamSpec, error) {
	if link == nil {
		return nil, fmt.Errorf("link is nil")
	}

	// Get interface details to determine node IDs
	// Use InterfacesByNode to get all interfaces, then find the ones we need
	interfacesByNode := s.State.InterfacesByNode()
	var ifaceA, ifaceB *core.NetworkInterface
	for _, ifaces := range interfacesByNode {
		for _, iface := range ifaces {
			if iface.ID == link.InterfaceA {
				ifaceA = iface
			}
			if iface.ID == link.InterfaceB {
				ifaceB = iface
			}
		}
	}
	if ifaceA == nil || ifaceB == nil {
		return nil, fmt.Errorf("interface not found: ifaceA=%v, ifaceB=%v", ifaceA != nil, ifaceB != nil)
	}

	beamSpec := &sbi.BeamSpec{
		NodeID:       ifaceA.ParentNodeID,
		InterfaceID:  link.InterfaceA,
		TargetNodeID: ifaceB.ParentNodeID,
		TargetIfID:   link.InterfaceB,
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
	var ifaceA *core.NetworkInterface
	for _, ifaces := range interfacesByNode {
		for _, iface := range ifaces {
			if iface.ID == link.InterfaceA {
				ifaceA = iface
				break
			}
		}
		if ifaceA != nil {
			break
		}
	}
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
	horizon := now.Add(1 * time.Hour) // Fixed 1-hour planning horizon for now

	// Get all potential links
	potentialLinks := s.getPotentialLinks()
	if len(potentialLinks) == 0 {
		s.log.Debug(ctx, "No potential links found for route scheduling")
		return nil
	}

	s.log.Debug(ctx, "Scheduling routes for potential links",
		logging.Int("link_count", len(potentialLinks)),
		logging.String("horizon", horizon.Format(time.RFC3339)),
	)

	// For each potential link, determine visibility windows and schedule route actions
	for _, link := range potentialLinks {
		if err := s.scheduleRoutesForLink(ctx, link, now, horizon); err != nil {
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
func (s *Scheduler) scheduleRoutesForLink(ctx context.Context, link *core.NetworkLink, now, horizon time.Time) error {
	// For now, use a simple approach: assume link is available from now to horizon
	// TODO: In future, compute actual visibility windows by sampling connectivity
	T_on := now
	T_off := horizon

	// Get interface details to determine node IDs
	interfacesByNode := s.State.InterfacesByNode()
	var ifaceA, ifaceB *core.NetworkInterface
	for _, ifaces := range interfacesByNode {
		for _, iface := range ifaces {
			if iface.ID == link.InterfaceA {
				ifaceA = iface
			}
			if iface.ID == link.InterfaceB {
				ifaceB = iface
			}
		}
	}
	if ifaceA == nil || ifaceB == nil {
		return fmt.Errorf("interface not found: ifaceA=%v, ifaceB=%v", ifaceA != nil, ifaceB != nil)
	}

	nodeAID := ifaceA.ParentNodeID
	nodeBID := ifaceB.ParentNodeID

	// Determine agent IDs for both nodes
	agentAID, err := s.agentIDForNode(nodeAID)
	if err != nil {
		return fmt.Errorf("failed to resolve agent for node A: %w", err)
	}
	agentBID, err := s.agentIDForNode(nodeBID)
	if err != nil {
		return fmt.Errorf("failed to resolve agent for node B: %w", err)
	}

	// Schedule SetRoute actions at T_on for both endpoints
	// Node A -> Node B
	entryIDAOn := fmt.Sprintf("route:%s:A->B:on:%d", link.ID, T_on.UnixNano())
	if !s.scheduledEntryIDs[entryIDAOn] {
		routeA := s.newRouteEntryForNode(nodeBID, link.InterfaceA)
		actionAOn := s.newSetRouteAction(entryIDAOn, sbi.AgentID(agentAID), T_on, routeA)
		if err := s.CDPI.SendCreateEntry(agentAID, actionAOn); err != nil {
			return fmt.Errorf("failed to send SetRoute for node A: %w", err)
		}
		s.scheduledEntryIDs[entryIDAOn] = true
	}

	// Node B -> Node A
	entryIDBOn := fmt.Sprintf("route:%s:B->A:on:%d", link.ID, T_on.UnixNano())
	if !s.scheduledEntryIDs[entryIDBOn] {
		routeB := s.newRouteEntryForNode(nodeAID, link.InterfaceB)
		actionBOn := s.newSetRouteAction(entryIDBOn, sbi.AgentID(agentBID), T_on, routeB)
		if err := s.CDPI.SendCreateEntry(agentBID, actionBOn); err != nil {
			return fmt.Errorf("failed to send SetRoute for node B: %w", err)
		}
		s.scheduledEntryIDs[entryIDBOn] = true
	}

	// Schedule DeleteRoute actions at T_off for both endpoints
	// Node A -> Node B
	entryIDAOff := fmt.Sprintf("route:%s:A->B:off:%d", link.ID, T_off.UnixNano())
	if !s.scheduledEntryIDs[entryIDAOff] {
		routeA := s.newRouteEntryForNode(nodeBID, link.InterfaceA)
		actionAOff := s.newDeleteRouteAction(entryIDAOff, sbi.AgentID(agentAID), T_off, routeA)
		if err := s.CDPI.SendCreateEntry(agentAID, actionAOff); err != nil {
			return fmt.Errorf("failed to send DeleteRoute for node A: %w", err)
		}
		s.scheduledEntryIDs[entryIDAOff] = true
	}

	// Node B -> Node A
	entryIDBOff := fmt.Sprintf("route:%s:B->A:off:%d", link.ID, T_off.UnixNano())
	if !s.scheduledEntryIDs[entryIDBOff] {
		routeB := s.newRouteEntryForNode(nodeAID, link.InterfaceB)
		actionBOff := s.newDeleteRouteAction(entryIDBOff, sbi.AgentID(agentBID), T_off, routeB)
		if err := s.CDPI.SendCreateEntry(agentBID, actionBOff); err != nil {
			return fmt.Errorf("failed to send DeleteRoute for node B: %w", err)
		}
		s.scheduledEntryIDs[entryIDBOff] = true
	}

	s.log.Debug(ctx, "Scheduled route actions for link",
		logging.String("link_id", link.ID),
		logging.String("node_a", nodeAID),
		logging.String("node_b", nodeBID),
		logging.String("on_time", T_on.Format(time.RFC3339)),
		logging.String("off_time", T_off.Format(time.RFC3339)),
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

