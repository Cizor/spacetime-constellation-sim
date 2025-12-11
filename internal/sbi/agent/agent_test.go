package agent

import (
	"context"
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/model"
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

	route := &model.RouteEntry{
		DestinationCIDR: "10.0.0.0/24",
		NextHopNodeID:   "nodeB",
		OutInterfaceID:  "if1",
	}
	meta := sbi.ActionMeta{
		RequestID: "req-1",
		SeqNo:     1,
		Token:     "token-abc",
	}
	action := sbi.NewRouteAction("action-1", id, sbi.ScheduledSetRoute, time.Now(), route, meta)

	// Should not panic and return nil (stub implementation)
	if err := agent.HandleScheduledAction(context.Background(), action); err != nil {
		t.Fatalf("HandleScheduledAction returned error: %v", err)
	}
}

