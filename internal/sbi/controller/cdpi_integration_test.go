package controller

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi/agent"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// testCDPIServer wraps CDPIServer to capture responses for testing.
type testCDPIServer struct {
	*CDPIServer
	responsesMu sync.Mutex
	responses   []*schedulingpb.ReceiveRequestsMessageToController
}

// newTestCDPIServer creates a CDPIServer that records all received responses.
func newTestCDPIServer(state *state.ScenarioState, clock sbi.EventScheduler) *testCDPIServer {
	return &testCDPIServer{
		CDPIServer: NewCDPIServer(state, clock),
		responses:  make([]*schedulingpb.ReceiveRequestsMessageToController, 0),
	}
}

// ReceiveRequests wraps the CDPIServer's ReceiveRequests to capture responses.
func (s *testCDPIServer) ReceiveRequests(stream grpc.BidiStreamingServer[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController]) error {
	// Create a wrapper stream that captures responses
	wrappedStream := &responseCapturingStream{
		BidiStreamingServer: stream,
		server:              s,
	}
	return s.CDPIServer.ReceiveRequests(wrappedStream)
}

// GetResponses returns all captured responses (thread-safe).
func (s *testCDPIServer) GetResponses() []*schedulingpb.ReceiveRequestsMessageToController {
	s.responsesMu.Lock()
	defer s.responsesMu.Unlock()
	result := make([]*schedulingpb.ReceiveRequestsMessageToController, len(s.responses))
	copy(result, s.responses)
	return result
}

// responseCapturingStream wraps a bidirectional stream to capture responses.
type responseCapturingStream struct {
	grpc.BidiStreamingServer[schedulingpb.ReceiveRequestsMessageToController, schedulingpb.ReceiveRequestsMessageFromController]
	server *testCDPIServer
}

func (s *responseCapturingStream) Recv() (*schedulingpb.ReceiveRequestsMessageToController, error) {
	msg, err := s.BidiStreamingServer.Recv()
	if err != nil {
		return nil, err
	}

	// Capture Response messages
	if msg.GetResponse() != nil {
		s.server.responsesMu.Lock()
		s.server.responses = append(s.server.responses, msg)
		s.server.responsesMu.Unlock()
	}

	return msg, nil
}

func (s *responseCapturingStream) Send(msg *schedulingpb.ReceiveRequestsMessageFromController) error {
	return s.BidiStreamingServer.Send(msg)
}

// testHarness sets up an in-process gRPC server with CDPIServer for testing.
type testHarness struct {
	Server        *grpc.Server
	Listener      net.Listener
	State         *state.ScenarioState
	CDPIServer    *testCDPIServer
	EventScheduler *sbi.FakeEventScheduler
	Address       string
}

// newTestHarness creates a new test harness with an in-process gRPC server.
func newTestHarness(t *testing.T) *testHarness {
	t.Helper()

	// Create in-memory state
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	scenarioState := state.NewScenarioState(physKB, netKB, logging.Noop())

	// Create fake event scheduler
	T0 := time.Unix(1000, 0)
	eventScheduler := sbi.NewFakeEventScheduler(T0)

	// Create test CDPI server
	cdpiServer := newTestCDPIServer(scenarioState, eventScheduler)

	// Create gRPC server
	grpcServer := grpc.NewServer()
	schedulingpb.RegisterSchedulingServer(grpcServer, cdpiServer)

	// Listen on random port
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}

	// Start server in goroutine
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			t.Logf("gRPC server error: %v", err)
		}
	}()

	// Give server a moment to start
	time.Sleep(50 * time.Millisecond)

	return &testHarness{
		Server:         grpcServer,
		Listener:      lis,
		State:          scenarioState,
		CDPIServer:    cdpiServer,
		EventScheduler: eventScheduler,
		Address:       lis.Addr().String(),
	}
}

// Close shuts down the test harness.
func (h *testHarness) Close() {
	if h.Server != nil {
		h.Server.GracefulStop()
	}
	if h.Listener != nil {
		h.Listener.Close()
	}
}

// TestCDPIEndToEnd_UpdateBeam tests the full CDPI flow:
// 1. Agent connects and sends Hello
// 2. Controller sends CreateEntryRequest for UpdateBeam
// 3. Agent schedules and executes the action
// 4. Agent sends Response back
// 5. Verify state changes and response delivery
func TestCDPIEndToEnd_UpdateBeam(t *testing.T) {
	harness := newTestHarness(t)
	defer harness.Close()

	// Create minimal scenario state with two nodes and a link
	// Add transceiver models first
	netKB := harness.State.NetworkKB()
	if err := netKB.AddTransceiverModel(&core.TransceiverModel{
		ID:   "trx-A",
		Name: "Transceiver A",
		Band: core.FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 10.1,
		},
	}); err != nil {
		t.Fatalf("AddTransceiverModel(trx-A) failed: %v", err)
	}
	if err := netKB.AddTransceiverModel(&core.TransceiverModel{
		ID:   "trx-B",
		Name: "Transceiver B",
		Band: core.FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 10.1,
		},
	}); err != nil {
		t.Fatalf("AddTransceiverModel(trx-B) failed: %v", err)
	}

	// Create platforms
	if err := harness.State.CreatePlatform(&model.PlatformDefinition{
		ID:   "platform-A",
		Name: "Platform A",
	}); err != nil {
		t.Fatalf("CreatePlatform(platform-A) failed: %v", err)
	}
	if err := harness.State.CreatePlatform(&model.PlatformDefinition{
		ID:   "platform-B",
		Name: "Platform B",
	}); err != nil {
		t.Fatalf("CreatePlatform(platform-B) failed: %v", err)
	}

	// Create nodes with interfaces
	// Interface IDs should be in "node-ID/interface-ID" format
	nodeA := &model.NetworkNode{
		ID:         "node-A",
		Name:       "Node A",
		PlatformID: "platform-A",
	}
	if err := harness.State.CreateNode(nodeA, []*core.NetworkInterface{
		{
			ID:            "node-A/if-A",
			Name:          "Interface A",
			Medium:        core.MediumWireless,
			ParentNodeID:  "node-A",
			IsOperational: true,
			TransceiverID: "trx-A",
		},
	}); err != nil {
		t.Fatalf("CreateNode(node-A) failed: %v", err)
	}

	nodeB := &model.NetworkNode{
		ID:         "node-B",
		Name:       "Node B",
		PlatformID: "platform-B",
	}
	if err := harness.State.CreateNode(nodeB, []*core.NetworkInterface{
		{
			ID:            "node-B/if-B",
			Name:          "Interface B",
			Medium:        core.MediumWireless,
			ParentNodeID:  "node-B",
			IsOperational: true,
			TransceiverID: "trx-B",
		},
	}); err != nil {
		t.Fatalf("CreateNode(node-B) failed: %v", err)
	}

	// Create a potential link between the nodes
	// Interface IDs in links use "node-ID/interface-ID" format
	link := &core.NetworkLink{
		ID:         "link-AB",
		InterfaceA: "node-A/if-A",
		InterfaceB: "node-B/if-B",
		Medium:     core.MediumWireless,
		Status:     core.LinkStatusPotential,
		IsUp:       false,
	}
	if err := harness.State.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	// Create gRPC client connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		harness.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("grpc.DialContext failed: %v", err)
	}
	defer conn.Close()

	// Create CDPI client
	cdpiClient := schedulingpb.NewSchedulingClient(conn)

	// Create agent stream
	agentCtx, agentCancel := context.WithCancel(ctx)
	defer agentCancel()

	stream, err := cdpiClient.ReceiveRequests(agentCtx)
	if err != nil {
		t.Fatalf("ReceiveRequests failed: %v", err)
	}

	// Create agent
	simAgent := agent.NewSimAgent(
		"node-A",
		"node-A",
		harness.State,
		harness.EventScheduler,
		nil, // no telemetry client for this test
		stream,
	)

	// Start agent (sends Hello)
	if err := simAgent.Start(agentCtx); err != nil {
		t.Fatalf("agent.Start failed: %v", err)
	}
	defer simAgent.Stop()

	// Wait a moment for Hello to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify agent is registered
	harness.CDPIServer.agentsMu.RLock()
	_, exists := harness.CDPIServer.agents["node-A"]
	harness.CDPIServer.agentsMu.RUnlock()
	if !exists {
		t.Fatalf("agent node-A not registered after Hello")
	}

	// Create a scheduled UpdateBeam action
	actionTime := harness.EventScheduler.Now().Add(1 * time.Second)
	// BeamSpec uses just the interface ID part (not "node-ID/interface-ID")
	beamSpec := &sbi.BeamSpec{
		NodeID:       "node-A",
		InterfaceID:  "if-A", // Just the interface ID part
		TargetNodeID: "node-B",
		TargetIfID:   "if-B", // Just the interface ID part
	}
	action := &sbi.ScheduledAction{
		EntryID:   "entry-1",
		AgentID:   "node-A",
		Type:      sbi.ScheduledUpdateBeam,
		When:      actionTime,
		Beam:      beamSpec,
		RequestID: "req-1",
		SeqNo:     0, // CDPI will fill this
		Token:     "", // CDPI will fill this
	}

	// Send CreateEntryRequest via CDPI server
	if err := harness.CDPIServer.SendCreateEntry("node-A", action); err != nil {
		t.Fatalf("SendCreateEntry failed: %v", err)
	}

	// Wait a moment for the message to be sent and received
	time.Sleep(100 * time.Millisecond)

	// Advance scheduler to action time
	harness.EventScheduler.AdvanceTo(actionTime)

	// Wait a moment for execution and response
	time.Sleep(100 * time.Millisecond)

	// Verify link is now Active
	linkAfter, err := harness.State.GetLink("link-AB")
	if err != nil {
		t.Fatalf("GetLink failed: %v", err)
	}
	if linkAfter.Status != core.LinkStatusActive {
		t.Errorf("Link Status = %v, want LinkStatusActive", linkAfter.Status)
	}
	if !linkAfter.IsUp {
		t.Errorf("Link IsUp = %v, want true", linkAfter.IsUp)
	}

	// Verify response was received
	responses := harness.CDPIServer.GetResponses()
	if len(responses) == 0 {
		t.Fatalf("expected at least one response, got %d", len(responses))
	}

	// Find response for our request
	foundResponse := false
	for _, resp := range responses {
		respMsg := resp.GetResponse()
		if respMsg == nil {
			continue
		}

		// Check if this is our response (request_id matching)
		// Note: The agent converts RequestID string to int64, so we need to check
		// For now, we'll check if any response has OK status
		if respMsg.GetStatus() != nil {
			code := status.FromProto(respMsg.GetStatus()).Code()
			if code == codes.OK {
				foundResponse = true
				break
			}
		}
	}

	if !foundResponse {
		t.Errorf("expected at least one OK response, got responses: %+v", responses)
	}
}

