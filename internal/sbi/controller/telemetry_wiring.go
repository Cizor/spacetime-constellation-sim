// Package controller contains controller-side SBI logic including telemetry wiring.
package controller

import (
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
)

// TelemetryComponents holds the telemetry server and state for controller-side wiring.
// This is the recommended way to construct and wire telemetry into the main gRPC server.
type TelemetryComponents struct {
	// Server is the TelemetryService gRPC server that should be registered
	// with the main gRPC server.
	Server *TelemetryServer

	// State is the TelemetryState that stores metrics received from agents.
	// It can be accessed for reading metrics (e.g., for NBI exposure in later chunks).
	State *state.TelemetryState
}

// NewTelemetryComponents creates a new telemetry stack with shared TelemetryState.
// This helper constructs both the TelemetryState and TelemetryServer, ensuring
// they are properly wired together.
//
// Usage:
//   - Construct once in controller setup
//   - Register TelemetryComponents.Server with the main gRPC server:
//     telemetrypb.RegisterTelemetryServer(grpcServer, components.Server)
func NewTelemetryComponents(log logging.Logger) *TelemetryComponents {
	ts := state.NewTelemetryState()
	srv := NewTelemetryServer(ts, log)
	return &TelemetryComponents{
		Server: srv,
		State:  ts,
	}
}

