package tests

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	common "aalyria.com/spacetime/api/common"
	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/nbi"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
	"github.com/signalsfoundry/constellation-simulator/timectrl"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestEndToEndNBI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()

	trxID := "trx-e2e-ku"
	if err := netKB.AddTransceiverModel(&core.TransceiverModel{
		ID:         trxID,
		Band:       core.FrequencyBand{MinGHz: 10.7, MaxGHz: 12.75},
		MaxRangeKm: 120000,
	}); err != nil {
		t.Fatalf("AddTransceiverModel: %v", err)
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

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	v1alpha.RegisterPlatformServiceServer(grpcServer, nbi.NewPlatformService(state, motion, logging.Noop()))
	v1alpha.RegisterNetworkNodeServiceServer(grpcServer, nbi.NewNetworkNodeService(state, logging.Noop()))
	v1alpha.RegisterNetworkLinkServiceServer(grpcServer, nbi.NewNetworkLinkService(state, logging.Noop()))
	v1alpha.RegisterServiceRequestServiceServer(grpcServer, nbi.NewServiceRequestService(state, logging.Noop()))
	v1alpha.RegisterScenarioServiceServer(grpcServer, nbi.NewScenarioService(state, logging.Noop()))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- grpcServer.Serve(lis)
	}()
	defer grpcServer.GracefulStop()

	conn, err := grpc.DialContext(ctx, lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.DialContext: %v", err)
	}
	defer conn.Close()

	platformClient := v1alpha.NewPlatformServiceClient(conn)
	nodeClient := v1alpha.NewNetworkNodeServiceClient(conn)
	linkClient := v1alpha.NewNetworkLinkServiceClient(conn)
	srClient := v1alpha.NewServiceRequestServiceClient(conn)
	scenarioClient := v1alpha.NewScenarioServiceClient(conn)

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

	if _, err := nodeClient.CreateNode(ctx, wirelessNodeProto(groundNode, groundID, groundIface, trxID)); err != nil {
		t.Fatalf("CreateNode ground: %v", err)
	}
	if _, err := nodeClient.CreateNode(ctx, wirelessNodeProto(satNode, satID, satIface, trxID)); err != nil {
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

func serviceRequestProto(id, srcNode, dstNode string) *resources.ServiceRequest {
	bw := float64(1_000_000)
	min := float64(500_000)
	priority := float64(1)

	return &resources.ServiceRequest{
		Type:     &id,
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

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
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
