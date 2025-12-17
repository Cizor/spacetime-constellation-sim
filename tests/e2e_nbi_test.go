package tests

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	common "aalyria.com/spacetime/api/common"
	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/nbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi/controller"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
	"github.com/signalsfoundry/constellation-simulator/timectrl"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type nbiTestEnv struct {
	ctx                     context.Context
	cancel                  context.CancelFunc
	state                   *sim.ScenarioState
	motion                  *deterministicMotion
	connectivity            *core.ConnectivityService
	physKB                  *kb.KnowledgeBase
	netKB                   *core.KnowledgeBase
	grpcServer              *grpc.Server
	serveErr                <-chan error
	transceiverID           string
	shortRangeTransceiverID string
	platformClient          v1alpha.PlatformServiceClient
	nodeClient              v1alpha.NetworkNodeServiceClient
	linkClient              v1alpha.NetworkLinkServiceClient
	srClient                v1alpha.ServiceRequestServiceClient
	scenarioClient          v1alpha.ScenarioServiceClient
	eventClock              *timectrl.TimeController
	eventScheduler          sbi.EventScheduler
	cdpi                    *controller.TestCDPI
	scheduler               *controller.Scheduler
}

func newNbiTestEnv(t *testing.T) *nbiTestEnv {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	trxID := "trx-e2e-ku"
	if err := netKB.AddTransceiverModel(&core.TransceiverModel{
		ID:         trxID,
		Band:       core.FrequencyBand{MinGHz: 10.7, MaxGHz: 12.75},
		MaxRangeKm: 120000,
	}); err != nil {
		cancel()
		t.Fatalf("AddTransceiverModel: %v", err)
	}
	shortTrxID := "trx-e2e-short"
	if err := netKB.AddTransceiverModel(&core.TransceiverModel{
		ID:         shortTrxID,
		Band:       core.FrequencyBand{MinGHz: 10.7, MaxGHz: 12.75},
		MaxRangeKm: 1.5,
	}); err != nil {
		cancel()
		t.Fatalf("AddTransceiverModel short: %v", err)
	}

	motion := newDeterministicMotion(physKB)
	connectivity := core.NewConnectivityService(netKB)
	connectivity.MinElevationDeg = 0

	state := sim.NewScenarioState(
		physKB,
		netKB,
		logging.Noop(),
		sim.WithMotionModel(motion),
		sim.WithConnectivityService(connectivity),
	)

	eventClock := timectrl.NewTimeController(time.Now().UTC(), 20*time.Millisecond, timectrl.Accelerated)
	eventScheduler := sbi.NewEventScheduler(eventClock)
	fakeCDPI := controller.NewTestCDPI()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	v1alpha.RegisterPlatformServiceServer(
		grpcServer,
		nbi.NewPlatformService(state, motion, logging.Noop()),
	)
	v1alpha.RegisterNetworkNodeServiceServer(
		grpcServer,
		nbi.NewNetworkNodeService(state, logging.Noop()),
	)
	v1alpha.RegisterNetworkLinkServiceServer(
		grpcServer,
		nbi.NewNetworkLinkService(state, logging.Noop()),
	)
	v1alpha.RegisterServiceRequestServiceServer(
		grpcServer,
		nbi.NewServiceRequestService(state, logging.Noop()),
	)
	v1alpha.RegisterScenarioServiceServer(
		grpcServer,
		nbi.NewScenarioService(state, logging.Noop()),
	)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- grpcServer.Serve(lis)
	}()

	conn, err := grpc.DialContext(ctx, lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		cancel()
		t.Fatalf("grpc.DialContext: %v", err)
	}

	env := &nbiTestEnv{
		ctx:                     ctx,
		cancel:                  cancel,
		state:                   state,
		motion:                  motion,
		connectivity:            connectivity,
		physKB:                  physKB,
		netKB:                   netKB,
		grpcServer:              grpcServer,
		serveErr:                serveErr,
		transceiverID:           trxID,
		shortRangeTransceiverID: shortTrxID,
		platformClient:          v1alpha.NewPlatformServiceClient(conn),
		nodeClient:              v1alpha.NewNetworkNodeServiceClient(conn),
		linkClient:              v1alpha.NewNetworkLinkServiceClient(conn),
		srClient:                v1alpha.NewServiceRequestServiceClient(conn),
		scenarioClient:          v1alpha.NewScenarioServiceClient(conn),
		eventClock:              eventClock,
		eventScheduler:          eventScheduler,
		cdpi:                    fakeCDPI,
	}

	t.Cleanup(func() {
		grpcServer.GracefulStop()
		_ = conn.Close()
		cancel()
	})

	return env
}

func TestEndToEndNBI(t *testing.T) {
	env := newNbiTestEnv(t)
	ctx := env.ctx
	physKB := env.physKB
	netKB := env.netKB
	motion := env.motion
	connectivity := env.connectivity
	state := env.state
	serveErr := env.serveErr

	platformClient := env.platformClient
	nodeClient := env.nodeClient
	linkClient := env.linkClient
	srClient := env.srClient
	scenarioClient := env.scenarioClient

	groundID := "platform-ground"
	satID := "platform-sat"
	groundPos := model.Motion{X: (core.EarthRadiusKm + 1) * 1000, Y: 0, Z: 0}
	satPath := []model.Motion{
		{X: -(core.EarthRadiusKm + 500) * 1000, Y: 0, Z: 0},
		{X: (core.EarthRadiusKm + 500) * 1000, Y: 0, Z: 0},
	}
	motion.setPath(groundID, []model.Motion{groundPos})
	motion.setPath(satID, satPath)

	if _, err := platformClient.CreatePlatform(ctx, platformProto(groundID, "GROUND_STATION", groundPos, common.PlatformDefinition_UNKNOWN_SOURCE)); err != nil {
		t.Fatalf("CreatePlatform ground: %v", err)
	}
	if _, err := platformClient.CreatePlatform(ctx, platformProto(satID, "SATELLITE", satPath[0], common.PlatformDefinition_SPACETRACK_ORG)); err != nil {
		t.Fatalf("CreatePlatform sat: %v", err)
	}

	groundNode := "node-ground"
	satNode := "node-sat"
	groundIface := "if-ground"
	satIface := "if-sat"

	if _, err := nodeClient.CreateNode(ctx, wirelessNodeProto(groundNode, groundID, groundIface, env.transceiverID)); err != nil {
		t.Fatalf("CreateNode ground: %v", err)
	}
	if _, err := nodeClient.CreateNode(ctx, wirelessNodeProto(satNode, satID, satIface, env.transceiverID)); err != nil {
		t.Fatalf("CreateNode sat: %v", err)
	}

	if _, err := linkClient.CreateLink(ctx, bidirectionalLinkProto(groundNode, groundIface, satNode, satIface)); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	if _, err := srClient.CreateServiceRequest(ctx, serviceRequestProto("sr-ground-sat", groundNode, satNode)); err != nil {
		t.Fatalf("CreateServiceRequest: %v", err)
	}

	start := time.Now().UTC()
	if err := motion.UpdatePositions(start); err != nil {
		t.Fatalf("initial UpdatePositions: %v", err)
	}
	pushNodePositions(physKB, netKB)
	connectivity.UpdateConnectivity()

	initialUp := linkIsUp(t, netKB, combineInterfaceRef(groundNode, groundIface), combineInterfaceRef(satNode, satIface))

	tick := 20 * time.Millisecond
	tc := timectrl.NewTimeController(start, tick, timectrl.Accelerated)

	tickErr := make(chan error, 1)
	tc.AddListener(func(simTime time.Time) {
		if err := state.RunSimTick(simTime, motion, nil, nil); err != nil {
			select {
			case tickErr <- err:
			default:
			}
		}
		pushNodePositions(physKB, netKB)
		connectivity.UpdateConnectivity()
	})

	done := tc.Start(2 * tick)
	select {
	case <-done:
	case err := <-tickErr:
		t.Fatalf("RunSimTick: %v", err)
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("grpc Serve: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("context deadline exceeded before sim ticks")
	}

	finalUp := linkIsUp(t, netKB, combineInterfaceRef(groundNode, groundIface), combineInterfaceRef(satNode, satIface))
	if initialUp {
		t.Fatalf("link should start down but was up")
	}
	if !finalUp {
		t.Fatalf("link did not come up after motion")
	}

	snapshot, err := scenarioClient.GetScenario(ctx, &v1alpha.GetScenarioRequest{})
	if err != nil {
		t.Fatalf("GetScenario: %v", err)
	}

	if got := len(snapshot.GetPlatforms()); got != 2 {
		t.Fatalf("platform count = %d, want 2", got)
	}
	if got := len(snapshot.GetNodes()); got != 2 {
		t.Fatalf("node count = %d, want 2", got)
	}
	if got := len(snapshot.GetLinks()); got != 1 {
		t.Fatalf("link count = %d, want 1", got)
	}
	if got := len(snapshot.GetServiceRequests()); got != 1 {
		t.Fatalf("service request count = %d, want 1", got)
	}
}

func TestEndToEndPriorityPreemptionCascade(t *testing.T) {
	env := newNbiTestEnv(t)
	ctx := env.ctx

	nodes := setupLinkedNodes(t, env, []string{"node-pre-A", "node-pre-B"})
	sched := env.ensureScheduler(t)
	start := env.eventScheduler.Now()
	windows := make(map[string][]controller.ContactWindow)
	for _, linkID := range linkIDsForNodePair(t, env.state, nodes[0], nodes[1]) {
		windows[linkID] = []controller.ContactWindow{
			{
				LinkID:    linkID,
				StartTime: start,
				EndTime:   start.Add(15 * time.Minute),
			},
		}
	}
	if len(windows) == 0 {
		t.Fatalf("no contact windows found for %s <-> %s", nodes[0], nodes[1])
	}
	sched.SetContactWindows(windows)

	lowReq := serviceRequestProtoWithPriority("sr-low", nodes[0], nodes[1], 1)
	if _, err := env.srClient.CreateServiceRequest(ctx, lowReq); err != nil {
		t.Fatalf("CreateServiceRequest low priority: %v", err)
	}
	simTime := runSimulationTicks(t, env, 6)
	limitLinkBandwidth(t, env, nodes[0], fmt.Sprintf("if-%s", nodes[0]), nodes[1], fmt.Sprintf("if-%s", nodes[1]), 1_000_000)
	runScheduler(t, env, simTime)

	statusLow := getServiceRequestStatus(t, env, lowReq.GetType())
	if !statusLow.IsProvisionedNow {
		t.Fatalf("low-priority service request should initially be provisioned")
	}

	highReq := serviceRequestProtoWithPriority("sr-high", nodes[0], nodes[1], 10)
	if _, err := env.srClient.CreateServiceRequest(ctx, highReq); err != nil {
		t.Fatalf("CreateServiceRequest high priority: %v", err)
	}
	simTime = simTime.Add(1 * time.Second)
	sched.SetContactWindows(windows)
	runScheduler(t, env, simTime)

	statusHigh := getServiceRequestStatus(t, env, highReq.GetType())
	if !statusHigh.IsProvisionedNow {
		t.Fatalf("high-priority service request should be provisioned (status=%+v)", statusHigh)
	}

	statusLow = getServiceRequestStatus(t, env, lowReq.GetType())
	if statusLow.IsProvisionedNow {
		t.Fatalf("low-priority service request should have been preempted")
	}
}

func TestEndToEndMultiHopServiceRequest(t *testing.T) {
	env := newNbiTestEnv(t)

	nodes := setupLinkedNodesWithTransceiver(t, env, env.shortRangeTransceiverID, []string{
		"node-hop-A",
		"node-hop-B",
		"node-hop-C",
	})

	sched := env.ensureScheduler(t)
	start := env.eventScheduler.Now()
	pairs := [][2]string{
		{nodes[0], nodes[1]},
		{nodes[1], nodes[2]},
	}
	linkIDsByPair := make([][]string, len(pairs))
	for i, pair := range pairs {
		linkIDsByPair[i] = linkIDsForNodePair(t, env.state, pair[0], pair[1])
		if len(linkIDsByPair[i]) == 0 {
			t.Fatalf("expected contact link between %s and %s", pair[0], pair[1])
		}
	}
	windows := make(map[string][]controller.ContactWindow)
	windowDuration := 2 * time.Minute
	spacing := 3 * time.Minute
	for idx, ids := range linkIDsByPair {
		sort.Strings(ids)
		windowStart := start.Add(time.Duration(idx) * spacing)
		windowEnd := windowStart.Add(windowDuration)
		for _, linkID := range ids {
			windows[linkID] = []controller.ContactWindow{
				{
					LinkID:    linkID,
					StartTime: windowStart,
					EndTime:   windowEnd,
				},
			}
		}
	}
	sched.SetContactWindows(windows)

	path, err := sched.FindMultiHopPath(context.Background(), nodes[0], nodes[len(nodes)-1], start, controller.ContactHorizon)
	if err != nil {
		t.Fatalf("FindMultiHopPath failed: %v", err)
	}
	pathNodes := make([]string, 0, len(path.Hops)+1)
	if len(path.Hops) > 0 {
		pathNodes = append(pathNodes, path.Hops[0].FromNodeID)
		for _, hop := range path.Hops {
			pathNodes = append(pathNodes, hop.ToNodeID)
		}
	}
	if len(pathNodes) < len(nodes) {
		t.Fatalf("expected multi-hop path through %d nodes, got %v", len(nodes), pathNodes)
	}
	if pathNodes[0] != nodes[0] || pathNodes[len(pathNodes)-1] != nodes[len(nodes)-1] {
		t.Fatalf("multi-hop path endpoints wrong: %v", pathNodes)
	}
}

func TestEndToEndDTNStoreForward(t *testing.T) {
	env := newNbiTestEnv(t)
	ctx := env.ctx

	nodes := []string{"node-dtn-A", "node-dtn-B", "node-dtn-C"}
	ifaces := map[string]string{}
	for _, node := range nodes {
		ifaces[node] = createNodeWithStorage(t, env, node, env.transceiverID, 10_000_000)
	}

	linkPairs := [][2]string{
		{nodes[0], nodes[1]},
		{nodes[1], nodes[2]},
	}
	for _, pair := range linkPairs {
		aIface := ifaces[pair[0]]
		bIface := ifaces[pair[1]]
		if _, err := env.linkClient.CreateLink(ctx, bidirectionalLinkProto(pair[0], aIface, pair[1], bIface)); err != nil {
			t.Fatalf("CreateLink %s-%s: %v", pair[0], pair[1], err)
		}
		limitLinkBandwidth(t, env, pair[0], aIface, pair[1], bIface, 1_500_000)
	}

	sched := env.ensureScheduler(t)
	start := env.eventScheduler.Now()
	linkIDsAB := linkIDsForNodePair(t, env.state, nodes[0], nodes[1])
	linkIDsBC := linkIDsForNodePair(t, env.state, nodes[1], nodes[2])
	if len(linkIDsAB) == 0 {
		t.Fatalf("missing link between %s and %s", nodes[0], nodes[1])
	}
	if len(linkIDsBC) == 0 {
		t.Fatalf("missing link between %s and %s", nodes[1], nodes[2])
	}
	windows := make(map[string][]controller.ContactWindow)
	for _, linkID := range linkIDsAB {
		windows[linkID] = []controller.ContactWindow{
			{
				LinkID:    linkID,
				StartTime: start.Add(0 * time.Second),
				EndTime:   start.Add(30 * time.Second),
			},
		}
	}
	for _, linkID := range linkIDsBC {
		windows[linkID] = []controller.ContactWindow{
			{
				LinkID:    linkID,
				StartTime: start.Add(60 * time.Second),
				EndTime:   start.Add(90 * time.Second),
			},
		}
	}
	sched.SetContactWindows(windows)
	if _, err := sched.FindDTNPath(ctx, nodes[0], nodes[2], 1_000_000, start); err != nil {
		t.Fatalf("FindDTNPath: %v", err)
	}

	sr := serviceRequestProtoWithPriority("sr-dtn", nodes[0], nodes[2], 2)
	if len(sr.Requirements) == 0 {
		t.Fatalf("service request requirements missing")
	}
	sr.Requirements[0].IsDisruptionTolerant = boolPtr(true)
	if _, err := env.srClient.CreateServiceRequest(ctx, sr); err != nil {
		t.Fatalf("CreateServiceRequest DTN: %v", err)
	}

	simTime := runSimulationTicks(t, env, 40)
	for _, pair := range linkPairs {
		limitLinkBandwidth(t, env, pair[0], ifaces[pair[0]], pair[1], ifaces[pair[1]], 1_500_000)
	}
	runScheduler(t, env, simTime)

	decisions, _ := sched.SnapshotDecisions()
	found := false
	for _, decision := range decisions {
		if decision.ServiceRequestID == sr.GetType() {
			found = true
			if !decision.DetnCandidate {
				t.Fatalf("expected DTN candidate for service request %s", sr.GetType())
			}
			break
		}
	}
	if !found {
		t.Fatalf("service request %s missing from decisions (%d decisions)", sr.GetType(), len(decisions))
	}
}

func TestDeletePlatformWithActiveNodeE2E(t *testing.T) {
	env := newNbiTestEnv(t)
	ctx := env.ctx

	platformID := "platform-in-use"
	pos := model.Motion{X: (core.EarthRadiusKm + 2) * 1000, Y: 0, Z: 0}
	env.motion.setPath(platformID, []model.Motion{pos})

	if _, err := env.platformClient.CreatePlatform(ctx, platformProto(platformID, "GROUND_STATION", pos, common.PlatformDefinition_UNKNOWN_SOURCE)); err != nil {
		t.Fatalf("CreatePlatform: %v", err)
	}

	nodeID := "node-on-platform"
	ifaceID := "if-on-platform"
	if _, err := env.nodeClient.CreateNode(ctx, wirelessNodeProto(nodeID, platformID, ifaceID, env.transceiverID)); err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	_, err := env.platformClient.DeletePlatform(ctx, &v1alpha.DeletePlatformRequest{PlatformId: &platformID})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("DeletePlatform code = %v, want FailedPrecondition (err=%v)", status.Code(err), err)
	}
	if err == nil || !strings.Contains(err.Error(), "referenced by nodes") {
		t.Fatalf("DeletePlatform error message = %v, want reference hint", err)
	}

	snapshot, snapErr := env.scenarioClient.GetScenario(ctx, &v1alpha.GetScenarioRequest{})
	if snapErr != nil {
		t.Fatalf("GetScenario: %v", snapErr)
	}
	if got := len(snapshot.GetPlatforms()); got != 1 {
		t.Fatalf("platform count = %d, want 1 (platform should remain)", got)
	}
	if got := len(snapshot.GetNodes()); got != 1 {
		t.Fatalf("node count = %d, want 1 (node should remain)", got)
	}
}

func TestDeleteNodeWithLinkedInterfacesE2E(t *testing.T) {
	env := newNbiTestEnv(t)
	ctx := env.ctx

	platformA := "platform-a"
	platformB := "platform-b"
	env.motion.setPath(platformA, []model.Motion{{X: (core.EarthRadiusKm + 3) * 1000}})
	env.motion.setPath(platformB, []model.Motion{{X: (core.EarthRadiusKm + 4) * 1000}})

	if _, err := env.platformClient.CreatePlatform(ctx, platformProto(platformA, "GROUND_STATION", model.Motion{X: (core.EarthRadiusKm + 3) * 1000}, common.PlatformDefinition_UNKNOWN_SOURCE)); err != nil {
		t.Fatalf("CreatePlatform A: %v", err)
	}
	if _, err := env.platformClient.CreatePlatform(ctx, platformProto(platformB, "GROUND_STATION", model.Motion{X: (core.EarthRadiusKm + 4) * 1000}, common.PlatformDefinition_UNKNOWN_SOURCE)); err != nil {
		t.Fatalf("CreatePlatform B: %v", err)
	}

	nodeA := "node-a"
	nodeB := "node-b"
	ifaceA := "if-a"
	ifaceB := "if-b"

	if _, err := env.nodeClient.CreateNode(ctx, wirelessNodeProto(nodeA, platformA, ifaceA, env.transceiverID)); err != nil {
		t.Fatalf("CreateNode A: %v", err)
	}
	if _, err := env.nodeClient.CreateNode(ctx, wirelessNodeProto(nodeB, platformB, ifaceB, env.transceiverID)); err != nil {
		t.Fatalf("CreateNode B: %v", err)
	}

	if _, err := env.linkClient.CreateLink(ctx, bidirectionalLinkProto(nodeA, ifaceA, nodeB, ifaceB)); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	_, err := env.nodeClient.DeleteNode(ctx, &v1alpha.DeleteNodeRequest{NodeId: &nodeA})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("DeleteNode code = %v, want FailedPrecondition (err=%v)", status.Code(err), err)
	}
	if err == nil || !strings.Contains(err.Error(), "node is referenced") {
		t.Fatalf("DeleteNode error message = %v, want reference hint", err)
	}

	if got := env.state.PhysicalKB().GetNetworkNode(nodeA); got == nil {
		t.Fatalf("node %q should still exist after failed delete", nodeA)
	}
	if ifaces := env.netKB.GetInterfacesForNode(nodeA); len(ifaces) != 1 {
		t.Fatalf("interface count for %s = %d, want 1", nodeA, len(ifaces))
	}
	if links := env.netKB.GetLinksForInterface(combineInterfaceRef(nodeA, ifaceA)); len(links) != 1 {
		t.Fatalf("link count for %s/%s = %d, want 1", nodeA, ifaceA, len(links))
	}
}

func TestCreateServiceRequestWithUnknownNodeE2E(t *testing.T) {
	env := newNbiTestEnv(t)
	ctx := env.ctx

	_, err := env.srClient.CreateServiceRequest(ctx, serviceRequestProto("sr-unknown-node", "missing-src", "missing-dst"))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CreateServiceRequest code = %v, want InvalidArgument (err=%v)", status.Code(err), err)
	}
	if err == nil || (!strings.Contains(err.Error(), "unknown src node") && !strings.Contains(err.Error(), "unknown dst node")) {
		t.Fatalf("CreateServiceRequest error message = %v, want unknown node hint", err)
	}

	snapshot, snapErr := env.scenarioClient.GetScenario(ctx, &v1alpha.GetScenarioRequest{})
	if snapErr != nil {
		t.Fatalf("GetScenario: %v", snapErr)
	}
	if got := len(snapshot.GetServiceRequests()); got != 0 {
		t.Fatalf("service request count = %d, want 0 after failed create", got)
	}
}

func platformProto(name, typ string, pos model.Motion, motionSource common.PlatformDefinition_MotionSource) *common.PlatformDefinition {
	x := pos.X
	y := pos.Y
	z := pos.Z

	return &common.PlatformDefinition{
		Name:         stringPtr(name),
		Type:         stringPtr(typ),
		MotionSource: &motionSource,
		Coordinates: &common.Motion{
			Type: &common.Motion_EcefFixed{
				EcefFixed: &common.PointAxes{
					Point: &common.Cartesian{XM: &x, YM: &y, ZM: &z},
				},
			},
		},
	}
}

func wirelessNodeProto(nodeID, platformID, ifaceID, trxID string) *resources.NetworkNode {
	typ := "ROUTER"

	return &resources.NetworkNode{
		NodeId: stringPtr(nodeID),
		Type:   &typ,
		NodeInterface: []*resources.NetworkInterface{
			{
				InterfaceId: stringPtr(ifaceID),
				InterfaceMedium: &resources.NetworkInterface_Wireless{
					Wireless: &resources.WirelessDevice{
						Platform: stringPtr(platformID),
						TransceiverModelId: &common.TransceiverModelId{
							TransceiverModelId: stringPtr(trxID),
						},
					},
				},
			},
		},
	}
}

func bidirectionalLinkProto(aNode, aIface, bNode, bIface string) *resources.BidirectionalLink {
	return &resources.BidirectionalLink{
		ANetworkNodeId: stringPtr(aNode),
		ATxInterfaceId: stringPtr(aIface),
		ARxInterfaceId: stringPtr(aIface),
		BNetworkNodeId: stringPtr(bNode),
		BTxInterfaceId: stringPtr(bIface),
		BRxInterfaceId: stringPtr(bIface),
	}
}

func pushNodePositions(phys *kb.KnowledgeBase, netKB *core.KnowledgeBase) {
	platforms := phys.ListPlatforms()
	platformByID := make(map[string]*model.PlatformDefinition, len(platforms))
	for _, p := range platforms {
		if p == nil {
			continue
		}
		platformByID[p.ID] = p
	}

	for _, node := range phys.ListNetworkNodes() {
		if node == nil {
			continue
		}
		if p := platformByID[node.PlatformID]; p != nil {
			netKB.SetNodeECEFPosition(node.ID, core.Vec3{
				X: p.Coordinates.X / 1000.0,
				Y: p.Coordinates.Y / 1000.0,
				Z: p.Coordinates.Z / 1000.0,
			})
		}
	}
}

func linkIsUp(t *testing.T, netKB *core.KnowledgeBase, a, b string) bool {
	t.Helper()

	for _, link := range netKB.GetAllNetworkLinks() {
		if link == nil {
			continue
		}
		if (link.InterfaceA == a && link.InterfaceB == b) || (link.InterfaceA == b && link.InterfaceB == a) {
			return link.IsUp
		}
	}

	t.Fatalf("link %s <-> %s not found", a, b)
	return false
}

func combineInterfaceRef(nodeID, ifaceID string) string {
	switch {
	case nodeID == "" && ifaceID == "":
		return ""
	case nodeID == "":
		return ifaceID
	case ifaceID == "":
		return nodeID
	default:
		return fmt.Sprintf("%s/%s", nodeID, ifaceID)
	}
}

func linkIDsForNodePair(t *testing.T, state *sim.ScenarioState, nodeA, nodeB string) []string {
	t.Helper()

	if state == nil {
		t.Fatalf("scenario state is nil when looking up link IDs for %s-%s", nodeA, nodeB)
	}

	aRef := combineInterfaceRef(nodeA, fmt.Sprintf("if-%s", nodeA))
	bRef := combineInterfaceRef(nodeB, fmt.Sprintf("if-%s", nodeB))
	ids := make([]string, 0)
	for _, link := range state.ListLinks() {
		if link == nil {
			continue
		}
		if (link.InterfaceA == aRef && link.InterfaceB == bRef) || (link.InterfaceA == bRef && link.InterfaceB == aRef) {
			ids = append(ids, link.ID)
		}
	}

	sort.Strings(ids)
	return ids
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func boolPtr(v bool) *bool {
	return &v
}

func wirelessNodeProtoWithStorage(nodeID, platformID, ifaceID, trxID string, storageBytes int64) *resources.NetworkNode {
	node := wirelessNodeProto(nodeID, platformID, ifaceID, trxID)
	if storageBytes > 0 {
		node.Storage = &resources.NetworkNode_Storage{
			AvailableBytes: &storageBytes,
		}
	}
	return node
}

func createNodeWithStorage(t *testing.T, env *nbiTestEnv, nodeID, trxID string, storageBytes int64) string {
	t.Helper()

	platformID := fmt.Sprintf("platform-%s", nodeID)
	pos := model.Motion{
		X: (core.EarthRadiusKm + 2) * 1000,
		Y: 0,
		Z: 0,
	}
	env.motion.setPath(platformID, []model.Motion{pos})

	if _, err := env.platformClient.CreatePlatform(env.ctx, platformProto(platformID, "GROUND_STATION", pos, common.PlatformDefinition_UNKNOWN_SOURCE)); err != nil {
		t.Fatalf("CreatePlatform %s: %v", platformID, err)
	}

	iface := fmt.Sprintf("if-%s", nodeID)
	if _, err := env.nodeClient.CreateNode(env.ctx, wirelessNodeProtoWithStorage(nodeID, platformID, iface, trxID, storageBytes)); err != nil {
		t.Fatalf("CreateNode %s: %v", nodeID, err)
	}
	env.registerAgent(nodeID)

	return iface
}

func newServiceRequestProto(id, srcNode, dstNode string, priority float64) *resources.ServiceRequest {
	bw := float64(1_000_000)
	min := float64(500_000)

	return &resources.ServiceRequest{
		Type:     stringPtr(id),
		SrcType:  &resources.ServiceRequest_SrcNodeId{SrcNodeId: srcNode},
		DstType:  &resources.ServiceRequest_DstNodeId{DstNodeId: dstNode},
		Priority: &priority,
		Requirements: []*resources.ServiceRequest_FlowRequirements{
			{
				BandwidthBpsRequested: &bw,
				BandwidthBpsMinimum:   &min,
			},
		},
	}
}

func serviceRequestProto(id, srcNode, dstNode string) *resources.ServiceRequest {
	return newServiceRequestProto(id, srcNode, dstNode, 1)
}

func serviceRequestProtoWithPriority(id, srcNode, dstNode string, priority float64) *resources.ServiceRequest {
	return newServiceRequestProto(id, srcNode, dstNode, priority)
}

func createLinkedNodesForTransceiver(t *testing.T, env *nbiTestEnv, trxID string, nodeIDs []string) []string {
	t.Helper()

	if len(nodeIDs) == 0 {
		return nil
	}

	var created []string
	for idx, node := range nodeIDs {
		platformID := fmt.Sprintf("platform-%s", node)
		pos := model.Motion{
			X: (core.EarthRadiusKm + float64(2+idx)) * 1000,
			Y: float64(idx) * 1000,
			Z: 0,
		}
		env.motion.setPath(platformID, []model.Motion{pos})
		if _, err := env.platformClient.CreatePlatform(env.ctx, platformProto(platformID, "GROUND_STATION", pos, common.PlatformDefinition_UNKNOWN_SOURCE)); err != nil {
			t.Fatalf("CreatePlatform %s: %v", platformID, err)
		}

		iface := fmt.Sprintf("if-%s", node)
		if _, err := env.nodeClient.CreateNode(env.ctx, wirelessNodeProto(node, platformID, iface, trxID)); err != nil {
			t.Fatalf("CreateNode %s: %v", node, err)
		}

		env.registerAgent(node)
		created = append(created, node)
	}

	for i := 0; i+1 < len(created); i++ {
		a := created[i]
		b := created[i+1]
		aIface := fmt.Sprintf("if-%s", a)
		bIface := fmt.Sprintf("if-%s", b)
		if _, err := env.linkClient.CreateLink(env.ctx, bidirectionalLinkProto(a, aIface, b, bIface)); err != nil {
			t.Fatalf("CreateLink %s-%s: %v", a, b, err)
		}
		limitLinkBandwidth(t, env, a, aIface, b, bIface, 1_000_000)
	}

	pushNodePositions(env.physKB, env.netKB)
	env.connectivity.UpdateConnectivity()

	return created
}

func setupLinkedNodes(t *testing.T, env *nbiTestEnv, nodeIDs []string) []string {
	return createLinkedNodesForTransceiver(t, env, env.transceiverID, nodeIDs)
}

func setupLinkedNodesWithTransceiver(t *testing.T, env *nbiTestEnv, trxID string, nodeIDs []string) []string {
	return createLinkedNodesForTransceiver(t, env, trxID, nodeIDs)
}

func limitLinkBandwidth(t *testing.T, env *nbiTestEnv, aNode, aIface, bNode, bIface string, capacity uint64) {
	t.Helper()

	aRef := combineInterfaceRef(aNode, aIface)
	bRef := combineInterfaceRef(bNode, bIface)
	if aRef == "" || bRef == "" {
		t.Fatalf("invalid interface refs for link bandwidth limit: %s, %s", aRef, bRef)
	}

	found := false
	for _, link := range env.state.ListLinks() {
		if link == nil {
			continue
		}
		if (link.InterfaceA == aRef && link.InterfaceB == bRef) || (link.InterfaceA == bRef && link.InterfaceB == aRef) {
			link.MaxBandwidthBps = capacity
			link.AvailableBandwidthBps = capacity
			if err := env.state.UpdateLink(link); err != nil {
				t.Fatalf("UpdateLink %s: %v", link.ID, err)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("link %s <-> %s not found when setting bandwidth", aRef, bRef)
	}
}

func runSimulationTicks(t *testing.T, env *nbiTestEnv, ticks int) time.Time {
	t.Helper()

	if ticks <= 0 {
		return time.Now().UTC()
	}

	start := time.Now().UTC()
	tick := 20 * time.Millisecond
	tc := timectrl.NewTimeController(start, tick, timectrl.Accelerated)
	duration := time.Duration(ticks) * tick

	tickErr := make(chan error, 1)
	tc.AddListener(func(simTime time.Time) {
		if err := env.state.RunSimTick(simTime, env.motion, nil, nil); err != nil {
			select {
			case tickErr <- err:
			default:
			}
		}
		pushNodePositions(env.physKB, env.netKB)
		env.connectivity.UpdateConnectivity()
	})

	done := tc.Start(duration)
	select {
	case <-done:
	case err := <-tickErr:
		t.Fatalf("RunSimTick: %v", err)
	case err := <-env.serveErr:
		if err != nil {
			t.Fatalf("grpc Serve: %v", err)
		}
	case <-env.ctx.Done():
		t.Fatalf("context deadline exceeded before sim ticks")
	}
	return start.Add(duration)
}

func runScheduler(t *testing.T, env *nbiTestEnv, simTime time.Time) {
	t.Helper()

	env.syncSchedulerClock(simTime)
	sched := env.ensureScheduler(t)
	if err := sched.ScheduleServiceRequests(env.ctx); err != nil {
		t.Fatalf("ScheduleServiceRequests: %v", err)
	}
}

func (env *nbiTestEnv) ensureScheduler(t *testing.T) *controller.Scheduler {
	t.Helper()

	if env.scheduler != nil {
		return env.scheduler
	}
	env.scheduler = controller.NewScheduler(env.state, env.eventScheduler, env.cdpi, logging.Noop(), sim.NewTelemetryState(), nil)
	return env.scheduler
}

func (env *nbiTestEnv) syncSchedulerClock(simTime time.Time) {
	if env == nil || env.eventClock == nil {
		return
	}
	env.eventClock.SetTime(simTime)
}

func (env *nbiTestEnv) registerAgent(nodeID string) {
	if env == nil || env.cdpi == nil {
		return
	}
	env.cdpi.RegisterAgent(nodeID)
}

func getServiceRequestStatus(t *testing.T, env *nbiTestEnv, srID string) *model.ServiceRequestStatus {
	t.Helper()

	if srID == "" {
		t.Fatalf("service request id is empty")
	}

	status, err := env.state.GetServiceRequestStatus(srID)
	if err != nil {
		t.Fatalf("state.GetServiceRequestStatus %s: %v", srID, err)
	}
	return status
}

type positionUpdater interface {
	UpdatePlatformPosition(id string, pos model.Motion) error
}

type deterministicMotion struct {
	mu       sync.Mutex
	updater  positionUpdater
	paths    map[string][]model.Motion
	progress map[string]int
}

func newDeterministicMotion(updater positionUpdater) *deterministicMotion {
	return &deterministicMotion{
		updater:  updater,
		paths:    make(map[string][]model.Motion),
		progress: make(map[string]int),
	}
}

func (m *deterministicMotion) AddPlatform(pd *model.PlatformDefinition) error {
	if pd == nil || pd.ID == "" {
		return fmt.Errorf("platform is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.paths[pd.ID]; !ok {
		m.paths[pd.ID] = []model.Motion{pd.Coordinates}
	}
	m.progress[pd.ID] = 0

	if m.updater != nil {
		_ = m.updater.UpdatePlatformPosition(pd.ID, m.paths[pd.ID][0])
	}

	return nil
}

func (m *deterministicMotion) RemovePlatform(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.paths, id)
	delete(m.progress, id)
	return nil
}

func (m *deterministicMotion) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.progress {
		m.progress[id] = 0
	}
}

func (m *deterministicMotion) UpdatePositions(_ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, path := range m.paths {
		if len(path) == 0 {
			continue
		}

		idx := m.progress[id]
		if idx >= len(path) {
			idx = len(path) - 1
		}
		pos := path[idx]
		if idx < len(path)-1 {
			m.progress[id] = idx + 1
		}
		if m.updater != nil {
			_ = m.updater.UpdatePlatformPosition(id, pos)
		}
	}

	return nil
}

func (m *deterministicMotion) setPath(id string, positions []model.Motion) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(positions) == 0 {
		return
	}
	m.paths[id] = positions
	m.progress[id] = 0
}
