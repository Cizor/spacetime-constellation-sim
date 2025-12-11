package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"

	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"google.golang.org/grpc"
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

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// NewSimAgent creates a new simulated agent with the given ID and dependencies.
func NewSimAgent(agentID sbi.AgentID, nodeID string, state *state.ScenarioState, scheduler sbi.EventScheduler, telemetryCli telemetrypb.TelemetryClient, stream grpc.BidiStreamingClient[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController]) *SimAgent {
	return &SimAgent{
		AgentID:      agentID,
		NodeID:       nodeID,
		State:        state,
		Scheduler:    scheduler,
		TelemetryCli: telemetryCli,
		Stream:       stream,
		pending:      make(map[string]*sbi.ScheduledAction),
		token:        generateToken(),
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

// execute executes a scheduled action and sends a Response.
// This is called by the EventScheduler when the action's time arrives.
// TODO (4.3): Implement full execution logic for all action types.
func (a *SimAgent) execute(action *sbi.ScheduledAction) {
	// TODO: Implement execution logic in 4.3
	_ = action
}

// generateToken generates a random token for schedule manipulation.
func generateToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "fallback-token"
	}
	return hex.EncodeToString(b[:])
}

