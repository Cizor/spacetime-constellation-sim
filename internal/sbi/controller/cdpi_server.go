package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"

	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
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
}

// AgentHandle represents an active agent connection to the controller.
// It tracks the agent's stream, outgoing message channel, and current token.
type AgentHandle struct {
	AgentID  string
	NodeID   string
	Stream   grpc.BidiStreamingServer[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController]
	outgoing chan *schedulingpb.ReceiveRequestsMessageFromController
	token    string
	seqNo    uint64 // monotonically increasing sequence number per agent
}

// NewCDPIServer creates a new CDPI server with the given dependencies.
func NewCDPIServer(state *state.ScenarioState, clock sbi.EventScheduler) *CDPIServer {
	return &CDPIServer{
		State:  state,
		Clock:  clock,
		agents: make(map[string]*AgentHandle),
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

	// Clean up on exit
	defer func() {
		s.agentsMu.Lock()
		delete(s.agents, agentID)
		close(handle.outgoing)
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
			close(handle.outgoing)
			<-sendDone // Wait for send goroutine to finish
			return err
		}

		// Handle different message types
		switch {
		case msg.GetHello() != nil:
			// Hello should only come once at the start
			// If we receive it again, it's an error
			return status.Error(codes.InvalidArgument, "Hello message received after initial handshake")

		case msg.GetResponse() != nil:
			// Handle Response
			response := msg.GetResponse()
			// Log status for observability
			// TODO: Add proper logging
			_ = response.GetRequestId()
			_ = response.GetStatus()

		default:
			// Unknown message type - ignore for now
			// Note: Reset is handled via the separate Reset RPC, not through the stream
		}
	}
}

// Reset implements the Reset RPC for CDPI.
// It clears any existing schedule entries for the agent and issues a fresh token.
func (s *CDPIServer) Reset(ctx context.Context, req *schedulingpb.ResetRequest) (*emptypb.Empty, error) {
	agentID := req.GetAgentId()
	if agentID == "" {
		return nil, status.Error(codes.InvalidArgument, "ResetRequest must contain agent_id")
	}

	s.agentsMu.Lock()
	defer s.agentsMu.Unlock()

	handle, exists := s.agents[agentID]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "agent %q not found", agentID)
	}

	// Clear any existing schedule entries (this will be handled by the agent)
	// Issue a fresh token
	handle.token = generateToken()
	handle.seqNo = 0

	return &emptypb.Empty{}, nil
}

// generateToken generates a random token for schedule manipulation.
func generateToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "fallback-token"
	}
	return hex.EncodeToString(b[:])
}

