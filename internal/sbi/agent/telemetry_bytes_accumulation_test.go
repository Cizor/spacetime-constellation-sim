package agent

import (
	"context"
	"testing"
	"time"

	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

// TestSimAgent_TelemetryBytes_IncreaseWhenUp verifies that BytesTx
// increases monotonically when interface is up and bandwidth > 0.
func TestSimAgent_TelemetryBytes_IncreaseWhenUp(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)

	// Create nodes with interfaces
	node1 := &model.NetworkNode{ID: "node1"}
	node2 := &model.NetworkNode{ID: "node2"}
	if err := scenarioState.CreateNode(node1, []*core.NetworkInterface{
		{ID: "if1", ParentNodeID: "node1", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode(node1) failed: %v", err)
	}
	if err := scenarioState.CreateNode(node2, []*core.NetworkInterface{
		{ID: "if2", ParentNodeID: "node2", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode(node2) failed: %v", err)
	}

	// Create an active link with known bandwidth (8000 bps = 1 kB/s)
	link := &core.NetworkLink{
		ID:             "link1",
		InterfaceA:     "if1",
		InterfaceB:     "if2",
		Medium:         core.MediumWireless,
		Status:         core.LinkStatusActive,
		IsUp:           true,
		MaxDataRateMbps: 0.008, // 8 kbps = 8000 bps = 1 kB/s
	}
	if err := scenarioState.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	scheduler := sbi.NewFakeEventScheduler(startTime)
	telemetryCli := &fakeTelemetryClientForTesting{}
	stream := &fakeStreamForTelemetry{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream)

	ctx := context.Background()
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer agent.Stop()

	// First tick at T0 + 1s
	scheduler.AdvanceTo(startTime.Add(1 * time.Second))
	time.Sleep(50 * time.Millisecond)

	calls := telemetryCli.getCalls()
	if len(calls) < 1 {
		t.Fatalf("expected at least 1 telemetry call, got %d", len(calls))
	}

	// Get first call's BytesTx
	firstCall := calls[0]
	if len(firstCall.GetInterfaceMetrics()) == 0 {
		t.Fatalf("expected at least one interface metric")
	}
	firstMetrics := firstCall.GetInterfaceMetrics()[0]
	firstBytesTx := int64(0)
	if len(firstMetrics.GetStandardInterfaceStatisticsDataPoints()) > 0 {
		if firstMetrics.GetStandardInterfaceStatisticsDataPoints()[0].TxBytes != nil {
			firstBytesTx = *firstMetrics.GetStandardInterfaceStatisticsDataPoints()[0].TxBytes
		}
	}

	// Second tick at T0 + 2s (another 1 second interval)
	scheduler.AdvanceTo(startTime.Add(2 * time.Second))
	time.Sleep(50 * time.Millisecond)

	calls = telemetryCli.getCalls()
	if len(calls) < 2 {
		t.Fatalf("expected at least 2 telemetry calls, got %d", len(calls))
	}

	// Get second call's BytesTx
	secondCall := calls[1]
	if len(secondCall.GetInterfaceMetrics()) == 0 {
		t.Fatalf("expected at least one interface metric in second call")
	}
	secondMetrics := secondCall.GetInterfaceMetrics()[0]
	secondBytesTx := int64(0)
	if len(secondMetrics.GetStandardInterfaceStatisticsDataPoints()) > 0 {
		if secondMetrics.GetStandardInterfaceStatisticsDataPoints()[0].TxBytes != nil {
			secondBytesTx = *secondMetrics.GetStandardInterfaceStatisticsDataPoints()[0].TxBytes
		}
	}

	// Verify BytesTx increased
	// Expected: firstBytesTx ≈ 1000 bytes (1s * 8000 bps / 8), secondBytesTx ≈ 2000 bytes (2s * 8000 bps / 8)
	// Allow for integer rounding
	if secondBytesTx <= firstBytesTx {
		t.Errorf("BytesTx did not increase: first=%d, second=%d", firstBytesTx, secondBytesTx)
	}
	if firstBytesTx < 500 || firstBytesTx > 1500 {
		t.Logf("First BytesTx = %d (expected ~1000, allowing for rounding)", firstBytesTx)
	}
	if secondBytesTx < 1500 || secondBytesTx > 2500 {
		t.Logf("Second BytesTx = %d (expected ~2000, allowing for rounding)", secondBytesTx)
	}
}

// TestSimAgent_TelemetryBytes_NoIncreaseWhenDown verifies that BytesTx
// does not increase when interface is down.
func TestSimAgent_TelemetryBytes_NoIncreaseWhenDown(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)

	// Create nodes with interfaces
	node1 := &model.NetworkNode{ID: "node1"}
	node2 := &model.NetworkNode{ID: "node2"}
	if err := scenarioState.CreateNode(node1, []*core.NetworkInterface{
		{ID: "if1", ParentNodeID: "node1", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode(node1) failed: %v", err)
	}
	if err := scenarioState.CreateNode(node2, []*core.NetworkInterface{
		{ID: "if2", ParentNodeID: "node2", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode(node2) failed: %v", err)
	}

	// Create a link that is NOT active (Potential status, IsUp=false)
	link := &core.NetworkLink{
		ID:             "link1",
		InterfaceA:     "if1",
		InterfaceB:     "if2",
		Medium:         core.MediumWireless,
		Status:         core.LinkStatusPotential, // Not active
		IsUp:           false,                    // Down
		MaxDataRateMbps: 100.0,                   // High bandwidth, but interface is down
	}
	if err := scenarioState.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	scheduler := sbi.NewFakeEventScheduler(startTime)
	telemetryCli := &fakeTelemetryClientForTesting{}
	stream := &fakeStreamForTelemetry{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream)

	ctx := context.Background()
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer agent.Stop()

	// Multiple ticks
	scheduler.AdvanceTo(startTime.Add(1 * time.Second))
	time.Sleep(50 * time.Millisecond)

	scheduler.AdvanceTo(startTime.Add(2 * time.Second))
	time.Sleep(50 * time.Millisecond)

	scheduler.AdvanceTo(startTime.Add(3 * time.Second))
	time.Sleep(50 * time.Millisecond)

	calls := telemetryCli.getCalls()
	if len(calls) == 0 {
		t.Fatalf("expected at least one telemetry call")
	}

	// Verify BytesTx remains 0 (or unchanged) across all calls
	for i, call := range calls {
		if len(call.GetInterfaceMetrics()) == 0 {
			continue
		}
		metrics := call.GetInterfaceMetrics()[0]
		if len(metrics.GetStandardInterfaceStatisticsDataPoints()) > 0 {
			txBytes := metrics.GetStandardInterfaceStatisticsDataPoints()[0].TxBytes
			if txBytes != nil && *txBytes > 0 {
				t.Errorf("Call %d: BytesTx = %d, expected 0 (interface is down)", i, *txBytes)
			}
		}
		// Verify Up flag is false
		if len(metrics.GetOperationalStateDataPoints()) > 0 {
			status := metrics.GetOperationalStateDataPoints()[0].GetValue()
			if status == telemetrypb.IfOperStatus_IF_OPER_STATUS_UP {
				t.Errorf("Call %d: expected Up=false, got Up=true", i)
			}
		}
	}
}

// TestSimAgent_TelemetryDisabled verifies that telemetry loop
// does not schedule events when telemetry is disabled.
func TestSimAgent_TelemetryDisabled(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)

	node := &model.NetworkNode{ID: "node1"}
	if err := scenarioState.CreateNode(node, []*core.NetworkInterface{
		{ID: "if1", ParentNodeID: "node1", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	scheduler := sbi.NewFakeEventScheduler(startTime)
	telemetryCli := &fakeTelemetryClientForTesting{}
	stream := &fakeStreamForTelemetry{}

	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, telemetryCli, stream)

	// Disable telemetry by setting interval to 0
	agent.telemetryInterval = 0

	ctx := context.Background()
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer agent.Stop()

	// Advance time - telemetry should not be scheduled
	scheduler.AdvanceTo(startTime.Add(10 * time.Second))
	time.Sleep(100 * time.Millisecond)

	// Verify no telemetry calls were made
	calls := telemetryCli.getCalls()
	if len(calls) > 0 {
		t.Errorf("expected no telemetry calls when disabled, got %d", len(calls))
	}
}

// TestSimAgent_TelemetryDisabled_NilClient verifies that telemetry loop
// handles nil client gracefully.
func TestSimAgent_TelemetryDisabled_NilClient(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)

	node := &model.NetworkNode{ID: "node1"}
	if err := scenarioState.CreateNode(node, []*core.NetworkInterface{
		{ID: "if1", ParentNodeID: "node1", IsOperational: true},
	}); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	scheduler := sbi.NewFakeEventScheduler(startTime)
	stream := &fakeStreamForTelemetry{}

	// Create agent with nil telemetry client
	agent := NewSimAgent("agent-1", "node1", scenarioState, scheduler, nil, stream)

	ctx := context.Background()
	// Start should not panic even with nil client
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer agent.Stop()

	// Advance time
	scheduler.AdvanceTo(startTime.Add(2 * time.Second))
	time.Sleep(100 * time.Millisecond)

	// Should not panic - telemetryTick should handle nil client gracefully
	// (The exact behavior depends on implementation, but it shouldn't crash)
}

