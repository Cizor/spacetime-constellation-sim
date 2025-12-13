package controller

import (
	"fmt"
	"strings"
)

// DumpAgentState returns debug info for a specific agent.
// It returns information about the agent handle if the agent is connected.
// For full agent state (including pending actions), use the agent's DumpAgentState method directly.
func (s *CDPIServer) DumpAgentState(agentID string) (string, error) {
	s.agentsMu.RLock()
	defer s.agentsMu.RUnlock()

	handle, ok := s.agents[agentID]
	if !ok {
		return "", fmt.Errorf("agent %s not found", agentID)
	}

	var buf strings.Builder
	buf.WriteString("CDPI Agent Handle:\n")
	buf.WriteString(fmt.Sprintf("  AgentID: %s\n", handle.AgentID))
	buf.WriteString(fmt.Sprintf("  NodeID: %s\n", handle.NodeID))
	buf.WriteString(fmt.Sprintf("  Token: %s\n", handle.CurrentToken()))
	buf.WriteString(fmt.Sprintf("  SeqNo: %d\n", handle.seqNo))

	return buf.String(), nil
}

