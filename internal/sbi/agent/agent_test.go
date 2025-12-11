package agent

import (
	"context"
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
)

func TestSimAgent_ID(t *testing.T) {
	id := sbi.AgentID("test-agent-1")
	agent := NewSimAgent(id)

	if got := agent.ID(); got != id {
		t.Fatalf("ID() = %q, want %q", got, id)
	}
}

func TestSimAgent_HandleScheduledAction(t *testing.T) {
	id := sbi.AgentID("test-agent-1")
	agent := NewSimAgent(id)

	action := &sbi.ScheduledAction{
		ID:      "action-1",
		AgentID: id,
		Kind:    sbi.ActionKindSetRoute,
		When:    time.Now(),
	}

	// Should not panic and return nil (stub implementation)
	if err := agent.HandleScheduledAction(context.Background(), action); err != nil {
		t.Fatalf("HandleScheduledAction returned error: %v", err)
	}
}

