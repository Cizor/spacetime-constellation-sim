package controller

import (
	"testing"

	"github.com/signalsfoundry/constellation-simulator/internal/logging"
)

func TestNewTelemetryComponents(t *testing.T) {
	log := logging.Noop()
	components := NewTelemetryComponents(log)

	if components == nil {
		t.Fatalf("expected non-nil TelemetryComponents")
	}
	if components.Server == nil {
		t.Fatalf("expected non-nil Server")
	}
	if components.State == nil {
		t.Fatalf("expected non-nil State")
	}
	if components.Server.Telemetry != components.State {
		t.Fatalf("Server.Telemetry should reference the same State instance")
	}
}

func TestNewTelemetryComponents_NilLogger(t *testing.T) {
	components := NewTelemetryComponents(nil)

	if components == nil {
		t.Fatalf("expected non-nil TelemetryComponents even with nil logger")
	}
	if components.Server == nil {
		t.Fatalf("expected non-nil Server")
	}
	if components.State == nil {
		t.Fatalf("expected non-nil State")
	}
}

