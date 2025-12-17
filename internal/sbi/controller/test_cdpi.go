package controller

import (
	"sync"

	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
)

// TestCDPI is a lightweight CDPI client implementation for tests.
type TestCDPI struct {
	mu     sync.Mutex
	agents map[string]struct{}
}

// NewTestCDPI constructs a test CDPI client.
func NewTestCDPI() *TestCDPI {
	return &TestCDPI{
		agents: make(map[string]struct{}),
	}
}

// SendCreateEntry is a no-op implementation for tests.
func (t *TestCDPI) SendCreateEntry(agentID string, action *sbi.ScheduledAction) error {
	return nil
}

// SendDeleteEntry is a no-op implementation for tests.
func (t *TestCDPI) SendDeleteEntry(agentID, entryID string) error {
	return nil
}

// RegisterAgent makes the test client report that an agent is available.
func (t *TestCDPI) RegisterAgent(agentID string) {
	if agentID == "" {
		return
	}
	t.mu.Lock()
	t.agents[agentID] = struct{}{}
	t.mu.Unlock()
}

func (t *TestCDPI) hasAgent(agentID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.agents) == 0 {
		return true
	}
	_, ok := t.agents[agentID]
	return ok
}
