// Package sbi contains telemetry configuration types.
// Note: Telemetry wiring helpers are in internal/sbi/controller package.
package sbi

import (
	"time"
)

// TelemetryConfig holds configuration for telemetry emission.
type TelemetryConfig struct {
	// Enabled indicates whether telemetry is enabled.
	// Default: true
	Enabled bool

	// Interval is the simulation time interval between telemetry emissions.
	// Default: 1 second
	Interval time.Duration
}

// DefaultTelemetryConfig returns a TelemetryConfig with sensible defaults.
func DefaultTelemetryConfig() TelemetryConfig {
	return TelemetryConfig{
		Enabled:  true,
		Interval: 1 * time.Second,
	}
}

