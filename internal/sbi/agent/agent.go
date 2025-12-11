package agent

import (
	"context"

	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
)

// SimAgent represents a simulated agent for a network node.
// This is a minimal skeleton that will be extended in later chunks with:
// - ScenarioState reference
// - EventScheduler for time-based execution
// - Telemetry client
// - CDPI client
type SimAgent struct {
	id sbi.AgentID
	// TODO (later chunks): add ScenarioState, EventScheduler, telemetry client, CDPI client, etc.
}

// NewSimAgent creates a new simulated agent with the given ID.
func NewSimAgent(id sbi.AgentID) *SimAgent {
	return &SimAgent{id: id}
}

// ID returns the agent's identifier.
func (a *SimAgent) ID() sbi.AgentID {
	return a.id
}

// HandleScheduledAction accepts a scheduled action from the controller.
// For now, this is a stub that will be implemented in later chunks to:
// - Insert the action into the agent's local schedule
// - Use EventScheduler and ScenarioState to execute at the correct simulation time
func (a *SimAgent) HandleScheduledAction(ctx context.Context, action *sbi.ScheduledAction) error {
	// TODO (later chunks): implement real scheduling and execution logic
	// For now, just validate the action
	if err := action.Validate(); err != nil {
		return err
	}
	return nil
}

