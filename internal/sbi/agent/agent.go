package agent

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/model"
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
	mu            sync.Mutex
	pending       map[string]*sbi.ScheduledAction // keyed by EntryID
	token         string                          // schedule_manipulation_token (empty until first message)
	lastSeqNoSeen uint64                          // last seen sequence number for logging/debugging

	// SR Policy tracking (stub for Scope 4)
	srMu       sync.Mutex
	srPolicies map[string]*sbi.SrPolicySpec // keyed by PolicyID

	// Telemetry state
	telemetryMu       sync.Mutex
	telemetryInterval time.Duration
	bytesTx           map[string]uint64 // per-interface transmitted bytes (monotonic)
	viaNodeMapping    map[string]string // optional mapping for Via addresses to node IDs
	lastTick          time.Time         // last telemetry tick time

	// Logging
	log logging.Logger

	// Metrics (optional, shared pointer from controller or global)
	Metrics *sbi.SBIMetrics

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// NewSimAgent creates a new simulated agent with the given ID and dependencies.
// It uses default telemetry configuration.
func NewSimAgent(agentID sbi.AgentID, nodeID string, state *state.ScenarioState, scheduler sbi.EventScheduler, telemetryCli telemetrypb.TelemetryClient, stream grpc.BidiStreamingClient[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController], log logging.Logger) *SimAgent {
	return NewSimAgentWithConfig(agentID, nodeID, state, scheduler, telemetryCli, stream, DefaultTelemetryConfig(), log)
}

// NewSimAgentWithConfig creates a new simulated agent with telemetry configuration.
func NewSimAgentWithConfig(agentID sbi.AgentID, nodeID string, state *state.ScenarioState, scheduler sbi.EventScheduler, telemetryCli telemetrypb.TelemetryClient, stream grpc.BidiStreamingClient[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController], telemetryConfig TelemetryConfig, log logging.Logger) *SimAgent {
	cfg := telemetryConfig.ApplyDefaults()
	if log == nil {
		log = logging.Noop()
	}
	return &SimAgent{
		AgentID:           agentID,
		NodeID:            nodeID,
		State:             state,
		Scheduler:         scheduler,
		TelemetryCli:      telemetryCli,
		Stream:            stream,
		pending:           make(map[string]*sbi.ScheduledAction),
		token:             "", // empty until first scheduling message establishes it
		lastSeqNoSeen:     0,
		srPolicies:        make(map[string]*sbi.SrPolicySpec),
		telemetryInterval: cfg.Interval,
		bytesTx:           make(map[string]uint64),
		viaNodeMapping:    make(map[string]string),
		lastTick:          time.Time{},
		log:               log,
	}
}

// ID returns the agent's identifier.
func (a *SimAgent) ID() sbi.AgentID {
	return a.AgentID
}

// SetStream sets the CDPI stream for this agent.
// This must be called before Start() if the stream was not provided in the constructor.
func (a *SimAgent) SetStream(stream grpc.BidiStreamingClient[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController]) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Stream = stream
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

	// Log agent stop
	if a.ctx != nil {
		a.log.Info(a.ctx, "agent: stop",
			logging.String("agent_id", string(a.AgentID)),
			logging.String("node_id", a.NodeID),
		)
	}
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
				a.log.Error(a.ctx, "agent: stream recv error",
					logging.String("agent_id", string(a.AgentID)),
					logging.Any("error", err),
				)
				return
			}

			if err := a.handleMessage(msg); err != nil {
				// Log error but continue processing
				a.log.Error(a.ctx, "agent: handle message error",
					logging.String("agent_id", string(a.AgentID)),
					logging.Any("error", err),
				)
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
		return a.handleDeleteEntry(requestID, msg.GetDeleteEntry())
	case msg.GetFinalize() != nil:
		return a.handleFinalize(requestID, msg.GetFinalize())
	default:
		// Unknown message type - ignore for now
		return nil
	}
}

// handleCreateEntry processes a CreateEntryRequest message.
// It validates the token, converts the proto to ScheduledAction, and schedules it.
func (a *SimAgent) handleCreateEntry(requestID int64, req *schedulingpb.CreateEntryRequest) error {
	// Validate and establish token
	token := req.GetScheduleManipulationToken()
	if token == "" {
		// Missing token - log and ignore
		a.log.Warn(a.ctx, "agent: create entry missing token",
			logging.String("agent_id", string(a.AgentID)),
			logging.Any("request_id", requestID),
		)
		a.sendRequestResponse(requestID, status.New(codes.InvalidArgument, "create entry missing token"))
		return nil
	}

	a.mu.Lock()
	// First valid message establishes the token
	if a.token == "" {
		a.token = token
		a.log.Info(a.ctx, "agent: token established",
			logging.String("agent_id", string(a.AgentID)),
			logging.String("token", token),
		)
	} else if a.token != token {
		// Token mismatch - log and ignore stale message
		a.log.Warn(a.ctx, "agent: create entry token mismatch",
			logging.String("agent_id", string(a.AgentID)),
			logging.String("expected_token", a.token),
			logging.String("got_token", token),
			logging.Any("request_id", requestID),
		)
		a.sendRequestResponse(requestID, status.New(codes.PermissionDenied, "schedule manipulation token mismatch"))
		a.mu.Unlock()
		return nil
	}

	// Validate seqno
	seqNo := req.GetSeqno()
	if seqNo <= 0 {
		// Non-positive seqno - log but proceed
		a.log.Debug(a.ctx, "agent: create entry non-positive seqno",
			logging.String("agent_id", string(a.AgentID)),
			logging.Any("seqno", seqNo),
		)
	} else if a.lastSeqNoSeen != 0 && seqNo <= a.lastSeqNoSeen {
		// Non-monotonic seqno - log warning but proceed
		a.log.Warn(a.ctx, "agent: create entry non-monotonic seqno",
			logging.String("agent_id", string(a.AgentID)),
			logging.Any("last_seqno", a.lastSeqNoSeen),
			logging.Any("current_seqno", seqNo),
		)
	}
	a.lastSeqNoSeen = seqNo
	a.mu.Unlock()

	// Convert proto to ScheduledAction
	action, err := a.convertCreateEntryToAction(req, requestID)
	if err != nil {
		a.log.Error(a.ctx, "agent: failed to convert create entry",
			logging.String("agent_id", string(a.AgentID)),
			logging.Any("request_id", requestID),
			logging.Any("error", err),
		)
		a.sendRequestResponse(requestID, status.New(codes.Internal, fmt.Sprintf("failed to convert create entry: %v", err)))
		return fmt.Errorf("failed to convert CreateEntryRequest: %w", err)
	}

	// Handle SR policies immediately (stub behavior for Scope 4)
	if action.Type == sbi.ScheduledSetSrPolicy && action.SrPolicy != nil {
		a.handleSetSrPolicy(action.SrPolicy)
	} else if action.Type == sbi.ScheduledDeleteSrPolicy && action.SrPolicy != nil {
		a.handleDeleteSrPolicy(action.SrPolicy.PolicyID)
	}

	// Insert into pending and schedule
	if err := a.HandleScheduledAction(a.ctx, action); err != nil {
		a.log.Error(a.ctx, "agent: failed to handle scheduled action",
			logging.String("agent_id", string(a.AgentID)),
			logging.String("entry_id", action.EntryID),
			logging.String("action_type", action.Type.String()),
			logging.Any("error", err),
		)
		a.sendRequestResponse(requestID, status.New(codes.Internal, fmt.Sprintf("failed to handle scheduled action: %v", err)))
		return err
	}

	a.log.Info(a.ctx, "agent: create entry scheduled",
		logging.String("agent_id", string(a.AgentID)),
		logging.String("entry_id", action.EntryID),
		logging.String("action_type", action.Type.String()),
		logging.Any("seqno", seqNo),
	)

	return nil
}

// handleDeleteEntry processes a DeleteEntryRequest message.
// It cancels the previously scheduled action by EntryID.
func (a *SimAgent) handleDeleteEntry(requestID int64, req *schedulingpb.DeleteEntryRequest) error {
	// Validate token
	token := req.GetScheduleManipulationToken()
	if token == "" {
		// Missing token - log and ignore
		a.log.Warn(a.ctx, "agent: delete entry missing token",
			logging.String("agent_id", string(a.AgentID)),
			logging.Any("request_id", requestID),
		)
		a.sendRequestResponse(requestID, status.New(codes.InvalidArgument, "delete entry missing token"))
		return nil
	}

	a.mu.Lock()
	// First valid message establishes the token
	if a.token == "" {
		a.token = token
		a.log.Info(a.ctx, "agent: token established via delete entry",
			logging.String("agent_id", string(a.AgentID)),
			logging.String("token", token),
		)
	} else if a.token != token {
		// Token mismatch - log and ignore
		a.log.Warn(a.ctx, "agent: delete entry token mismatch",
			logging.String("agent_id", string(a.AgentID)),
			logging.String("expected_token", a.token),
			logging.String("got_token", token),
		)
		a.sendRequestResponse(requestID, status.New(codes.PermissionDenied, "schedule manipulation token mismatch"))
		a.mu.Unlock()
		return nil
	}

	// Validate seqno
	seqNo := req.GetSeqno()
	if seqNo <= 0 {
		// Non-positive seqno - log but proceed
		a.log.Debug(a.ctx, "agent: delete entry non-positive seqno",
			logging.String("agent_id", string(a.AgentID)),
			logging.Any("seqno", seqNo),
		)
	} else if a.lastSeqNoSeen != 0 && seqNo <= a.lastSeqNoSeen {
		// Non-monotonic seqno - log warning but proceed
		a.log.Warn(a.ctx, "agent: delete entry non-monotonic seqno",
			logging.String("agent_id", string(a.AgentID)),
			logging.Any("last_seqno", a.lastSeqNoSeen),
			logging.Any("current_seqno", seqNo),
		)
	}
	a.lastSeqNoSeen = seqNo

	entryID := req.GetId()
	if entryID == "" {
		a.mu.Unlock()
		a.log.Error(a.ctx, "agent: delete entry empty entry_id",
			logging.String("agent_id", string(a.AgentID)),
			logging.Any("request_id", requestID),
		)
		a.sendRequestResponse(requestID, status.New(codes.InvalidArgument, "delete entry requires entry ID"))
		return fmt.Errorf("DeleteEntryRequest has empty entry ID")
	}
	defer a.mu.Unlock()

	// Find and cancel the scheduled action
	if action, exists := a.pending[entryID]; exists {
		// Cancel the scheduled event (using EntryID as event ID)
		a.Scheduler.Cancel(entryID)
		delete(a.pending, entryID)

		// Notify controller that the action was cancelled before execution.
		a.sendResponseForAction(action, status.New(codes.Canceled, "scheduled action cancelled"))

		a.log.Info(a.ctx, "agent: delete entry cancelled",
			logging.String("agent_id", string(a.AgentID)),
			logging.String("entry_id", entryID),
			logging.String("action_type", action.Type.String()),
		)
		a.sendRequestResponse(requestID, status.New(codes.OK, "scheduled action cancelled"))
	} else {
		a.log.Debug(a.ctx, "agent: delete entry not found",
			logging.String("agent_id", string(a.AgentID)),
			logging.String("entry_id", entryID),
		)
		a.sendRequestResponse(requestID, status.New(codes.NotFound, "entry not found"))
	}

	return nil
}

// handleFinalize processes a FinalizeRequest message.
// It drops any pending entries with When < cutoffTime.
func (a *SimAgent) handleFinalize(requestID int64, req *schedulingpb.FinalizeRequest) error {
	// Validate token
	token := req.GetScheduleManipulationToken()
	if token == "" {
		// Missing token - log and ignore
		a.log.Warn(a.ctx, "agent: finalize missing token",
			logging.String("agent_id", string(a.AgentID)),
		)
		a.sendRequestResponse(requestID, status.New(codes.InvalidArgument, "finalize missing token"))
		return nil
	}

	a.mu.Lock()
	// First valid message establishes the token
	if a.token == "" {
		a.token = token
		a.log.Info(a.ctx, "agent: token established via finalize",
			logging.String("agent_id", string(a.AgentID)),
			logging.String("token", token),
		)
	} else if a.token != token {
		// Token mismatch - log and ignore
		a.log.Warn(a.ctx, "agent: finalize token mismatch",
			logging.String("agent_id", string(a.AgentID)),
			logging.String("expected_token", a.token),
			logging.String("got_token", token),
		)
		a.sendRequestResponse(requestID, status.New(codes.PermissionDenied, "schedule manipulation token mismatch"))
		a.mu.Unlock()
		return nil
	}

	// Validate seqno
	seqNo := req.GetSeqno()
	if seqNo <= 0 {
		// Non-positive seqno - log but proceed
		a.log.Debug(a.ctx, "agent: finalize non-positive seqno",
			logging.String("agent_id", string(a.AgentID)),
			logging.Any("seqno", seqNo),
		)
	} else if a.lastSeqNoSeen != 0 && seqNo <= a.lastSeqNoSeen {
		// Non-monotonic seqno - log warning but proceed
		a.log.Warn(a.ctx, "agent: finalize non-monotonic seqno",
			logging.String("agent_id", string(a.AgentID)),
			logging.Any("last_seqno", a.lastSeqNoSeen),
			logging.Any("current_seqno", seqNo),
		)
	}
	a.lastSeqNoSeen = seqNo

	// Extract cutoff time
	cutoffTime, err := a.extractTimeFromFinalize(req)
	if err != nil {
		a.mu.Unlock()
		a.log.Error(a.ctx, "agent: finalize failed to extract cutoff time",
			logging.String("agent_id", string(a.AgentID)),
			logging.Any("error", err),
		)
		a.sendRequestResponse(requestID, status.New(codes.InvalidArgument, fmt.Sprintf("failed to extract cutoff time: %v", err)))
		return fmt.Errorf("failed to extract cutoff time: %w", err)
	}

	// Drop entries with When < cutoff
	prunedCount := 0
	for entryID, action := range a.pending {
		if action.When.Before(cutoffTime) {
			// Cancel and remove
			a.Scheduler.Cancel(entryID)
			delete(a.pending, entryID)
			prunedCount++
		}
	}
	a.mu.Unlock()

	if prunedCount > 0 {
		a.log.Info(a.ctx, "agent: finalize pruned entries",
			logging.String("agent_id", string(a.AgentID)),
			logging.Int("pruned_count", prunedCount),
			logging.Any("cutoff_time", cutoffTime),
		)
	}
	a.sendRequestResponse(requestID, status.New(codes.OK, "finalize processed"))

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

	var messageSpec *sbi.DTNMessageSpec
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
	case req.GetStoreMessage() != nil:
		actionType = sbi.ScheduledStoreMessage
		messageSpec, err = a.convertProtoDTNMessageSpec(req.GetStoreMessage().GetMessage())
		if err != nil {
			return nil, fmt.Errorf("failed to convert StoreMessage: %w", err)
		}
	case req.GetForwardMessage() != nil:
		actionType = sbi.ScheduledForwardMessage
		messageSpec, err = a.convertProtoDTNMessageSpec(req.GetForwardMessage().GetMessage())
		if err != nil {
			return nil, fmt.Errorf("failed to convert ForwardMessage: %w", err)
		}
	default:
		return nil, fmt.Errorf("CreateEntryRequest has no ConfigurationChange")
	}

	// Create ScheduledAction
	action := &sbi.ScheduledAction{
		EntryID:     req.GetId(),
		AgentID:     a.AgentID,
		Type:        actionType,
		When:        when,
		RequestID:   meta.RequestID,
		SeqNo:       meta.SeqNo,
		Token:       meta.Token,
		Beam:        beam,
		Route:       route,
		SrPolicy:    srPolicy,
		MessageSpec: messageSpec,
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
		return gpsTimeFromDuration(req.GetTimeGpst().AsDuration()), nil
	}
	return time.Time{}, fmt.Errorf("CreateEntryRequest has no time field")
}

// extractTimeFromFinalize extracts the cutoff time from a FinalizeRequest.
func (a *SimAgent) extractTimeFromFinalize(req *schedulingpb.FinalizeRequest) (time.Time, error) {
	if req.GetUpTo() != nil {
		return req.GetUpTo().AsTime(), nil
	}
	if req.GetUpToGpst() != nil {
		return gpsTimeFromDuration(req.GetUpToGpst().AsDuration()), nil
	}
	return time.Time{}, fmt.Errorf("FinalizeRequest has no time field")
}

var gpsEpoch = time.Date(1980, time.January, 6, 0, 0, 0, 0, time.UTC)

var gpsLeapSecondSchedule = []struct {
	effective time.Time
	offset    int
}{
	{time.Date(1980, time.January, 6, 0, 0, 0, 0, time.UTC), 0},
	{time.Date(1981, time.July, 1, 0, 0, 0, 0, time.UTC), 1},
	{time.Date(1982, time.July, 1, 0, 0, 0, 0, time.UTC), 2},
	{time.Date(1983, time.July, 1, 0, 0, 0, 0, time.UTC), 3},
	{time.Date(1985, time.July, 1, 0, 0, 0, 0, time.UTC), 4},
	{time.Date(1988, time.January, 1, 0, 0, 0, 0, time.UTC), 5},
	{time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC), 6},
	{time.Date(1991, time.January, 1, 0, 0, 0, 0, time.UTC), 7},
	{time.Date(1992, time.July, 1, 0, 0, 0, 0, time.UTC), 8},
	{time.Date(1993, time.July, 1, 0, 0, 0, 0, time.UTC), 9},
	{time.Date(1994, time.July, 1, 0, 0, 0, 0, time.UTC), 10},
	{time.Date(1996, time.January, 1, 0, 0, 0, 0, time.UTC), 11},
	{time.Date(1997, time.July, 1, 0, 0, 0, 0, time.UTC), 12},
	{time.Date(1999, time.January, 1, 0, 0, 0, 0, time.UTC), 13},
	{time.Date(2006, time.January, 1, 0, 0, 0, 0, time.UTC), 14},
	{time.Date(2009, time.January, 1, 0, 0, 0, 0, time.UTC), 15},
	{time.Date(2012, time.July, 1, 0, 0, 0, 0, time.UTC), 16},
	{time.Date(2015, time.July, 1, 0, 0, 0, 0, time.UTC), 17},
	{time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC), 18},
}

func gpsTimeFromDuration(duration time.Duration) time.Time {
	gpsTime := gpsEpoch.Add(duration)
	return gpsToUTC(gpsTime)
}

func gpsToUTC(gpsTime time.Time) time.Time {
	offset := gpsLeapSecondsAt(gpsTime)
	return gpsTime.Add(-time.Duration(offset) * time.Second)
}

func gpsLeapSecondsAt(gpsTime time.Time) int {
	offset := 0
	for _, entry := range gpsLeapSecondSchedule {
		if gpsTime.Equal(entry.effective) || gpsTime.After(entry.effective) {
			offset = entry.offset
			continue
		}
		break
	}
	return offset
}

// convertUpdateBeamToBeamSpec converts an UpdateBeam proto to a BeamSpec.
// This is a simplified conversion that extracts basic beam information.
func (a *SimAgent) convertUpdateBeamToBeamSpec(updateBeam *schedulingpb.UpdateBeam) (*sbi.BeamSpec, error) {
	beam := updateBeam.GetBeam()
	if beam == nil {
		return nil, fmt.Errorf("UpdateBeam has no beam")
	}

	interfaceID := beam.GetAntennaId()
	beamSpec := &sbi.BeamSpec{
		NodeID:      a.NodeID,
		InterfaceID: interfaceID,
	}
	beamSpec.Target = convertBeamTargetProto(beam.GetTarget())

	if len(beam.GetEndpoints()) > 0 {
		// Use first endpoint as target (simplified)
		for nodeID := range beam.GetEndpoints() {
			beamSpec.TargetNodeID = nodeID
			break
		}
		if len(beam.GetEndpoints()) > 1 {
			a.log.Debug(a.ctx, "agent: multiple beam endpoints received",
				logging.String("agent_id", string(a.AgentID)),
				logging.String("interface_id", interfaceID),
				logging.Int("endpoint_count", len(beam.GetEndpoints())),
			)
		}
	}

	if targetNode, targetIface, ok := a.findTargetForInterface(interfaceID); ok {
		if beamSpec.TargetNodeID == "" {
			beamSpec.TargetNodeID = targetNode
		}
		if targetIface != "" {
			beamSpec.TargetIfID = targetIface
		}
	}

	if beamSpec.TargetNodeID == "" {
		a.log.Debug(a.ctx, "agent: update beam missing target node info",
			logging.String("agent_id", string(a.AgentID)),
			logging.String("interface_id", interfaceID),
		)
	}

	if beamSpec.TargetNodeID == "" {
		if resolved := a.nodeIDFromBeamTarget(beamSpec.Target); resolved != "" {
			beamSpec.TargetNodeID = resolved
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

	interfaceID := extractInterfaceIDFromBeamID(beamID)
	if interfaceID == "" {
		return nil, fmt.Errorf("unable to derive interface ID from beam ID %q", beamID)
	}

	beamSpec := &sbi.BeamSpec{
		NodeID:      a.NodeID,
		InterfaceID: interfaceID,
	}

	if targetNode, targetIface, ok := a.findTargetForInterface(interfaceID); ok {
		beamSpec.TargetNodeID = targetNode
		beamSpec.TargetIfID = targetIface
	}

	if beamSpec.TargetNodeID == "" {
		a.log.Debug(a.ctx, "agent: delete beam unable to resolve target node",
			logging.String("agent_id", string(a.AgentID)),
			logging.String("interface_id", interfaceID),
			logging.String("beam_id", beamID),
		)
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
	// Map Via to a known node ID when possible; otherwise fall back to the raw value.
	if viaNode := a.lookupNodeForVia(setRoute.GetVia()); viaNode != "" {
		route.NextHopNodeID = viaNode
	} else if setRoute.GetVia() != "" {
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

func convertBeamTargetProto(target *schedulingpb.BeamTarget) *sbi.BeamTarget {
	if target == nil || target.GetTarget() == nil {
		return nil
	}

	switch payload := target.GetTarget().(type) {
	case *schedulingpb.BeamTarget_AzEl:
		if az := payload.AzEl; az != nil {
			return &sbi.BeamTarget{
				AzEl: &sbi.BeamAzElTarget{
					AzimuthDeg:   az.GetAzDeg(),
					ElevationDeg: az.GetElDeg(),
				},
			}
		}
	case *schedulingpb.BeamTarget_Cartesian:
		if cart := payload.Cartesian; cart != nil {
			return &sbi.BeamTarget{
				Cartesian: &sbi.BeamCartesianTarget{
					Frame: cart.GetReferenceFrame(),
					Coordinates: model.Coordinates{
						X: cart.GetXM(),
						Y: cart.GetYM(),
						Z: cart.GetZM(),
					},
				},
			}
		}
	case *schedulingpb.BeamTarget_StateVector:
		if sv := payload.StateVector; sv != nil {
			return &sbi.BeamTarget{
				StateVector: &sbi.BeamStateVectorTarget{
					Frame: sv.GetReferenceFrame(),
					Position: model.Coordinates{
						X: sv.GetXM(),
						Y: sv.GetYM(),
						Z: sv.GetZM(),
					},
					Velocity: model.Motion{
						X: sv.GetXDotMPerS(),
						Y: sv.GetYDotMPerS(),
						Z: sv.GetZDotMPerS(),
					},
				},
			}
		}
	}

	return nil
}

const beamTargetMatchThresholdMeters = 1000.0

func (a *SimAgent) nodeIDFromBeamTarget(target *sbi.BeamTarget) string {
	if target == nil || a.State == nil {
		return ""
	}
	coords, ok := beamTargetCoordinates(target)
	if !ok {
		return ""
	}
	phys := a.State.PhysicalKB()
	if phys == nil {
		return ""
	}

	best := ""
	bestDist := math.MaxFloat64
	for _, node := range a.State.ListNodes() {
		if node == nil || node.PlatformID == "" {
			continue
		}
		platform := phys.GetPlatform(node.PlatformID)
		if platform == nil {
			continue
		}
		nodeCoords := model.Coordinates{
			X: platform.Coordinates.X,
			Y: platform.Coordinates.Y,
			Z: platform.Coordinates.Z,
		}
		if dist := coordinatesDistance(coords, nodeCoords); dist < bestDist {
			bestDist = dist
			best = node.ID
		}
	}

	if bestDist <= beamTargetMatchThresholdMeters {
		return best
	}
	return ""
}

func beamTargetCoordinates(target *sbi.BeamTarget) (model.Coordinates, bool) {
	if target == nil {
		return model.Coordinates{}, false
	}
	if target.Cartesian != nil {
		return target.Cartesian.Coordinates, true
	}
	if target.StateVector != nil {
		return target.StateVector.Position, true
	}
	return model.Coordinates{}, false
}

func coordinatesDistance(a, b model.Coordinates) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	dz := a.Z - b.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func (a *SimAgent) convertProtoDTNMessageSpec(proto *schedulingpb.DTNMessageSpec) (*sbi.DTNMessageSpec, error) {
	if proto == nil {
		return nil, fmt.Errorf("DTN message spec proto is nil")
	}
	spec := &sbi.DTNMessageSpec{
		ServiceRequestID: proto.GetServiceRequestId(),
		MessageID:        proto.GetMessageId(),
		MessageSizeBytes: proto.GetSizeBytes(),
		StorageNodeID:    proto.GetStorageNode(),
		DestinationNode:  proto.GetDestinationNode(),
		LinkID:           proto.GetLinkId(),
		NextHopNodeID:    proto.GetNextHopNode(),
	}
	if proto.GetStorageStart() != nil {
		spec.StorageStart = proto.GetStorageStart().AsTime()
	}
	if proto.GetStorageDuration() != nil {
		spec.StorageDuration = proto.GetStorageDuration().AsDuration()
	}
	if proto.GetExpiryTime() != nil {
		spec.ExpiryTime = proto.GetExpiryTime().AsTime()
	}
	return spec, nil
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
		// If TargetIfID is empty, try to look it up from the link
		if action.Beam.TargetIfID == "" && action.Beam.TargetNodeID != "" {
			// Look up the link to find the target interface
			links := a.State.ListLinks()
			for _, link := range links {
				if link == nil {
					continue
				}
				// Check if this link connects our interface to the target node
				srcIfRef := fmt.Sprintf("%s/%s", action.Beam.NodeID, action.Beam.InterfaceID)
				if (link.InterfaceA == srcIfRef && link.InterfaceB != "" && a.getNodeIDFromInterfaceRef(link.InterfaceB) == action.Beam.TargetNodeID) ||
					(link.InterfaceB == srcIfRef && link.InterfaceA != "" && a.getNodeIDFromInterfaceRef(link.InterfaceA) == action.Beam.TargetNodeID) {
					// Extract interface ID from the other end
					if link.InterfaceA == srcIfRef {
						action.Beam.TargetIfID = a.getInterfaceIDFromRef(link.InterfaceB)
					} else {
						action.Beam.TargetIfID = a.getInterfaceIDFromRef(link.InterfaceA)
					}
					break
				}
			}
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

	case sbi.ScheduledStoreMessage:
		if action.MessageSpec == nil {
			err = fmt.Errorf("ScheduledStoreMessage action has nil MessageSpec")
			break
		}
		msg := state.StoredMessage{
			MessageID:        action.MessageSpec.MessageID,
			ServiceRequestID: action.MessageSpec.ServiceRequestID,
			SizeBytes:        action.MessageSpec.MessageSizeBytes,
			ArrivalTime:      action.MessageSpec.StorageStart,
			ExpiryTime:       action.MessageSpec.ExpiryTime,
			Destination:      action.MessageSpec.DestinationNode,
			State:            state.MessageStateStored,
		}
		if msg.ArrivalTime.IsZero() {
			msg.ArrivalTime = a.Scheduler.Now()
		}
		err = a.State.StoreMessage(a.NodeID, msg)

	case sbi.ScheduledForwardMessage:
		if action.MessageSpec == nil {
			err = fmt.Errorf("ScheduledForwardMessage action has nil MessageSpec")
			break
		}
		_, err = a.State.RetrieveMessage(a.NodeID, action.MessageSpec.MessageID)

	case sbi.ScheduledSetSrPolicy, sbi.ScheduledDeleteSrPolicy:
		// For Scope 4, SrPolicy is stubbed - already handled in handleCreateEntry
		// No-op here since policies are stored immediately when CreateEntry is received
		err = nil

	default:
		err = fmt.Errorf("unknown action type: %v", action.Type)
	}

	// Build response status
	if err != nil {
		responseStatus = status.New(codes.Internal, err.Error())
		a.log.Error(a.ctx, "agent: action execution failed",
			logging.String("agent_id", string(a.AgentID)),
			logging.String("entry_id", action.EntryID),
			logging.String("action_type", action.Type.String()),
			logging.Any("error", err),
		)
	} else {
		responseStatus = status.New(codes.OK, "OK")
		a.log.Info(a.ctx, "agent: action executed",
			logging.String("agent_id", string(a.AgentID)),
			logging.String("entry_id", action.EntryID),
			logging.String("action_type", action.Type.String()),
		)
		// Increment metrics on successful execution
		if a.Metrics != nil {
			a.Metrics.IncActionsExecuted()
		}
	}

	// Remove from pending map
	a.mu.Lock()
	delete(a.pending, action.EntryID)
	a.mu.Unlock()

	a.sendResponseForAction(action, responseStatus)
}

// Reset clears the agent's pending schedule and resets token/seqno.
// This should be called when the agent's schedule is reset (e.g., on startup
// or after a schedule reset event). The agent will then send a Reset RPC
// to the controller, which will issue a new token.
func (a *SimAgent) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Clear all pending actions
	for entryID := range a.pending {
		a.Scheduler.Cancel(entryID)
	}
	a.pending = make(map[string]*sbi.ScheduledAction)

	// Clear token and seqno - next scheduling message will establish new token
	a.token = ""
	a.lastSeqNoSeen = 0

	// Clear SR policies
	a.srMu.Lock()
	a.srPolicies = make(map[string]*sbi.SrPolicySpec)
	a.srMu.Unlock()
}

// handleSetSrPolicy stores an SR policy in the agent's SR policy registry.
// This is a stub implementation for Scope 4 - it does not affect routing.
func (a *SimAgent) handleSetSrPolicy(spec *sbi.SrPolicySpec) {
	if spec == nil || spec.PolicyID == "" {
		return
	}

	a.srMu.Lock()
	defer a.srMu.Unlock()

	if a.srPolicies == nil {
		a.srPolicies = make(map[string]*sbi.SrPolicySpec)
	}

	// Store a copy to prevent external mutation
	cp := *spec
	a.srPolicies[spec.PolicyID] = &cp

	a.log.Info(a.ctx, "agent: stored sr policy",
		logging.String("agent_id", string(a.AgentID)),
		logging.String("node_id", a.NodeID),
		logging.String("policy_id", spec.PolicyID),
	)
}

// handleDeleteSrPolicy removes an SR policy from the agent's registry.
// This is a stub implementation for Scope 4.
func (a *SimAgent) handleDeleteSrPolicy(policyID string) {
	if policyID == "" {
		return
	}

	a.srMu.Lock()
	defer a.srMu.Unlock()

	delete(a.srPolicies, policyID)

	a.log.Info(a.ctx, "agent: deleted sr policy",
		logging.String("agent_id", string(a.AgentID)),
		logging.String("node_id", a.NodeID),
		logging.String("policy_id", policyID),
	)
}

// DumpSrPolicies returns all SR policies currently stored on the agent.
// This is a debug helper for tests/CLI tools and is not exposed via NBI yet.
func (a *SimAgent) DumpSrPolicies() []*sbi.SrPolicySpec {
	a.srMu.Lock()
	defer a.srMu.Unlock()

	out := make([]*sbi.SrPolicySpec, 0, len(a.srPolicies))
	for _, p := range a.srPolicies {
		// Return shallow copies to prevent external mutation
		cp := *p
		out = append(out, &cp)
	}
	return out
}

// GetToken returns the current schedule manipulation token.
// This is used by the controller to validate incoming requests.
func (a *SimAgent) GetToken() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.token
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

	a.log.Info(a.ctx, "agent: telemetry loop started",
		logging.String("agent_id", string(a.AgentID)),
		logging.Any("interval", a.telemetryInterval),
		logging.Any("next_tick", nextTick),
	)
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
	metrics, modemMetrics := a.buildInterfaceMetrics(deltaSec)

	if len(metrics) == 0 && len(modemMetrics) == 0 {
		// No interfaces to report - reschedule anyway
		a.rescheduleTelemetry()
		return
	}

	// Build ExportMetricsRequest
	req := &telemetrypb.ExportMetricsRequest{
		InterfaceMetrics: metrics,
		ModemMetrics:     modemMetrics,
	}

	// Send metrics to controller (with node_id in metadata)
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("x-node-id", a.NodeID))

	_, err := a.TelemetryCli.ExportMetrics(ctx, req)
	if err != nil {
		a.log.Error(a.ctx, "agent: telemetry export failed",
			logging.String("agent_id", string(a.AgentID)),
			logging.Int("metrics_count", len(metrics)),
			logging.Any("error", err),
		)
	} else {
		a.log.Debug(a.ctx, "agent: telemetry exported",
			logging.String("agent_id", string(a.AgentID)),
			logging.Int("metrics_count", len(metrics)),
		)
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
// It uses deriveInterfaceState to determine up/down state and bandwidth, and
// additionally includes modem metrics for each interface.
func (a *SimAgent) buildInterfaceMetrics(deltaSec float64) ([]*telemetrypb.InterfaceMetrics, []*telemetrypb.ModemMetrics) {
	if a.State == nil {
		return nil, nil
	}

	// Get all interfaces for this node
	interfaces := a.State.ListInterfacesForNode(a.NodeID)
	if len(interfaces) == 0 {
		return nil, nil
	}

	now := a.Scheduler.Now()
	nowProto := timestamppb.New(now)

	var result []*telemetrypb.InterfaceMetrics
	var modemResult []*telemetrypb.ModemMetrics

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

		if modem := a.CollectModemMetrics(a.NodeID, iface); modem != nil {
			if proto := a.modemMetricsToProto(modem); proto != nil {
				modemResult = append(modemResult, proto)
			}
		}
	}

	return result, modemResult
}

// deriveInterfaceState determines the operational state and bandwidth for an interface.
// It checks all links connected to this interface and determines:
// - up: true if at least one link is Active (Status=LinkStatusActive) and IsUp=true
// - bandwidthBps: the maximum MaxDataRateMbps across all active links, converted to bits per second
// Returns (up bool, bandwidthBps float64).
func (a *SimAgent) deriveInterfaceState(nodeID, ifaceID string) (bool, float64) {
	if a.State == nil {
		return false, 0
	}
	up, bandwidth, _ := a.interfaceStats(nodeID, ifaceID)
	return up, bandwidth
}

func (a *SimAgent) interfaceStats(nodeID, ifaceID string) (bool, float64, float64) {
	if a.State == nil {
		return false, 0, 0
	}

	allLinks := a.State.ListLinks()
	ifaceRef := nodeID + "/" + ifaceID

	var maxBandwidthMbps float64
	bestSNR := math.Inf(-1)
	hasActiveLink := false

	for _, link := range allLinks {
		if link == nil {
			continue
		}
		if !linkConnectsInterface(link, ifaceID, ifaceRef) {
			continue
		}
		if link.Status == core.LinkStatusActive && link.IsUp {
			hasActiveLink = true
			if link.MaxDataRateMbps > maxBandwidthMbps {
				maxBandwidthMbps = link.MaxDataRateMbps
			}
			if link.SNRdB > bestSNR {
				bestSNR = link.SNRdB
			}
		}
	}

	if !hasActiveLink {
		return false, 0, 0
	}

	bandwidthBps := maxBandwidthMbps * 1e6
	return hasActiveLink, bandwidthBps, bestSNR
}

func linkConnectsInterface(link *core.NetworkLink, ifaceID, ifaceRef string) bool {
	return link.InterfaceA == ifaceID || link.InterfaceA == ifaceRef || link.InterfaceB == ifaceID || link.InterfaceB == ifaceRef
}

// CollectModemMetrics builds modem-level metrics for the given interface.
func (a *SimAgent) CollectModemMetrics(nodeID string, iface *core.NetworkInterface) *state.ModemMetrics {
	if a.State == nil || iface == nil {
		return nil
	}

	_, bandwidthBps, snr := a.interfaceStats(nodeID, iface.ID)
	trx := a.State.NetworkKB().GetTransceiverModel(iface.TransceiverID)

	modulation := "unknown"
	if trx != nil {
		if trx.Name != "" {
			modulation = trx.Name
		} else if trx.ID != "" {
			modulation = trx.ID
		}
	}

	timestamp := a.Scheduler.Now()
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	return &state.ModemMetrics{
		NodeID:        nodeID,
		InterfaceID:   iface.ID,
		SNRdB:         snr,
		Modulation:    modulation,
		CodingRate:    "1/2",
		BER:           a.estimateBER(snr),
		ThroughputBps: uint64(bandwidthBps),
		Timestamp:     timestamp,
	}
}

func (a *SimAgent) estimateBER(snr float64) float64 {
	if snr <= 0 {
		return 1e-3
	}
	linear := math.Pow(10, snr/10)
	if linear <= 0 {
		return 1e-3
	}
	return 0.5 * math.Erfc(math.Sqrt(linear)/math.Sqrt2)
}

func (a *SimAgent) modemMetricsToProto(m *state.ModemMetrics) *telemetrypb.ModemMetrics {
	if m == nil || m.InterfaceID == "" {
		return nil
	}
	ts := m.Timestamp
	if ts.IsZero() {
		ts = a.Scheduler.Now()
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	modulator := m.Modulation
	if modulator == "" {
		modulator = "unknown"
	}
	sinrVal := m.SNRdB
	return &telemetrypb.ModemMetrics{
		DemodulatorId: stringPtr(m.InterfaceID),
		SinrDataPoints: []*telemetrypb.SinrDataPoint{
			{
				Time:        timestamppb.New(ts),
				ModulatorId: stringPtr(modulator),
				SinrDb:      float64Ptr(sinrVal),
			},
		},
	}
}

// stringPtr returns a pointer to the given string.
func stringPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}

// SetViaNodeMapping configures a map used to convert Via addresses into node IDs.
func (a *SimAgent) SetViaNodeMapping(mapping map[string]string) {
	if mapping == nil {
		return
	}
	a.mu.Lock()
	a.viaNodeMapping = make(map[string]string, len(mapping))
	for via, node := range mapping {
		a.viaNodeMapping[via] = node
	}
	a.mu.Unlock()
}

func (a *SimAgent) lookupNodeForVia(via string) string {
	if via == "" {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.viaNodeMapping[via]
}

// getNodeIDFromInterfaceRef extracts the node ID from an interface reference.
// Interface references can be in "node-ID/interface-ID" format or just "interface-ID".
func (a *SimAgent) getNodeIDFromInterfaceRef(ifRef string) string {
	parts := strings.SplitN(ifRef, "/", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return ""
}

// getInterfaceIDFromRef extracts the interface ID from an interface reference.
// Interface references can be in "node-ID/interface-ID" format or just "interface-ID".
func (a *SimAgent) getInterfaceIDFromRef(ifRef string) string {
	parts := strings.SplitN(ifRef, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ifRef
}

// parseRequestID attempts to convert the stored request ID string to an int64.
func parseRequestID(requestID string) int64 {
	if requestID == "" {
		return 0
	}
	var parsed int64
	if _, err := fmt.Sscanf(requestID, "%d", &parsed); err == nil {
		return parsed
	}
	return 0
}

// sendResponseForAction sends a response for the given action status to the controller.
func (a *SimAgent) sendResponseForAction(action *sbi.ScheduledAction, responseStatus *status.Status) {
	if action == nil || responseStatus == nil {
		return
	}

	response := &schedulingpb.ReceiveRequestsMessageToController{
		Response: &schedulingpb.ReceiveRequestsMessageToController_Response{
			RequestId: parseRequestID(action.RequestID),
			Status:    responseStatus.Proto(),
		},
	}

	if a.Stream == nil {
		return
	}

	if sendErr := a.Stream.Send(response); sendErr != nil {
		a.log.Error(a.ctx, "agent: failed to send response",
			logging.String("agent_id", string(a.AgentID)),
			logging.String("entry_id", action.EntryID),
			logging.Any("error", sendErr),
		)
	}
}

// sendRequestResponse sends a protocol response for a request that does not map to an action.
func (a *SimAgent) sendRequestResponse(requestID int64, responseStatus *status.Status) {
	if responseStatus == nil || a.Stream == nil {
		return
	}

	resp := &schedulingpb.ReceiveRequestsMessageToController{
		Response: &schedulingpb.ReceiveRequestsMessageToController_Response{
			RequestId: requestID,
			Status:    responseStatus.Proto(),
		},
	}

	if err := a.Stream.Send(resp); err != nil {
		a.log.Error(a.ctx, "agent: failed to send request response",
			logging.String("agent_id", string(a.AgentID)),
			logging.Any("request_id", requestID),
			logging.Any("error", err),
		)
	}
}

// findTargetForInterface locates the peer node and interface for a local interface reference.
func (a *SimAgent) findTargetForInterface(interfaceID string) (string, string, bool) {
	if a.State == nil || interfaceID == "" || a.NodeID == "" {
		return "", "", false
	}
	srcRef := fmt.Sprintf("%s/%s", a.NodeID, interfaceID)
	for _, link := range a.State.ListLinks() {
		if link == nil {
			continue
		}
		if link.InterfaceA == srcRef && link.InterfaceB != "" {
			return a.getNodeIDFromInterfaceRef(link.InterfaceB), a.getInterfaceIDFromRef(link.InterfaceB), true
		}
		if link.InterfaceB == srcRef && link.InterfaceA != "" {
			return a.getNodeIDFromInterfaceRef(link.InterfaceA), a.getInterfaceIDFromRef(link.InterfaceA), true
		}
	}
	return "", "", false
}

// extractInterfaceIDFromBeamID derives the local interface identifier from a Beam ID.
func extractInterfaceIDFromBeamID(beamID string) string {
	if beamID == "" {
		return ""
	}
	if idx := strings.LastIndex(beamID, ":"); idx >= 0 {
		beamID = beamID[idx+1:]
	}
	if idx := strings.LastIndex(beamID, "/"); idx >= 0 {
		beamID = beamID[idx+1:]
	}
	return beamID
}
