package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi/agent"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi/controller"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"google.golang.org/grpc"
)

// SBIRuntime encapsulates all SBI components and their lifecycle.
// It owns TelemetryState, TelemetryServer, CDPIServer, Scheduler, and Agents.
type SBIRuntime struct {
	// State is the shared scenario state
	State *sim.ScenarioState

	// Clock is the shared event scheduler
	Clock sbi.EventScheduler

	// Telemetry components
	TelemetryState *sim.TelemetryState
	TelemetryServer *controller.TelemetryServer

	// CDPI server
	CDPI *controller.CDPIServer

	// Controller scheduler
	Scheduler *controller.Scheduler

	// Agents (one per node)
	Agents []*agent.SimAgent

	// gRPC client connection for agents (in-process)
	conn *grpc.ClientConn

	// Logger
	log logging.Logger

	// Lifecycle
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NewSBIRuntimeWithServers creates a new SBI runtime using pre-created servers.
// This is useful when the servers need to be registered on a gRPC server before
// the runtime is fully initialized.
// Parameters:
//   - state: the scenario state
//   - clock: the event scheduler (bound to sim clock)
//   - telemetryState: pre-created telemetry state
//   - telemetryServer: pre-created telemetry server
//   - cdpiServer: pre-created CDPI server
//   - conn: gRPC client connection for agents to reach CDPI/Telemetry servers
//   - log: optional logger
func NewSBIRuntimeWithServers(
	state *sim.ScenarioState,
	clock sbi.EventScheduler,
	telemetryState *sim.TelemetryState,
	telemetryServer *controller.TelemetryServer,
	cdpiServer *controller.CDPIServer,
	conn *grpc.ClientConn,
	log logging.Logger,
) (*SBIRuntime, error) {
	if state == nil {
		return nil, fmt.Errorf("state is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("clock is nil")
	}
	if telemetryState == nil {
		return nil, fmt.Errorf("telemetryState is nil")
	}
	if telemetryServer == nil {
		return nil, fmt.Errorf("telemetryServer is nil")
	}
	if cdpiServer == nil {
		return nil, fmt.Errorf("cdpiServer is nil")
	}
	if conn == nil {
		return nil, fmt.Errorf("conn is nil")
	}
	if log == nil {
		log = logging.Noop()
	}

	// Create scheduler
	scheduler := controller.NewScheduler(state, clock, cdpiServer, log, telemetryState)

	// Create agents for each node
	// Note: Streams will be created in StartAgents when we have the gRPC connection
	nodes := state.ListNodes()
	agents := make([]*agent.SimAgent, 0, len(nodes))

	// Create telemetry client (shared by all agents)
	telemetryClient := telemetrypb.NewTelemetryClient(conn)

	for _, node := range nodes {
		if node == nil || node.ID == "" {
			continue
		}

		// For Scope 4, agent_id equals node_id
		agentID := sbi.AgentID(node.ID)

		// Create agent without stream (will be set in StartAgents)
		ag := agent.NewSimAgent(agentID, node.ID, state, clock, telemetryClient, nil, log)
		agents = append(agents, ag)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &SBIRuntime{
		State:          state,
		Clock:          clock,
		TelemetryState: telemetryState,
		TelemetryServer: telemetryServer,
		CDPI:          cdpiServer,
		Scheduler:     scheduler,
		Agents:        agents,
		conn:          conn,
		log:           log,
		ctx:           ctx,
		cancel:        cancel,
	}, nil
}

// NewSBIRuntime creates a new SBI runtime with all components wired together.
// Parameters:
//   - state: the scenario state
//   - clock: the event scheduler (bound to sim clock)
//   - conn: gRPC client connection for agents to reach CDPI/Telemetry servers
//   - log: optional logger
func NewSBIRuntime(state *sim.ScenarioState, clock sbi.EventScheduler, conn *grpc.ClientConn, log logging.Logger) (*SBIRuntime, error) {
	if state == nil {
		return nil, fmt.Errorf("state is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("clock is nil")
	}
	if conn == nil {
		return nil, fmt.Errorf("conn is nil")
	}
	if log == nil {
		log = logging.Noop()
	}

	// Create telemetry components
	telemetryState := sim.NewTelemetryState()
	telemetryServer := controller.NewTelemetryServer(telemetryState, log)

	// Create CDPI server
	cdpiServer := controller.NewCDPIServer(state, clock, log)

	// Create scheduler
	scheduler := controller.NewScheduler(state, clock, cdpiServer, log, telemetryState)

	// Create agents for each node
	// Note: Streams will be created in StartAgents when we have the gRPC connection
	nodes := state.ListNodes()
	agents := make([]*agent.SimAgent, 0, len(nodes))

	// Create telemetry client (shared by all agents)
	telemetryClient := telemetrypb.NewTelemetryClient(conn)

	for _, node := range nodes {
		if node == nil || node.ID == "" {
			continue
		}

		// For Scope 4, agent_id equals node_id
		agentID := sbi.AgentID(node.ID)

		// Create agent without stream (will be set in StartAgents)
		ag := agent.NewSimAgent(agentID, node.ID, state, clock, telemetryClient, nil, log)
		agents = append(agents, ag)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &SBIRuntime{
		State:          state,
		Clock:          clock,
		TelemetryState: telemetryState,
		TelemetryServer: telemetryServer,
		CDPI:          cdpiServer,
		Scheduler:     scheduler,
		Agents:        agents,
		conn:          conn,
		log:           log,
		ctx:           ctx,
		cancel:        cancel,
	}, nil
}

// StartAgents starts all agents, connecting them to the CDPI server.
// Each agent will send a Hello message and begin participating in SBI.
func (r *SBIRuntime) StartAgents(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create gRPC client for CDPI
	cdpiClient := schedulingpb.NewSchedulingClient(r.conn)

	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	for _, ag := range r.Agents {
		if ag == nil {
			continue
		}

		wg.Add(1)
		go func(a *agent.SimAgent) {
			defer wg.Done()

			// Create CDPI stream for this agent
			stream, err := cdpiClient.ReceiveRequests(ctx)
			if err != nil {
				r.log.Warn(ctx, "failed to create CDPI stream for agent",
					logging.String("agent_id", string(a.AgentID)),
					logging.String("error", err.Error()),
				)
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
				return
			}

			// Set the stream on the agent
			a.SetStream(stream)

			// Start the agent
			if err := a.Start(ctx); err != nil {
				r.log.Warn(ctx, "failed to start agent",
					logging.String("agent_id", string(a.AgentID)),
					logging.String("error", err.Error()),
				)
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
				return
			}

			r.log.Debug(ctx, "agent started",
				logging.String("agent_id", string(a.AgentID)),
				logging.String("node_id", a.NodeID),
			)
		}(ag)
	}

	// Wait for all agents to start (or first error)
	wg.Wait()

	return firstErr
}

// StopAgents stops all agents cleanly.
func (r *SBIRuntime) StopAgents() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cancel != nil {
		r.cancel()
	}

	for _, ag := range r.Agents {
		if ag != nil {
			ag.Stop()
		}
	}
}

// Close shuts down the SBI runtime, stopping agents and cleaning up resources.
func (r *SBIRuntime) Close() error {
	r.StopAgents()
	// Note: We don't close the gRPC connection here as it may be shared
	// The caller is responsible for closing the connection
	return nil
}
