package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CDPIServer implements the ControlDataPlaneInterface gRPC service.
// It manages bidirectional streams with agents, sending scheduled actions
// and receiving Hello/Reset/Response messages.
type CDPIServer struct {
	schedulingpb.UnimplementedSchedulingServer

	State    *state.ScenarioState
	Clock    sbi.EventScheduler
	agentsMu sync.RWMutex
	agents   map[string]*AgentHandle // tracked by AgentID
	log      logging.Logger
}

// AgentHandle represents an active agent connection to the controller.
// It tracks the agent's stream, outgoing message channel, and current token.
type AgentHandle struct {
	AgentID  string
	NodeID   string
	Stream   grpc.BidiStreamingServer[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController]
	outgoing chan *schedulingpb.ReceiveRequestsMessageFromController
	token    string
	seqNoMu  sync.Mutex // protects seqNo
	seqNo    uint64     // monotonically increasing sequence number per agent
}

// CurrentToken returns the current schedule manipulation token for this agent.
func (h *AgentHandle) CurrentToken() string {
	return h.token
}

// SetToken sets the schedule manipulation token for this agent.
func (h *AgentHandle) SetToken(token string) {
	h.token = token
}

// NextSeqNo increments and returns the next sequence number for this agent.
// It is thread-safe and ensures monotonically increasing sequence numbers.
func (h *AgentHandle) NextSeqNo() uint64 {
	h.seqNoMu.Lock()
	defer h.seqNoMu.Unlock()
	h.seqNo++
	return h.seqNo
}

// NewCDPIServer creates a new CDPI server with the given dependencies.
func NewCDPIServer(state *state.ScenarioState, clock sbi.EventScheduler, log logging.Logger) *CDPIServer {
	if log == nil {
		log = logging.Noop()
	}
	return &CDPIServer{
		State:  state,
		Clock:  clock,
		agents: make(map[string]*AgentHandle),
		log:    log,
	}
}

// ReceiveRequests implements the bidirectional streaming RPC for CDPI.
// It waits for a Hello message from the agent, creates an AgentHandle,
// and manages the bidirectional stream.
func (s *CDPIServer) ReceiveRequests(stream grpc.BidiStreamingServer[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController]) error {
	// Wait for first message from agent - must be Hello
	firstMsg, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "failed to receive initial message: %v", err)
	}

	// Extract Hello message
	hello := firstMsg.GetHello()
	if hello == nil {
		return status.Error(codes.InvalidArgument, "first message must be Hello")
	}

	agentID := hello.GetAgentId()
	if agentID == "" {
		return status.Error(codes.InvalidArgument, "Hello message must contain agent_id")
	}

	// Map agent_id to NodeID
	// For Scope 4, we use a simple mapping: agent_id equals node_id
	// In a real implementation, this could be looked up from node config
	nodeID := agentID

	// Verify node exists in scenario state
	node, _, err := s.State.GetNode(nodeID)
	if err != nil {
		return status.Errorf(codes.NotFound, "node %q not found for agent %q: %v", nodeID, agentID, err)
	}
	if node == nil {
		return status.Errorf(codes.NotFound, "node %q not found for agent %q", nodeID, agentID)
	}

	// Create AgentHandle
	handle := &AgentHandle{
		AgentID:  agentID,
		NodeID:   nodeID,
		Stream:   stream,
		outgoing: make(chan *schedulingpb.ReceiveRequestsMessageFromController, 10),
		token:    generateToken(),
		seqNo:    0,
	}

	// Register agent handle
	s.agentsMu.Lock()
	s.agents[agentID] = handle
	s.agentsMu.Unlock()

	// Log agent connection
	s.log.Info(context.Background(), "cdpi: agent connected",
		logging.String("agent_id", agentID),
		logging.String("node_id", nodeID),
	)

	// Clean up on exit
	cleanupDone := false
	defer func() {
		s.agentsMu.Lock()
		delete(s.agents, agentID)
		if !cleanupDone {
			close(handle.outgoing)
			cleanupDone = true
		}
		s.agentsMu.Unlock()
	}()

	// Start goroutine to send messages from controller to agent
	sendDone := make(chan error, 1)
	go func() {
		for msg := range handle.outgoing {
			if err := stream.Send(msg); err != nil {
				sendDone <- err
				return
			}
		}
		sendDone <- nil
	}()

	// Loop reading agent→controller messages
	for {
		msg, err := stream.Recv()
		if err != nil {
			// Stream closed or error
			s.agentsMu.Lock()
			if !cleanupDone {
				close(handle.outgoing)
				cleanupDone = true
			}
			s.agentsMu.Unlock()
			<-sendDone // Wait for send goroutine to finish
			// Log stream closure
			s.log.Info(context.Background(), "cdpi: agent stream closed",
				logging.String("agent_id", agentID),
				logging.String("error", err.Error()),
			)
			return err
		}

		// Handle different message types
		switch {
		case msg.GetHello() != nil:
			// Hello should only come once at the start
			// If we receive it again, it's an error
			s.log.Warn(context.Background(), "cdpi: unexpected Hello after handshake",
				logging.String("agent_id", agentID),
			)
			return status.Error(codes.InvalidArgument, "Hello message received after initial handshake")

		case msg.GetResponse() != nil:
			// Handle Response
			response := msg.GetResponse()
			// Log response from agent
			reqID := response.GetRequestId()
			statusProto := response.GetStatus()
			statusCode := "unknown"
			if statusProto != nil {
				// Extract status code from proto
				statusCode = fmt.Sprintf("%d", statusProto.GetCode())
			}
			s.log.Debug(context.Background(), "cdpi: response from agent",
				logging.String("agent_id", agentID),
				logging.Any("request_id", reqID),
				logging.String("status", statusCode),
			)

		default:
			// Unknown message type - ignore for now
			// Note: Reset is handled via the separate Reset RPC, not through the stream
		}
	}
}

// setAgentToken sets or rotates the schedule manipulation token for an agent.
// It optionally resets the sequence number to 0.
// This is used by Reset-handling code to rotate tokens.
func (s *CDPIServer) setAgentToken(agentID, token string) error {
	s.agentsMu.Lock()
	defer s.agentsMu.Unlock()

	handle, ok := s.agents[agentID]
	if !ok {
		return fmt.Errorf("cdpi: agent %q not connected", agentID)
	}

	handle.SetToken(token)
	handle.seqNoMu.Lock()
	handle.seqNo = 0 // reset seqno on token rotation
	handle.seqNoMu.Unlock()
	return nil
}

// Reset implements the Reset RPC for CDPI.
// It clears any existing schedule entries for the agent and issues a fresh token.
func (s *CDPIServer) Reset(ctx context.Context, req *schedulingpb.ResetRequest) (*emptypb.Empty, error) {
	agentID := req.GetAgentId()
	if agentID == "" {
		return nil, status.Error(codes.InvalidArgument, "ResetRequest must contain agent_id")
	}

	// Generate a fresh token and set it
	newToken := generateToken()
	if err := s.setAgentToken(agentID, newToken); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}

	// Log agent reset
	s.log.Info(ctx, "cdpi: agent reset",
		logging.String("agent_id", agentID),
		logging.String("token", newToken),
	)

	return &emptypb.Empty{}, nil
}

// buildCreateEntryMessage builds a CreateEntryRequest message with token and seqno.
func (s *CDPIServer) buildCreateEntryMessage(h *AgentHandle, action *sbi.ScheduledAction) (*schedulingpb.ReceiveRequestsMessageFromController, error) {
	seqNo := h.NextSeqNo()
	token := h.CurrentToken()

	// Convert ScheduledAction to CreateEntryRequest
	createEntry, err := convertActionToCreateEntry(action, token, seqNo)
	if err != nil {
		return nil, fmt.Errorf("failed to convert action to CreateEntryRequest: %w", err)
	}

	return &schedulingpb.ReceiveRequestsMessageFromController{
		Request: &schedulingpb.ReceiveRequestsMessageFromController_CreateEntry{
			CreateEntry: createEntry,
		},
	}, nil
}

// SendCreateEntry sends a CreateEntryRequest to the specified agent.
// It converts the ScheduledAction to a proto message and pushes it onto
// the agent's outgoing channel.
func (s *CDPIServer) SendCreateEntry(agentID string, action *sbi.ScheduledAction) error {
	if action == nil {
		return status.Error(codes.InvalidArgument, "action must not be nil")
	}

	s.agentsMu.RLock()
	handle, exists := s.agents[agentID]
	s.agentsMu.RUnlock()

	if !exists {
		return status.Errorf(codes.NotFound, "cdpi: agent %q not connected", agentID)
	}

	msg, err := s.buildCreateEntryMessage(handle, action)
	if err != nil {
		return status.Errorf(codes.Internal, "cdpi: failed to build create-entry message for agent %q: %v", agentID, err)
	}

	// Send to agent's outgoing channel
	select {
	case handle.outgoing <- msg:
		return nil
	default:
		return status.Errorf(codes.ResourceExhausted, "cdpi: outgoing channel full for agent %q", agentID)
	}
}

// buildDeleteEntryMessage builds a DeleteEntryRequest message with token and seqno.
func (s *CDPIServer) buildDeleteEntryMessage(h *AgentHandle, entryID string) *schedulingpb.ReceiveRequestsMessageFromController {
	seqNo := h.NextSeqNo()
	token := h.CurrentToken()

	// Build DeleteEntryRequest
	deleteEntry := &schedulingpb.DeleteEntryRequest{
		ScheduleManipulationToken: token,
		Seqno:                     seqNo,
		Id:                        entryID,
	}

	return &schedulingpb.ReceiveRequestsMessageFromController{
		Request: &schedulingpb.ReceiveRequestsMessageFromController_DeleteEntry{
			DeleteEntry: deleteEntry,
		},
	}
}

// SendDeleteEntry sends a DeleteEntryRequest to the specified agent.
func (s *CDPIServer) SendDeleteEntry(agentID, entryID string) error {
	if entryID == "" {
		return status.Error(codes.InvalidArgument, "entryID must not be empty")
	}

	s.agentsMu.RLock()
	handle, exists := s.agents[agentID]
	s.agentsMu.RUnlock()

	if !exists {
		return status.Errorf(codes.NotFound, "cdpi: agent %q not connected", agentID)
	}

	msg := s.buildDeleteEntryMessage(handle, entryID)
	if msg == nil {
		return status.Errorf(codes.Internal, "cdpi: failed to build delete-entry message for agent %q", agentID)
	}

	// Send to agent's outgoing channel
	select {
	case handle.outgoing <- msg:
		// Log DeleteEntry sent
		s.log.Debug(context.Background(), "cdpi: send delete entry",
			logging.String("agent_id", agentID),
			logging.String("entry_id", entryID),
		)
		return nil
	default:
		s.log.Warn(context.Background(), "cdpi: outgoing channel full",
			logging.String("agent_id", agentID),
		)
		return status.Errorf(codes.ResourceExhausted, "cdpi: outgoing channel full for agent %q", agentID)
	}
}

// buildFinalizeMessage builds a FinalizeRequest message with token and seqno.
func (s *CDPIServer) buildFinalizeMessage(h *AgentHandle, cutoff time.Time) (*schedulingpb.ReceiveRequestsMessageFromController, error) {
	seqNo := h.NextSeqNo()
	token := h.CurrentToken()

	// Convert time.Time to protobuf Timestamp
	cutoffProto := timestamppb.New(cutoff)
	if err := cutoffProto.CheckValid(); err != nil {
		return nil, fmt.Errorf("invalid cutoff time: %w", err)
	}

	// Build FinalizeRequest
	finalize := &schedulingpb.FinalizeRequest{
		ScheduleManipulationToken: token,
		Seqno:                     seqNo,
		UpTo:                      cutoffProto,
	}

	return &schedulingpb.ReceiveRequestsMessageFromController{
		Request: &schedulingpb.ReceiveRequestsMessageFromController_Finalize{
			Finalize: finalize,
		},
	}, nil
}

// SendFinalize sends a FinalizeRequest to the specified agent.
func (s *CDPIServer) SendFinalize(agentID string, cutoff time.Time) error {
	s.agentsMu.RLock()
	handle, exists := s.agents[agentID]
	s.agentsMu.RUnlock()

	if !exists {
		return status.Errorf(codes.NotFound, "cdpi: agent %q not connected", agentID)
	}

	msg, err := s.buildFinalizeMessage(handle, cutoff)
	if err != nil {
		return status.Errorf(codes.Internal, "cdpi: failed to build finalize message for agent %q: %v", agentID, err)
	}

	// Send to agent's outgoing channel
	select {
	case handle.outgoing <- msg:
		// Log Finalize sent
		s.log.Debug(context.Background(), "cdpi: send finalize",
			logging.String("agent_id", agentID),
			logging.Any("cutoff", cutoff),
		)
		return nil
	default:
		s.log.Warn(context.Background(), "cdpi: outgoing channel full",
			logging.String("agent_id", agentID),
		)
		return status.Errorf(codes.ResourceExhausted, "cdpi: outgoing channel full for agent %q", agentID)
	}
}

// convertActionToCreateEntry converts a ScheduledAction to a CreateEntryRequest proto.
func convertActionToCreateEntry(action *sbi.ScheduledAction, token string, seqNo uint64) (*schedulingpb.CreateEntryRequest, error) {
	// Convert time.Time to protobuf Timestamp
	whenProto := timestamppb.New(action.When)
	if err := whenProto.CheckValid(); err != nil {
		return nil, fmt.Errorf("invalid action time: %v", err)
	}

	// Build base CreateEntryRequest
	createEntry := &schedulingpb.CreateEntryRequest{
		ScheduleManipulationToken: token,
		Seqno:                     seqNo,
		Id:                        action.EntryID,
		Time:                      whenProto,
	}

	// Set ConfigurationChange based on action type
	switch action.Type {
	case sbi.ScheduledUpdateBeam:
		if action.Beam == nil {
			return nil, fmt.Errorf("ScheduledUpdateBeam requires non-nil Beam")
		}
		updateBeam := convertBeamSpecToUpdateBeam(action.Beam)
		createEntry.ConfigurationChange = &schedulingpb.CreateEntryRequest_UpdateBeam{
			UpdateBeam: updateBeam,
		}

	case sbi.ScheduledDeleteBeam:
		if action.Beam == nil {
			return nil, fmt.Errorf("ScheduledDeleteBeam requires non-nil Beam")
		}
		deleteBeam := convertBeamSpecToDeleteBeam(action.Beam)
		createEntry.ConfigurationChange = &schedulingpb.CreateEntryRequest_DeleteBeam{
			DeleteBeam: deleteBeam,
		}

	case sbi.ScheduledSetRoute:
		if action.Route == nil {
			return nil, fmt.Errorf("ScheduledSetRoute requires non-nil Route")
		}
		setRoute := convertRouteEntryToSetRoute(action.Route)
		createEntry.ConfigurationChange = &schedulingpb.CreateEntryRequest_SetRoute{
			SetRoute: setRoute,
		}

	case sbi.ScheduledDeleteRoute:
		if action.Route == nil {
			return nil, fmt.Errorf("ScheduledDeleteRoute requires non-nil Route")
		}
		deleteRoute := convertRouteEntryToDeleteRoute(action.Route)
		createEntry.ConfigurationChange = &schedulingpb.CreateEntryRequest_DeleteRoute{
			DeleteRoute: deleteRoute,
		}

	case sbi.ScheduledSetSrPolicy:
		if action.SrPolicy == nil {
			return nil, fmt.Errorf("ScheduledSetSrPolicy requires non-nil SrPolicy")
		}
		setSrPolicy := convertSrPolicySpecToSetSrPolicy(action.SrPolicy)
		createEntry.ConfigurationChange = &schedulingpb.CreateEntryRequest_SetSrPolicy{
			SetSrPolicy: setSrPolicy,
		}

	case sbi.ScheduledDeleteSrPolicy:
		if action.SrPolicy == nil {
			return nil, fmt.Errorf("ScheduledDeleteSrPolicy requires non-nil SrPolicy")
		}
		deleteSrPolicy := convertSrPolicySpecToDeleteSrPolicy(action.SrPolicy)
		createEntry.ConfigurationChange = &schedulingpb.CreateEntryRequest_DeleteSrPolicy{
			DeleteSrPolicy: deleteSrPolicy,
		}

	default:
		return nil, fmt.Errorf("unsupported action type: %v", action.Type)
	}

	return createEntry, nil
}

// convertBeamSpecToUpdateBeam converts a BeamSpec to an UpdateBeam proto.
// This is a simplified conversion for Scope 4.
func convertBeamSpecToUpdateBeam(beam *sbi.BeamSpec) *schedulingpb.UpdateBeam {
	// Build a minimal Beam proto
	beamProto := &schedulingpb.Beam{
		AntennaId: beam.InterfaceID,
	}

	// Populate Endpoints with target node ID so agent can extract TargetNodeID
	// For Scope 4, we use a simplified approach where the endpoint key is the target node ID
	if beam.TargetNodeID != "" {
		beamProto.Endpoints = map[string]*schedulingpb.Endpoint{
			beam.TargetNodeID: {}, // Empty endpoint is sufficient for Scope 4
		}
	}

	return &schedulingpb.UpdateBeam{
		Beam: beamProto,
	}
}

// convertBeamSpecToDeleteBeam converts a BeamSpec to a DeleteBeam proto.
// For Scope 4, we use the interface ID as the beam ID.
func convertBeamSpecToDeleteBeam(beam *sbi.BeamSpec) *schedulingpb.DeleteBeam {
	// Use interface ID as beam ID (simplified for Scope 4)
	beamID := beam.InterfaceID
	if beamID == "" {
		// Fallback: construct a beam ID from node and interface
		beamID = beam.NodeID + ":" + beam.InterfaceID
	}

	return &schedulingpb.DeleteBeam{
		Id: beamID,
	}
}

// convertRouteEntryToSetRoute converts a RouteEntry to a SetRoute proto.
func convertRouteEntryToSetRoute(route *model.RouteEntry) *schedulingpb.SetRoute {
	// SetRoute uses From (source), To (destination), Via (next hop), Dev (output interface)
	// RouteEntry has DestinationCIDR, NextHopNodeID, OutInterfaceID
	// For Scope 4, we'll use DestinationCIDR for both From and To (simplified)
	return &schedulingpb.SetRoute{
		From: "",                    // Source prefix - not in RouteEntry, leave empty
		To:   route.DestinationCIDR, // Destination prefix
		Via:  route.NextHopNodeID,   // Next hop address/node
		Dev:  route.OutInterfaceID,  // Output device/interface
	}
}

// convertRouteEntryToDeleteRoute converts a RouteEntry to a DeleteRoute proto.
func convertRouteEntryToDeleteRoute(route *model.RouteEntry) *schedulingpb.DeleteRoute {
	return &schedulingpb.DeleteRoute{
		From: route.DestinationCIDR, // From is source prefix
		To:   route.DestinationCIDR, // To is destination prefix
	}
}

// convertSrPolicySpecToSetSrPolicy converts an SrPolicySpec to a SetSrPolicy proto.
// This is a stub implementation for Scope 4.
func convertSrPolicySpecToSetSrPolicy(srPolicy *sbi.SrPolicySpec) *schedulingpb.SetSrPolicy {
	// Minimal stub for Scope 4
	return &schedulingpb.SetSrPolicy{
		Id: srPolicy.PolicyID,
	}
}

// convertSrPolicySpecToDeleteSrPolicy converts an SrPolicySpec to a DeleteSrPolicy proto.
// This is a stub implementation for Scope 4.
func convertSrPolicySpecToDeleteSrPolicy(srPolicy *sbi.SrPolicySpec) *schedulingpb.DeleteSrPolicy {
	return &schedulingpb.DeleteSrPolicy{
		Id: srPolicy.PolicyID,
	}
}

// generateToken generates a random token for schedule manipulation.
func generateToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "fallback-token"
	}
	return hex.EncodeToString(b[:])
}

