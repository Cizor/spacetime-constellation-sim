package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
)

// DumpAgentState returns a human-readable string representation of the
// agent's current state for debugging purposes.
// If telemetry is provided, it includes last known telemetry metrics for
// interfaces on this agent's node.
func (a *SimAgent) DumpAgentState(telemetry *state.TelemetryState) string {
	a.mu.Lock()
	defer a.mu.Unlock()

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("Agent: %s (NodeID: %s)\n", a.AgentID, a.NodeID))
	buf.WriteString(fmt.Sprintf("Token: %s\n", a.token))
	buf.WriteString(fmt.Sprintf("Last SeqNo Seen: %d\n", a.lastSeqNoSeen))
	buf.WriteString(fmt.Sprintf("Pending Actions: %d\n", len(a.pending)))

	// List pending actions
	if len(a.pending) > 0 {
		buf.WriteString("Pending Scheduled Actions:\n")
		for entryID, action := range a.pending {
			buf.WriteString(fmt.Sprintf("  - EntryID: %s\n", entryID))
			buf.WriteString(fmt.Sprintf("    Type: %s\n", action.Type.String()))
			buf.WriteString(fmt.Sprintf("    When: %s\n", action.When.Format(time.RFC3339)))
			if action.RequestID != "" {
				buf.WriteString(fmt.Sprintf("    RequestID: %s\n", action.RequestID))
			}
			if action.SeqNo > 0 {
				buf.WriteString(fmt.Sprintf("    SeqNo: %d\n", action.SeqNo))
			}
			// Add action-specific details
			if action.Beam != nil {
				buf.WriteString(fmt.Sprintf("    Beam: %s/%s -> %s/%s\n",
					action.Beam.NodeID, action.Beam.InterfaceID,
					action.Beam.TargetNodeID, action.Beam.TargetIfID))
			}
			if action.Route != nil {
				buf.WriteString(fmt.Sprintf("    Route: %s via %s@%s\n",
					action.Route.DestinationCIDR,
					action.Route.NextHopNodeID,
					action.Route.OutInterfaceID))
			}
			if action.SrPolicy != nil {
				buf.WriteString(fmt.Sprintf("    SR Policy: %s\n", action.SrPolicy.PolicyID))
			}
		}
	} else {
		buf.WriteString("  (no pending actions)\n")
	}

	// Add SR policies if any
	a.srMu.Lock()
	srPolicyCount := len(a.srPolicies)
	a.srMu.Unlock()
	if srPolicyCount > 0 {
		buf.WriteString(fmt.Sprintf("SR Policies: %d\n", srPolicyCount))
	}

	// Add telemetry metrics if available
	if telemetry != nil {
		// Get all interfaces for this node
		// We need to iterate over all interfaces - TelemetryState doesn't have ListInterfacesForNode
		// So we'll use GetMetrics with empty interfaceID to get all, or we can check known interfaces
		// For now, let's check if we can get metrics for any interface
		// Since TelemetryState uses "nodeID/interfaceID" as key, we need to know interface IDs
		// Let's get interfaces from State if available
		if a.State != nil {
			interfaces := a.State.ListInterfacesForNode(a.NodeID)
			if len(interfaces) > 0 {
				buf.WriteString("\nLast Telemetry Metrics:\n")
				for _, iface := range interfaces {
					if iface == nil {
						continue
					}
					metrics := telemetry.GetMetrics(a.NodeID, iface.ID)
					if metrics != nil {
						buf.WriteString(fmt.Sprintf("  Interface: %s\n", iface.ID))
						buf.WriteString(fmt.Sprintf("    Up: %v\n", metrics.Up))
						buf.WriteString(fmt.Sprintf("    BytesTx: %d\n", metrics.BytesTx))
						buf.WriteString(fmt.Sprintf("    BytesRx: %d\n", metrics.BytesRx))
						if metrics.SNRdB > 0 {
							buf.WriteString(fmt.Sprintf("    SNRdB: %.2f\n", metrics.SNRdB))
						}
						if metrics.Modulation != "" {
							buf.WriteString(fmt.Sprintf("    Modulation: %s\n", metrics.Modulation))
						}
					}
				}
			}
		}
	}

	// Add metrics snapshot if available
	if a.Metrics != nil {
		snap := a.Metrics.Snapshot()
		buf.WriteString("\nMetrics Snapshot:\n")
		buf.WriteString(fmt.Sprintf("  Actions Executed: %d\n", snap.NumActionsExecuted))
	}

	return buf.String()
}

