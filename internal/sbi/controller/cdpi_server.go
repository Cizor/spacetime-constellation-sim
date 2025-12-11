package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"google.golang.org/grpc"
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

// generateToken generates a random token for schedule manipulation.
func generateToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "fallback-token"
	}
	return hex.EncodeToString(b[:])
}

