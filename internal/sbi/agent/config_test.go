package agent

import (
	"testing"
	"time"
)

func TestTelemetryConfig_ApplyDefaults_ZeroValue(t *testing.T) {
	cfg := TelemetryConfig{}
	applied := cfg.ApplyDefaults()

	// Zero value should default to enabled=true, interval=1s
	if !applied.Enabled {
		t.Fatalf("expected Enabled=true for zero value config (default), got %v", applied.Enabled)
	}
	if applied.Interval != 1*time.Second {
		t.Fatalf("expected Interval=1s for zero value config, got %v", applied.Interval)
	}
}

func TestTelemetryConfig_ApplyDefaults_Disabled(t *testing.T) {
	// Note: We can't distinguish between zero value and explicitly disabled
	// when both Enabled=false and Interval=0. The current implementation
	// treats zero value as "use defaults" (enabled). To explicitly disable,
	// use DefaultTelemetryConfig() and then set Enabled=false.
	cfg := DefaultTelemetryConfig()
	cfg.Enabled = false
	applied := cfg.ApplyDefaults()

	if applied.Enabled {
		t.Fatalf("expected Enabled=false when explicitly disabled")
	}
	if applied.Interval != 0 {
		t.Fatalf("expected Interval=0 when disabled, got %v", applied.Interval)
	}
}

func TestTelemetryConfig_ApplyDefaults_CustomInterval(t *testing.T) {
	cfg := TelemetryConfig{
		Enabled:  true,
		Interval: 500 * time.Millisecond,
	}
	applied := cfg.ApplyDefaults()

	if !applied.Enabled {
		t.Fatalf("expected Enabled=true")
	}
	if applied.Interval != 500*time.Millisecond {
		t.Fatalf("expected Interval=500ms, got %v", applied.Interval)
	}
}

func TestTelemetryConfig_ApplyDefaults_ZeroInterval(t *testing.T) {
	cfg := TelemetryConfig{
		Enabled:  true,
		Interval: 0,
	}
	applied := cfg.ApplyDefaults()

	if !applied.Enabled {
		t.Fatalf("expected Enabled=true")
	}
	if applied.Interval != 1*time.Second {
		t.Fatalf("expected Interval=1s (default) for zero interval, got %v", applied.Interval)
	}
}

func TestDefaultTelemetryConfig(t *testing.T) {
	cfg := DefaultTelemetryConfig()

	if !cfg.Enabled {
		t.Fatalf("expected Enabled=true in default config")
	}
	if cfg.Interval != 1*time.Second {
		t.Fatalf("expected Interval=1s in default config, got %v", cfg.Interval)
	}
}

