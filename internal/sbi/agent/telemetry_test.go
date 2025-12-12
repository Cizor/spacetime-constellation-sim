package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

// fakeTelemetryClientForTesting captures ExportMetrics calls for testing.
type fakeTelemetryClientForTesting struct {
	telemetrypb.TelemetryClient
	mu      sync.Mutex
	calls   []*telemetrypb.ExportMetricsRequest
	contexts []context.Context
}

func (f *fakeTelemetryClientForTesting) ExportMetrics(ctx context.Context, req *telemetrypb.ExportMetricsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	f.contexts = append(f.contexts, ctx)
	return &emptypb.Empty{}, nil
}

func (f *fakeTelemetryClientForTesting) getCalls() []*telemetrypb.ExportMetricsRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]*telemetrypb.ExportMetricsRequest, len(f.calls))
	copy(result, f.calls)
	return result
}

func (f *fakeTelemetryClientForTesting) getContexts() []context.Context {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]context.Context, len(f.contexts))
	copy(result, f.contexts)
	return result
}

func TestSimAgent_TelemetryLoop(t *testing.T) {
	// Setup
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	
	// Add a node and interface
	node := &model.NetworkNode{
		ID: "node1",
	}
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

	// Start agent
	ctx := context.Background()
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer agent.Stop()

	// Advance time to trigger telemetry
	scheduler.AdvanceTo(startTime.Add(2 * time.Second))

	// Give telemetry a moment to execute
	time.Sleep(100 * time.Millisecond)

	// Verify telemetry was called
	calls := telemetryCli.getCalls()
	if len(calls) == 0 {
		t.Fatalf("expected at least one ExportMetrics call")
	}

	// Verify node_id is in metadata
	contexts := telemetryCli.getContexts()
	if len(contexts) > 0 {
		md, ok := metadata.FromOutgoingContext(contexts[0])
		if ok {
			vals := md.Get("x-node-id")
			if len(vals) == 0 || vals[0] != "node1" {
				t.Fatalf("expected node_id in metadata, got %v", vals)
			}
		}
	}

	// Verify metrics structure
	req := calls[0]
	if len(req.GetInterfaceMetrics()) == 0 {
		t.Fatalf("expected at least one interface metric")
	}

	metrics := req.GetInterfaceMetrics()[0]
	if metrics.GetInterfaceId() != "if1" {
		t.Fatalf("expected interface_id=if1, got %q", metrics.GetInterfaceId())
	}
}

func TestSimAgent_TelemetryLoop_Periodic(t *testing.T) {
	// Setup
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

	ctx := context.Background()
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer agent.Stop()

	// Advance time multiple intervals
	scheduler.AdvanceTo(startTime.Add(1 * time.Second))
	time.Sleep(50 * time.Millisecond)
	
	scheduler.AdvanceTo(startTime.Add(2 * time.Second))
	time.Sleep(50 * time.Millisecond)

	scheduler.AdvanceTo(startTime.Add(3 * time.Second))
	time.Sleep(50 * time.Millisecond)

	// Verify multiple telemetry calls
	calls := telemetryCli.getCalls()
	if len(calls) < 2 {
		t.Fatalf("expected at least 2 telemetry calls, got %d", len(calls))
	}
}

// fakeStreamForTelemetry is a minimal stub stream for telemetry tests.
type fakeStreamForTelemetry struct {
	grpc.BidiStreamingClient[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController]
}

func (f *fakeStreamForTelemetry) Send(msg *schedulingpb.ReceiveRequestsMessageToController) error {
	return nil
}

func (f *fakeStreamForTelemetry) Recv() (*schedulingpb.ReceiveRequestsMessageFromController, error) {
	// Block forever to simulate an active stream
	select {}
}

func TestSimAgent_TelemetryLoop_NoInterfaces(t *testing.T) {
	// Setup agent with node but no interfaces
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	log := logging.Noop()
	scenarioState := state.NewScenarioState(physKB, netKB, log)
	
	node := &model.NetworkNode{ID: "node1"}
	if err := scenarioState.CreateNode(node, nil); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
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

	// Advance time - telemetry should still be scheduled but send empty metrics
	scheduler.AdvanceTo(startTime.Add(2 * time.Second))
	time.Sleep(100 * time.Millisecond)

	// Telemetry loop should still run (reschedule) even with no interfaces
	// The exact behavior depends on implementation, but it shouldn't crash
}

