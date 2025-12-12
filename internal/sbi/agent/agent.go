package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/model"
	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SimAgent represents a simulated agent for a network node.
// It owns a local schedule, executes actions at the right sim time,
// updates the KB, and drives Telemetry + Responses.
type SimAgent struct {
	// Identity
	AgentID sbi.AgentID // from Hello (maps to node/platform ID)
	NodeID  string      // the node this agent represents

	// Dependencies
	State        *state.ScenarioState
	Scheduler    sbi.EventScheduler
	TelemetryCli telemetrypb.TelemetryClient
	Stream       grpc.BidiStreamingClient[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController]

	// Internal state
	mu      sync.Mutex
	pending map[string]*sbi.ScheduledAction // keyed by EntryID
	token   string                           // schedule_manipulation_token

	// Telemetry state
	telemetryMu  sync.Mutex
	telemetryInterval time.Duration
	bytesTx      map[string]uint64 // per-interface transmitted bytes (monotonic)
	lastTick     time.Time         // last telemetry tick time

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// NewSimAgent creates a new simulated agent with the given ID and dependencies.
// It uses default telemetry configuration.
func NewSimAgent(agentID sbi.AgentID, nodeID string, state *state.ScenarioState, scheduler sbi.EventScheduler, telemetryCli telemetrypb.TelemetryClient, stream grpc.BidiStreamingClient[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController]) *SimAgent {
	return NewSimAgentWithConfig(agentID, nodeID, state, scheduler, telemetryCli, stream, DefaultTelemetryConfig())
}

// NewSimAgentWithConfig creates a new simulated agent with telemetry configuration.
func NewSimAgentWithConfig(agentID sbi.AgentID, nodeID string, state *state.ScenarioState, scheduler sbi.EventScheduler, telemetryCli telemetrypb.TelemetryClient, stream grpc.BidiStreamingClient[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController], telemetryConfig TelemetryConfig) *SimAgent {
	cfg := telemetryConfig.ApplyDefaults()
	return &SimAgent{
		AgentID:           agentID,
		NodeID:            nodeID,
		State:             state,
		Scheduler:         scheduler,
		TelemetryCli:      telemetryCli,
		Stream:            stream,
		pending:           make(map[string]*sbi.ScheduledAction),
		token:             generateToken(),
		telemetryInterval: cfg.Interval,
		bytesTx:           make(map[string]uint64),
		lastTick:          time.Time{},
	}
}

// ID returns the agent's identifier.
func (a *SimAgent) ID() sbi.AgentID {
	return a.AgentID
}

// HandleScheduledAction accepts a scheduled action from the controller.
// This is the interface method used by the controller to send actions to agents.
// The action is inserted into the agent's local schedule and executed at the correct time.
func (a *SimAgent) HandleScheduledAction(ctx context.Context, action *sbi.ScheduledAction) error {
	// Validate the action
	if err := action.Validate(); err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Insert into pending map
	a.pending[action.EntryID] = action

	// Schedule execution at the correct simulation time
	eventID := a.Scheduler.Schedule(action.When, func() {
		a.execute(action)
	})

	// Store event ID in action for cancellation (we can extend ScheduledAction if needed)
	// For now, we'll use the EntryID as the event ID since they should match
	_ = eventID

	return nil
}

// Start connects to the CDPI stream, sends Hello, and starts the read loop.
// It returns an error if the stream cannot be established or Hello fails.
func (a *SimAgent) Start(ctx context.Context) error {
	if a.Stream == nil {
		return fmt.Errorf("agent %s: stream is nil", a.AgentID)
	}

	// Create context for agent lifecycle
	a.ctx, a.cancel = context.WithCancel(ctx)

	// Send Hello message
	hello := &schedulingpb.ReceiveRequestsMessageToController{
		Hello: &schedulingpb.ReceiveRequestsMessageToController_Hello{
			AgentId: string(a.AgentID),
		},
	}

	if err := a.Stream.Send(hello); err != nil {
		return fmt.Errorf("agent %s: failed to send Hello: %w", a.AgentID, err)
	}

	// Start read loop in a goroutine
	go a.readLoop()

	// Start telemetry loop if TelemetryCli is available and interval is set
	if a.TelemetryCli != nil && a.telemetryInterval > 0 {
		a.startTelemetryLoop()
	}

	return nil
}

// Stop cancels the agent's context and drains queues.
func (a *SimAgent) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cancel != nil {
		a.cancel()
	}

	// Clear pending actions
	a.pending = make(map[string]*sbi.ScheduledAction)
}

// readLoop reads messages from the CDPI stream and processes them.
// It handles CreateEntryRequest, DeleteEntryRequest, FinalizeRequest,
// and SetSrPolicy/DeleteSrPolicy messages.
func (a *SimAgent) readLoop() {
	for {
		select {
		case <-a.ctx.Done():
			return
		default:
			msg, err := a.Stream.Recv()
			if err != nil {
				// Stream closed or error - agent should handle reconnection
				// For now, just exit the loop
				return
			}

			if err := a.handleMessage(msg); err != nil {
				// Log error but continue processing
				// TODO: Add proper logging
				_ = err
			}
		}
	}
}

// handleMessage processes a single message from the controller.
func (a *SimAgent) handleMessage(msg *schedulingpb.ReceiveRequestsMessageFromController) error {
	// Extract request ID for response correlation
	requestID := msg.GetRequestId()

	// Handle different message types
	switch {
	case msg.GetCreateEntry() != nil:
		return a.handleCreateEntry(requestID, msg.GetCreateEntry())
	case msg.GetDeleteEntry() != nil:
		return a.handleDeleteEntry(msg.GetDeleteEntry())
	case msg.GetFinalize() != nil:
		return a.handleFinalize(msg.GetFinalize())
	default:
		// Unknown message type - ignore for now
		return nil
	}
}

// handleCreateEntry processes a CreateEntryRequest message.
// It validates the token, converts the proto to ScheduledAction, and schedules it.
func (a *SimAgent) handleCreateEntry(requestID int64, req *schedulingpb.CreateEntryRequest) error {
	// Validate token
	a.mu.Lock()
	tokenMatch := req.GetScheduleManipulationToken() == a.token
	a.mu.Unlock()

	if !tokenMatch {
		// Token mismatch - log and ignore (forgiving behavior for Scope 4)
		// TODO: Add proper logging
		return nil
	}

	// Convert proto to ScheduledAction
	action, err := a.convertCreateEntryToAction(req, requestID)
	if err != nil {
		return fmt.Errorf("failed to convert CreateEntryRequest: %w", err)
	}

	// Insert into pending and schedule
	return a.HandleScheduledAction(a.ctx, action)
}

// handleDeleteEntry processes a DeleteEntryRequest message.
// It cancels the previously scheduled action by EntryID.
func (a *SimAgent) handleDeleteEntry(req *schedulingpb.DeleteEntryRequest) error {
	entryID := req.GetId()
	if entryID == "" {
		return fmt.Errorf("DeleteEntryRequest has empty entry ID")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Find and cancel the scheduled action
	if action, exists := a.pending[entryID]; exists {
		// Cancel the scheduled event (using EntryID as event ID)
		a.Scheduler.Cancel(entryID)
		delete(a.pending, entryID)
		_ = action // TODO: Send response if needed
	}

	return nil
}

// handleFinalize processes a FinalizeRequest message.
// It drops any pending entries with When < cutoffTime.
func (a *SimAgent) handleFinalize(req *schedulingpb.FinalizeRequest) error {
	// Validate token
	a.mu.Lock()
	tokenMatch := req.GetScheduleManipulationToken() == a.token
	a.mu.Unlock()

	if !tokenMatch {
		// Token mismatch - log and ignore
		// TODO: Add proper logging
		return nil
	}

	// Extract cutoff time
	cutoffTime, err := a.extractTimeFromFinalize(req)
	if err != nil {
		return fmt.Errorf("failed to extract cutoff time: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Drop entries with When < cutoff
	for entryID, action := range a.pending {
		if action.When.Before(cutoffTime) {
			// Cancel and remove
			a.Scheduler.Cancel(entryID)
			delete(a.pending, entryID)
		}
	}

	return nil
}

// convertCreateEntryToAction converts a CreateEntryRequest proto to a ScheduledAction.
func (a *SimAgent) convertCreateEntryToAction(req *schedulingpb.CreateEntryRequest, requestID int64) (*sbi.ScheduledAction, error) {
	// Extract timing
	when, err := a.extractTimeFromCreateEntry(req)
	if err != nil {
		return nil, fmt.Errorf("failed to extract time: %w", err)
	}

	// Extract metadata
	meta := sbi.ActionMeta{
		RequestID: fmt.Sprintf("%d", requestID),
		SeqNo:     int64(req.GetSeqno()),
		Token:     req.GetScheduleManipulationToken(),
	}

	// Determine action type and payload based on ConfigurationChange
	var actionType sbi.ScheduledActionType
	var beam *sbi.BeamSpec
	var route *model.RouteEntry
	var srPolicy *sbi.SrPolicySpec

	switch {
	case req.GetUpdateBeam() != nil:
		actionType = sbi.ScheduledUpdateBeam
		beam, err = a.convertUpdateBeamToBeamSpec(req.GetUpdateBeam())
		if err != nil {
			return nil, fmt.Errorf("failed to convert UpdateBeam: %w", err)
		}
	case req.GetDeleteBeam() != nil:
		actionType = sbi.ScheduledDeleteBeam
		beam, err = a.convertDeleteBeamToBeamSpec(req.GetDeleteBeam())
		if err != nil {
			return nil, fmt.Errorf("failed to convert DeleteBeam: %w", err)
		}
	case req.GetSetRoute() != nil:
		actionType = sbi.ScheduledSetRoute
		route, err = a.convertSetRouteToRouteEntry(req.GetSetRoute())
		if err != nil {
			return nil, fmt.Errorf("failed to convert SetRoute: %w", err)
		}
	case req.GetDeleteRoute() != nil:
		actionType = sbi.ScheduledDeleteRoute
		route, err = a.convertDeleteRouteToRouteEntry(req.GetDeleteRoute())
		if err != nil {
			return nil, fmt.Errorf("failed to convert DeleteRoute: %w", err)
		}
	case req.GetSetSrPolicy() != nil:
		actionType = sbi.ScheduledSetSrPolicy
		srPolicy = &sbi.SrPolicySpec{
			PolicyID: req.GetSetSrPolicy().GetId(),
		}
	case req.GetDeleteSrPolicy() != nil:
		actionType = sbi.ScheduledDeleteSrPolicy
		srPolicy = &sbi.SrPolicySpec{
			PolicyID: req.GetDeleteSrPolicy().GetId(),
		}
	default:
		return nil, fmt.Errorf("CreateEntryRequest has no ConfigurationChange")
	}

	// Create ScheduledAction
	action := &sbi.ScheduledAction{
		EntryID:   req.GetId(),
		AgentID:   a.AgentID,
		Type:      actionType,
		When:      when,
		RequestID: meta.RequestID,
		SeqNo:     meta.SeqNo,
		Token:     meta.Token,
		Beam:      beam,
		Route:     route,
		SrPolicy:  srPolicy,
	}

	return action, nil
}

// extractTimeFromCreateEntry extracts the execution time from a CreateEntryRequest.
// It prefers Time (timestamp) over TimeGpst (duration from GPS epoch).
func (a *SimAgent) extractTimeFromCreateEntry(req *schedulingpb.CreateEntryRequest) (time.Time, error) {
	if req.GetTime() != nil {
		return req.GetTime().AsTime(), nil
	}
	if req.GetTimeGpst() != nil {
		// For Scope 4, we'll use a simple approach: treat GPST as relative to now
		// In a real implementation, this would be relative to GPS epoch
		// TODO: Implement proper GPST handling if needed
		duration := req.GetTimeGpst().AsDuration()
		return a.Scheduler.Now().Add(duration), nil
	}
	return time.Time{}, fmt.Errorf("CreateEntryRequest has no time field")
}

// extractTimeFromFinalize extracts the cutoff time from a FinalizeRequest.
func (a *SimAgent) extractTimeFromFinalize(req *schedulingpb.FinalizeRequest) (time.Time, error) {
	if req.GetUpTo() != nil {
		return req.GetUpTo().AsTime(), nil
	}
	if req.GetUpToGpst() != nil {
		// Similar to extractTimeFromCreateEntry
		duration := req.GetUpToGpst().AsDuration()
		return a.Scheduler.Now().Add(duration), nil
	}
	return time.Time{}, fmt.Errorf("FinalizeRequest has no time field")
}

// convertUpdateBeamToBeamSpec converts an UpdateBeam proto to a BeamSpec.
// This is a simplified conversion that extracts basic beam information.
func (a *SimAgent) convertUpdateBeamToBeamSpec(updateBeam *schedulingpb.UpdateBeam) (*sbi.BeamSpec, error) {
	beam := updateBeam.GetBeam()
	if beam == nil {
		return nil, fmt.Errorf("UpdateBeam has no beam")
	}

	beamSpec := &sbi.BeamSpec{
		NodeID:      a.NodeID, // Use agent's node ID
		InterfaceID: beam.GetAntennaId(),
	}

	// Extract target information from beam endpoints
	// For Scope 4, we'll use a simplified approach
	// TODO: Extract proper target node/interface from beam.Endpoints or beam.Target
	if len(beam.GetEndpoints()) > 0 {
		// Use first endpoint as target (simplified)
		for nodeID := range beam.GetEndpoints() {
			beamSpec.TargetNodeID = nodeID
			break
		}
	}

	return beamSpec, nil
}

// convertDeleteBeamToBeamSpec converts a DeleteBeam proto to a BeamSpec.
// DeleteBeam only has a beam ID, so we need to look up the beam to get interface info.
// For Scope 4, we'll use a simplified approach.
func (a *SimAgent) convertDeleteBeamToBeamSpec(deleteBeam *schedulingpb.DeleteBeam) (*sbi.BeamSpec, error) {
	beamID := deleteBeam.GetId()
	if beamID == "" {
		return nil, fmt.Errorf("DeleteBeam has no beam ID")
	}

	// For Scope 4, we'll construct a minimal BeamSpec with just the beam ID
	// The actual link lookup will happen in execute() via ApplyBeamDelete
	// TODO: Look up beam from state to get interface info
	beamSpec := &sbi.BeamSpec{
		NodeID: a.NodeID,
		// InterfaceID and TargetIfID will need to be determined from the beam ID
		// For now, we'll leave them empty and let ApplyBeamDelete handle it
	}

	return beamSpec, nil
}

// convertSetRouteToRouteEntry converts a SetRoute proto to a RouteEntry.
func (a *SimAgent) convertSetRouteToRouteEntry(setRoute *schedulingpb.SetRoute) (*model.RouteEntry, error) {
	route := &model.RouteEntry{
		DestinationCIDR: setRoute.GetTo(),
		OutInterfaceID:  setRoute.GetDev(),
	}

	// Via is the next hop address, but we need NextHopNodeID
	// For Scope 4, we'll leave it empty if Via is not a node ID
	// TODO: Map Via address to node ID if needed
	if setRoute.GetVia() != "" {
		// Try to use Via as node ID (simplified)
		route.NextHopNodeID = setRoute.GetVia()
	}

	return route, nil
}

// convertDeleteRouteToRouteEntry converts a DeleteRoute proto to a RouteEntry.
// DeleteRoute only has From/To, so we construct a minimal RouteEntry for lookup.
func (a *SimAgent) convertDeleteRouteToRouteEntry(deleteRoute *schedulingpb.DeleteRoute) (*model.RouteEntry, error) {
	route := &model.RouteEntry{
		DestinationCIDR: deleteRoute.GetTo(),
		// NextHopNodeID and OutInterfaceID are not needed for deletion
	}

	return route, nil
}

// execute executes a scheduled action and sends a Response.
// This is called by the EventScheduler when the action's time arrives.
// It switches on action.Type and calls the appropriate ScenarioState method,
// then sends a Response back to the controller.
func (a *SimAgent) execute(action *sbi.ScheduledAction) {
	var err error
	var responseStatus *status.Status

	// Execute the action based on type
	switch action.Type {
	case sbi.ScheduledUpdateBeam:
		if action.Beam == nil {
			err = fmt.Errorf("ScheduledUpdateBeam action has nil Beam")
			break
		}
		err = a.State.ApplyBeamUpdate(a.NodeID, action.Beam)

	case sbi.ScheduledDeleteBeam:
		if action.Beam == nil {
			err = fmt.Errorf("ScheduledDeleteBeam action has nil Beam")
			break
		}
		err = a.State.ApplyBeamDelete(
			action.Beam.NodeID,
			action.Beam.InterfaceID,
			action.Beam.TargetNodeID,
			action.Beam.TargetIfID,
		)

	case sbi.ScheduledSetRoute:
		if action.Route == nil {
			err = fmt.Errorf("ScheduledSetRoute action has nil Route")
			break
		}
		err = a.State.InstallRoute(a.NodeID, *action.Route)

	case sbi.ScheduledDeleteRoute:
		if action.Route == nil {
			err = fmt.Errorf("ScheduledDeleteRoute action has nil Route")
			break
		}
		err = a.State.RemoveRoute(a.NodeID, action.Route.DestinationCIDR)

	case sbi.ScheduledSetSrPolicy, sbi.ScheduledDeleteSrPolicy:
		// For Scope 4, SrPolicy is stubbed - just log and succeed
		// TODO: Add proper logging
		_ = action.SrPolicy
		err = nil

	default:
		err = fmt.Errorf("unknown action type: %v", action.Type)
	}

	// Build response status
	if err != nil {
		responseStatus = status.New(codes.Internal, err.Error())
	} else {
		responseStatus = status.New(codes.OK, "OK")
	}

	// Parse request ID from action
	var requestID int64
	if action.RequestID != "" {
		// Try to parse as int64
		_, _ = fmt.Sscanf(action.RequestID, "%d", &requestID)
	}

	// Send Response
	response := &schedulingpb.ReceiveRequestsMessageToController{
		Response: &schedulingpb.ReceiveRequestsMessageToController_Response{
			RequestId: requestID,
			Status:    responseStatus.Proto(),
		},
	}

	// Remove from pending map
	a.mu.Lock()
	delete(a.pending, action.EntryID)
	a.mu.Unlock()

	// Send response (non-blocking, best-effort)
	if a.Stream != nil {
		_ = a.Stream.Send(response)
	}
}

// Reset clears the agent's pending schedule and generates a new token.
// This should be called when the agent's schedule is reset (e.g., on startup
// or after a schedule reset event). The agent will then send a Reset RPC
// to the controller (in Chunk 5 when CDPI server is implemented).
func (a *SimAgent) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Clear all pending actions
	for entryID := range a.pending {
		a.Scheduler.Cancel(entryID)
	}
	a.pending = make(map[string]*sbi.ScheduledAction)

	// Generate a new token
	a.token = generateToken()
}

// GetToken returns the current schedule manipulation token.
// This is used by the controller to validate incoming requests.
func (a *SimAgent) GetToken() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.token
}

// validateToken checks if the provided token matches the agent's current token.
// Returns true if tokens match, false otherwise.
func (a *SimAgent) validateToken(token string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return token == a.token
}

// startTelemetryLoop starts the periodic telemetry emission loop.
// It schedules the first telemetry tick and reschedules itself after each tick.
func (a *SimAgent) startTelemetryLoop() {
	if a.TelemetryCli == nil {
		return
	}

	now := a.Scheduler.Now()
	a.telemetryMu.Lock()
	a.lastTick = now
	a.telemetryMu.Unlock()

	// Schedule first telemetry tick after the interval
	nextTick := now.Add(a.telemetryInterval)
	a.Scheduler.Schedule(nextTick, func() {
		a.telemetryTick()
	})
}

// telemetryTick collects interface metrics and sends them to the controller.
// It reschedules itself for the next interval.
func (a *SimAgent) telemetryTick() {
	// Check if agent is still running
	select {
	case <-a.ctx.Done():
		return
	default:
	}

	now := a.Scheduler.Now()

	// Calculate delta time since last tick
	a.telemetryMu.Lock()
	lastTick := a.lastTick
	if lastTick.IsZero() {
		lastTick = now
	}
	delta := now.Sub(lastTick)
	a.lastTick = now
	a.telemetryMu.Unlock()

	// Build interface metrics
	deltaSec := delta.Seconds()
	metrics := a.buildInterfaceMetrics(deltaSec)

	if len(metrics) == 0 {
		// No interfaces to report - reschedule anyway
		a.rescheduleTelemetry()
		return
	}

	// Build ExportMetricsRequest
	req := &telemetrypb.ExportMetricsRequest{
		InterfaceMetrics: metrics,
	}

	// Send metrics to controller (with node_id in metadata)
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("x-node-id", a.NodeID))

	_, err := a.TelemetryCli.ExportMetrics(ctx, req)
	if err != nil {
		// Log error but continue telemetry loop
		// TODO: Add proper logging
		_ = err
	}

	// Reschedule next tick
	a.rescheduleTelemetry()
}

// rescheduleTelemetry schedules the next telemetry tick.
func (a *SimAgent) rescheduleTelemetry() {
	now := a.Scheduler.Now()
	nextTick := now.Add(a.telemetryInterval)
	a.Scheduler.Schedule(nextTick, func() {
		a.telemetryTick()
	})
}

// buildInterfaceMetrics builds telemetry metrics for all interfaces on this agent's node.
// It uses deriveInterfaceState to determine up/down status and bandwidth.
func (a *SimAgent) buildInterfaceMetrics(deltaSec float64) []*telemetrypb.InterfaceMetrics {
	if a.State == nil {
		return nil
	}

	// Get all interfaces for this node
	interfaces := a.State.ListInterfacesForNode(a.NodeID)
	if len(interfaces) == 0 {
		return nil
	}

	now := a.Scheduler.Now()
	nowProto := timestamppb.New(now)

	var result []*telemetrypb.InterfaceMetrics

	for _, iface := range interfaces {
		// Derive interface state (up/down, bandwidth)
		up, bandwidthBps := a.deriveInterfaceState(a.NodeID, iface.ID)

		// Update byte counters
		a.telemetryMu.Lock()
		bytesTx := a.bytesTx[iface.ID]
		if up && bandwidthBps > 0 {
			// Estimate bytes transmitted based on bandwidth and time delta
			bytesDelta := uint64(bandwidthBps * deltaSec / 8)
			bytesTx += bytesDelta
			a.bytesTx[iface.ID] = bytesTx
		}
		a.telemetryMu.Unlock()

		// Build operational state data point
		var operStatus telemetrypb.IfOperStatus
		if up {
			operStatus = telemetrypb.IfOperStatus_IF_OPER_STATUS_UP
		} else {
			operStatus = telemetrypb.IfOperStatus_IF_OPER_STATUS_DOWN
		}

		// Build statistics data point
		txBytes := int64(bytesTx)
		rxBytes := int64(0) // Rx bytes not tracked yet

		metrics := &telemetrypb.InterfaceMetrics{
			InterfaceId: stringPtr(iface.ID),
			OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{
				{
					Time:  nowProto,
					Value: &operStatus,
				},
			},
			StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{
				{
					Time:    nowProto,
					TxBytes: &txBytes,
					RxBytes: &rxBytes,
				},
			},
		}

		result = append(result, metrics)
	}

	return result
}

// deriveInterfaceState determines the operational state and bandwidth for an interface.
// This is a stub implementation; full implementation will be done in issue #150.
// Returns (up bool, bandwidthBps float64).
func (a *SimAgent) deriveInterfaceState(nodeID, ifaceID string) (bool, float64) {
	// Stub implementation - will be fully implemented in issue #150
	// For now, return false/0 to avoid incorrect telemetry
	return false, 0
}

// generateToken generates a random token for schedule manipulation.
func generateToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "fallback-token"
	}
	return hex.EncodeToString(b[:])
}

// stringPtr returns a pointer to the given string.
func stringPtr(s string) *string {
	return &s
}

