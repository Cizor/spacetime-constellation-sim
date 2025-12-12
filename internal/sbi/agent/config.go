// Package agent contains agent configuration types.
package agent

import (
	"time"
)

// TelemetryConfig holds configuration for agent telemetry emission.
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

// ApplyDefaults applies default values to config fields that are zero or invalid.
// Zero value config (both fields zero) defaults to Enabled=true, Interval=1s.
// If Enabled is false (even if zero value), telemetry is disabled and Interval is set to 0.
// If Enabled is true but Interval is <= 0, Interval is set to 1 second.
func (c TelemetryConfig) ApplyDefaults() TelemetryConfig {
	// Check if this is truly zero value (both fields are zero)
	isZeroValue := !c.Enabled && c.Interval == 0
	
	// Zero value -> use defaults (enabled=true, interval=1s)
	if isZeroValue {
		return TelemetryConfig{
			Enabled:  true,
			Interval: 1 * time.Second,
		}
	}
	
	// Explicitly disabled (Enabled=false, regardless of interval)
	if !c.Enabled {
		return TelemetryConfig{
			Enabled:  false,
			Interval: 0,
		}
	}
	
	// Enabled but zero/negative interval -> use default interval
	if c.Interval <= 0 {
		c.Interval = 1 * time.Second
	}
	return c
}

