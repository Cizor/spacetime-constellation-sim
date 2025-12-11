package sbi

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// AgentID identifies a simulated agent (typically maps to a node/platform ID).
type AgentID string

// ActionKind represents the type of scheduled action.
type ActionKind int

const (
	ActionKindUnknown ActionKind = iota
	ActionKindUpdateBeam
	ActionKindDeleteBeam
	ActionKindSetRoute
	ActionKindDeleteRoute
	ActionKindSetSrPolicy    // stub, used later
	ActionKindDeleteSrPolicy // stub, used later
)

// ScheduledAction represents a single action scheduled for execution at a specific
// simulation time. This is a minimal placeholder that will be extended in Chunk 2
// with detailed payloads (BeamSpec, RouteEntry, etc.) and controller-side metadata.
type ScheduledAction struct {
	ID      string    // internal entry ID or schedule entry ID
	AgentID AgentID  // which agent/node this action is for
	Kind    ActionKind
	When    time.Time // simulation time; using time.Time keeps it consistent with SimClock/EventScheduler

	// TODO (Chunk 2): add BeamSpec, RouteEntry, SrPolicySpec, and controller-side metadata
	// (RequestID, SeqNo, schedule_manipulation_token, etc.)
}

// Agent represents a per-node simulated agent from the controller's perspective.
// The controller sends scheduled actions to agents via HandleScheduledAction.
type Agent interface {
	ID() AgentID
	HandleScheduledAction(ctx context.Context, action *ScheduledAction) error
}

// AgentRegistry manages registration and lookup of agents.
type AgentRegistry interface {
	Register(agent Agent) error
	Get(agentID AgentID) (Agent, bool)
	Unregister(agentID AgentID)
}

// InMemoryAgentRegistry is a thread-safe in-memory implementation of AgentRegistry.
type InMemoryAgentRegistry struct {
	mu     sync.RWMutex
	agents map[AgentID]Agent
}

// NewInMemoryAgentRegistry creates a new in-memory agent registry.
func NewInMemoryAgentRegistry() *InMemoryAgentRegistry {
	return &InMemoryAgentRegistry{
		agents: make(map[AgentID]Agent),
	}
}

// Register adds an agent to the registry. Returns an error if the agent is already registered.
func (r *InMemoryAgentRegistry) Register(agent Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := agent.ID()
	if _, exists := r.agents[id]; exists {
		return fmt.Errorf("agent %s already registered", id)
	}
	r.agents[id] = agent
	return nil
}

// Get retrieves an agent by ID. Returns the agent and true if found, nil and false otherwise.
func (r *InMemoryAgentRegistry) Get(agentID AgentID) (Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[agentID]
	return a, ok
}

// Unregister removes an agent from the registry.
func (r *InMemoryAgentRegistry) Unregister(agentID AgentID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, agentID)
}

