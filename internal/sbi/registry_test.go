package sbi

import (
	"context"
	"testing"
)

// dummyAgent is a test implementation of Agent for registry tests.
type dummyAgent struct {
	id AgentID
}

func (d *dummyAgent) ID() AgentID {
	return d.id
}

func (d *dummyAgent) HandleScheduledAction(ctx context.Context, action *ScheduledAction) error {
	return nil
}

func TestInMemoryAgentRegistry_Register(t *testing.T) {
	reg := NewInMemoryAgentRegistry()

	agent1 := &dummyAgent{id: "agent-1"}
	if err := reg.Register(agent1); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify agent is registered
	got, ok := reg.Get("agent-1")
	if !ok {
		t.Fatalf("Get returned ok=false after Register")
	}
	if got.ID() != "agent-1" {
		t.Fatalf("Get returned wrong agent: got ID %q, want %q", got.ID(), "agent-1")
	}
}

func TestInMemoryAgentRegistry_DuplicateRegister(t *testing.T) {
	reg := NewInMemoryAgentRegistry()

	agent1 := &dummyAgent{id: "agent-1"}
	if err := reg.Register(agent1); err != nil {
		t.Fatalf("First Register failed: %v", err)
	}

	// Duplicate register should fail
	agent2 := &dummyAgent{id: "agent-1"}
	if err := reg.Register(agent2); err == nil {
		t.Fatalf("Duplicate Register should have returned error")
	}
}

func TestInMemoryAgentRegistry_Unregister(t *testing.T) {
	reg := NewInMemoryAgentRegistry()

	agent1 := &dummyAgent{id: "agent-1"}
	if err := reg.Register(agent1); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify agent is registered
	_, ok := reg.Get("agent-1")
	if !ok {
		t.Fatalf("Get returned ok=false before Unregister")
	}

	// Unregister
	reg.Unregister("agent-1")

	// Verify agent is gone
	_, ok = reg.Get("agent-1")
	if ok {
		t.Fatalf("Get returned ok=true after Unregister")
	}
}

func TestInMemoryAgentRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewInMemoryAgentRegistry()

	// Concurrent registration
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			agent := &dummyAgent{id: AgentID(string(rune('a' + id)))}
			_ = reg.Register(agent)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all agents are registered
	for i := 0; i < 10; i++ {
		id := AgentID(string(rune('a' + i)))
		_, ok := reg.Get(id)
		if !ok {
			t.Fatalf("Agent %q not found after concurrent registration", id)
		}
	}
}

