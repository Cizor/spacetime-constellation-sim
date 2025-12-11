package sbi

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/signalsfoundry/constellation-simulator/model"
)

// AgentID identifies a simulated agent (typically maps to a node/platform ID).
type AgentID string

// ScheduledActionType represents the type of scheduled action.
// This enum covers all action types that can be scheduled via SBI.
type ScheduledActionType int

const (
	ScheduledActionUnknown ScheduledActionType = iota
	ScheduledUpdateBeam
	ScheduledDeleteBeam
	ScheduledSetRoute
	ScheduledDeleteRoute
	ScheduledSetSrPolicy    // stub for future behavior
	ScheduledDeleteSrPolicy // stub for future behavior
)

// ActionKind is an alias for ScheduledActionType for backward compatibility.
// New code should use ScheduledActionType.
type ActionKind = ScheduledActionType

// Backward compatibility constants
const (
	ActionKindUnknown       = ScheduledActionUnknown
	ActionKindUpdateBeam    = ScheduledUpdateBeam
	ActionKindDeleteBeam    = ScheduledDeleteBeam
	ActionKindSetRoute      = ScheduledSetRoute
	ActionKindDeleteRoute   = ScheduledDeleteRoute
	ActionKindSetSrPolicy   = ScheduledSetSrPolicy
	ActionKindDeleteSrPolicy = ScheduledDeleteSrPolicy
)

// BeamSpec describes a beam/pointing configuration for activating a link.
// This is an internal type that carries the semantic information needed to
// identify and activate a link between two interfaces.
type BeamSpec struct {
	// NodeID is the ID of the node that owns the beam (agent/node)
	NodeID string

	// InterfaceID is the local interface/antenna ID on that node
	InterfaceID string

	// TargetNodeID is the ID of the peer node, if applicable
	TargetNodeID string

	// TargetIfID is the peer interface ID, if applicable
	TargetIfID string

	// Optional RF / link parameters (keep this minimal for now)
	FrequencyHz float64
	BandwidthHz float64
	PowerDBw    float64

	// TODO: extend with pointing angles / coordinates if needed later
}

// SrPolicySpec describes an SR (Segment Routing) policy configuration.
// For Scope 4, this is a stub that accepts SBI messages but doesn't
// affect forwarding behavior yet.
type SrPolicySpec struct {
	PolicyID string
	// TODO: add segment list, preferences, etc. in later scopes
}

// ScheduledAction represents a single action scheduled for execution at a specific
// simulation time. This is the full domain model that carries all information
// needed to execute the action and correlate it with SBI protocol messages.
type ScheduledAction struct {
	// Identity
	EntryID string              // schedule entry ID (from SBI)
	AgentID AgentID             // which agent/node this action is for
	Type    ScheduledActionType // what kind of action this is

	// Timing (simulation clock)
	When time.Time // simulation time at which the action should execute

	// Controller-side metadata from SBI
	RequestID string // SBI request_id for correlation
	SeqNo     int64  // SBI sequence number (per agent)
	Token     string // schedule_manipulation_token

	// Payloads (only one should be non-nil / meaningful depending on Type)
	Beam     *BeamSpec
	Route    *model.RouteEntry // reuse existing RouteEntry type from model package
	SrPolicy *SrPolicySpec
}

// Validate checks that the ScheduledAction is well-formed.
// Returns an error if the action is invalid, nil otherwise.
func (a *ScheduledAction) Validate() error {
	if a.Type == ScheduledActionUnknown {
		return errors.New("ScheduledAction.Type must not be ScheduledActionUnknown")
	}
	if a.EntryID == "" {
		return errors.New("ScheduledAction.EntryID must not be empty")
	}
	if a.When.IsZero() {
		return errors.New("ScheduledAction.When must not be zero")
	}

	// Validate that the appropriate payload is present based on Type
	switch a.Type {
	case ScheduledUpdateBeam, ScheduledDeleteBeam:
		if a.Beam == nil {
			return fmt.Errorf("ScheduledAction with Type %v must have non-nil Beam", a.Type)
		}
	case ScheduledSetRoute, ScheduledDeleteRoute:
		if a.Route == nil {
			return fmt.Errorf("ScheduledAction with Type %v must have non-nil Route", a.Type)
		}
	case ScheduledSetSrPolicy, ScheduledDeleteSrPolicy:
		if a.SrPolicy == nil {
			return fmt.Errorf("ScheduledAction with Type %v must have non-nil SrPolicy", a.Type)
		}
	}

	return nil
}

// ActionMeta contains controller-side metadata for scheduled actions.
// This is a convenience struct for constructing actions with metadata.
type ActionMeta struct {
	RequestID string
	SeqNo     int64
	Token     string
}

// NewBeamAction creates a new ScheduledAction for a beam-related operation.
func NewBeamAction(entryID string, agentID AgentID, actionType ScheduledActionType, when time.Time, beam *BeamSpec, meta ActionMeta) *ScheduledAction {
	return &ScheduledAction{
		EntryID:   entryID,
		AgentID:   agentID,
		Type:      actionType,
		When:      when,
		RequestID: meta.RequestID,
		SeqNo:     meta.SeqNo,
		Token:     meta.Token,
		Beam:      beam,
	}
}

// NewRouteAction creates a new ScheduledAction for a route-related operation.
func NewRouteAction(entryID string, agentID AgentID, actionType ScheduledActionType, when time.Time, route *model.RouteEntry, meta ActionMeta) *ScheduledAction {
	return &ScheduledAction{
		EntryID:   entryID,
		AgentID:   agentID,
		Type:      actionType,
		When:      when,
		RequestID: meta.RequestID,
		SeqNo:     meta.SeqNo,
		Token:     meta.Token,
		Route:     route,
	}
}

// NewSrPolicyAction creates a new ScheduledAction for an SR policy-related operation.
func NewSrPolicyAction(entryID string, agentID AgentID, actionType ScheduledActionType, when time.Time, srPolicy *SrPolicySpec, meta ActionMeta) *ScheduledAction {
	return &ScheduledAction{
		EntryID:   entryID,
		AgentID:   agentID,
		Type:      actionType,
		When:      when,
		RequestID: meta.RequestID,
		SeqNo:     meta.SeqNo,
		Token:     meta.Token,
		SrPolicy:  srPolicy,
	}
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

