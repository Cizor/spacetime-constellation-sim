package nbi

import (
	"context"
	"strings"
	"testing"
	"time"

	common "aalyria.com/spacetime/api/common"
	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
	core "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
	"google.golang.org/protobuf/types/known/durationpb"
)

type stubMotionModel struct {
	added   []string
	removed []string
	resets  int
}

func (m *stubMotionModel) AddPlatform(pd *model.PlatformDefinition) error {
	if pd != nil {
		m.added = append(m.added, pd.ID)
	}
	return nil
}

func (m *stubMotionModel) RemovePlatform(platformID string) error {
	m.removed = append(m.removed, platformID)
	return nil
}

func (m *stubMotionModel) Reset() {
	m.resets++
}

type stubConnectivity struct {
	resets int
}

func (c *stubConnectivity) Reset() {
	c.resets++
}

func stringPtr(s string) *string { return &s }

func newPlatformProto(id string) *common.PlatformDefinition {
	typ := "SATELLITE"
	ms := common.PlatformDefinition_SPACETRACK_ORG
	return &common.PlatformDefinition{
		Name:         stringPtr(id),
		Type:         &typ,
		MotionSource: &ms,
	}
}

func newNodeProto(id, platformID string, ifaceIDs ...string) *resources.NetworkNode {
	typ := "ROUTER"
	var ifaces []*resources.NetworkInterface
	for _, ifaceID := range ifaceIDs {
		ifaces = append(ifaces, &resources.NetworkInterface{
			InterfaceId: stringPtr(ifaceID),
			InterfaceMedium: &resources.NetworkInterface_Wired{
				Wired: &resources.WiredDevice{
					PlatformId: stringPtr(platformID),
				},
			},
		})
	}
	return &resources.NetworkNode{
		NodeId:        stringPtr(id),
		Type:          &typ,
		NodeInterface: ifaces,
	}
}

func newBidirectionalLinkProto(aNode, aIface, bNode, bIface string) *resources.BidirectionalLink {
	return &resources.BidirectionalLink{
		ANetworkNodeId: stringPtr(aNode),
		ATxInterfaceId: stringPtr(aIface),
		ARxInterfaceId: stringPtr(aIface),
		BNetworkNodeId: stringPtr(bNode),
		BTxInterfaceId: stringPtr(bIface),
		BRxInterfaceId: stringPtr(bIface),
	}
}

func newServiceRequestProto(id, srcNode, dstNode string) *resources.ServiceRequest {
	bw := float64(1_000_000)
	min := float64(500_000)
	lat := durationpb.New(2 * time.Second)

	return &resources.ServiceRequest{
		Type: &id,
		SrcType: &resources.ServiceRequest_SrcNodeId{
			SrcNodeId: srcNode,
		},
		DstType: &resources.ServiceRequest_DstNodeId{
			DstNodeId: dstNode,
		},
		Requirements: []*resources.ServiceRequest_FlowRequirements{
			{
				BandwidthBpsRequested: &bw,
				BandwidthBpsMinimum:   &min,
				LatencyMaximum:        lat,
			},
		},
	}
}

func newScenarioServicesForTest() (*ScenarioService, *PlatformService, *NetworkNodeService, *NetworkLinkService, *ServiceRequestService, *sim.ScenarioState, *stubMotionModel, *stubConnectivity) {
	motion := &stubMotionModel{}
	connectivity := &stubConnectivity{}
	state := sim.NewScenarioState(
		kb.NewKnowledgeBase(),
		core.NewKnowledgeBase(),
		logging.Noop(),
		sim.WithMotionModel(motion),
		sim.WithConnectivityService(connectivity),
	)

	return NewScenarioService(state, logging.Noop()),
		NewPlatformService(state, motion, logging.Noop()),
		NewNetworkNodeService(state, logging.Noop()),
		NewNetworkLinkService(state, logging.Noop()),
		NewServiceRequestService(state, logging.Noop()),
		state,
		motion,
		connectivity
}

func TestScenarioServiceRoundtrip(t *testing.T) {
	ctx := context.Background()
	scenarioSvc, platformSvc, nodeSvc, linkSvc, srSvc, state, motion, connectivity := newScenarioServicesForTest()

	platformID := "platform-roundtrip"
	if _, err := platformSvc.CreatePlatform(ctx, newPlatformProto(platformID)); err != nil {
		t.Fatalf("CreatePlatform error: %v", err)
	}

	nodeA := "node-a"
	nodeB := "node-b"
	if _, err := nodeSvc.CreateNode(ctx, newNodeProto(nodeA, platformID, "ifA")); err != nil {
		t.Fatalf("CreateNode(%s) error: %v", nodeA, err)
	}
	if _, err := nodeSvc.CreateNode(ctx, newNodeProto(nodeB, platformID, "ifB")); err != nil {
		t.Fatalf("CreateNode(%s) error: %v", nodeB, err)
	}

	if _, err := linkSvc.CreateLink(ctx, newBidirectionalLinkProto(nodeA, "ifA", nodeB, "ifB")); err != nil {
		t.Fatalf("CreateLink error: %v", err)
	}

	srID := "sr-roundtrip"
	if _, err := srSvc.CreateServiceRequest(ctx, newServiceRequestProto(srID, nodeA, nodeB)); err != nil {
		t.Fatalf("CreateServiceRequest error: %v", err)
	}

	before, err := scenarioSvc.GetScenario(ctx, &v1alpha.GetScenarioRequest{})
	if err != nil {
		t.Fatalf("GetScenario before clear error: %v", err)
	}

	if _, err := scenarioSvc.ClearScenario(ctx, &v1alpha.ClearScenarioRequest{}); err != nil {
		t.Fatalf("ClearScenario error: %v", err)
	}

	if got := len(state.ListPlatforms()); got != 0 {
		t.Fatalf("ListPlatforms after clear = %d, want 0", got)
	}
	if got := len(state.ListNodes()); got != 0 {
		t.Fatalf("ListNodes after clear = %d, want 0", got)
	}
	if got := len(state.NetworkKB().GetAllInterfaces()); got != 0 {
		t.Fatalf("GetAllInterfaces after clear = %d, want 0", got)
	}
	if got := len(state.ListLinks()); got != 0 {
		t.Fatalf("ListLinks after clear = %d, want 0", got)
	}
	if got := len(state.ListServiceRequests()); got != 0 {
		t.Fatalf("ListServiceRequests after clear = %d, want 0", got)
	}
	if motion.resets != 1 {
		t.Fatalf("motion.Reset calls = %d, want 1", motion.resets)
	}
	if connectivity.resets != 1 {
		t.Fatalf("connectivity.Reset calls = %d, want 1", connectivity.resets)
	}

	loadReq := &v1alpha.LoadScenarioRequest{
		Platforms:       before.GetPlatforms(),
		Nodes:           before.GetNodes(),
		Links:           before.GetLinks(),
		ServiceRequests: before.GetServiceRequests(),
	}
	if _, err := scenarioSvc.LoadScenario(ctx, loadReq); err != nil {
		t.Fatalf("LoadScenario error: %v", err)
	}
	if motion.resets != 2 {
		t.Fatalf("motion.Reset calls after LoadScenario = %d, want 2", motion.resets)
	}
	if connectivity.resets != 2 {
		t.Fatalf("connectivity.Reset calls after LoadScenario = %d, want 2", connectivity.resets)
	}

	after, err := scenarioSvc.GetScenario(ctx, &v1alpha.GetScenarioRequest{})
	if err != nil {
		t.Fatalf("GetScenario after load error: %v", err)
	}

	assertSnapshotsEquivalent(t, before, after)
}

func assertSnapshotsEquivalent(t *testing.T, want, got *v1alpha.ScenarioSnapshot) {
	t.Helper()

	if want == nil {
		t.Fatal("want snapshot is nil")
	}
	if got == nil {
		t.Fatal("got snapshot is nil")
	}

	if len(want.GetPlatforms()) != len(got.GetPlatforms()) {
		t.Fatalf("platform count = %d, want %d", len(got.GetPlatforms()), len(want.GetPlatforms()))
	}
	wantPlatforms := make(map[string]*common.PlatformDefinition)
	for _, pd := range want.GetPlatforms() {
		wantPlatforms[pd.GetName()] = pd
	}
	for _, pd := range got.GetPlatforms() {
		if exp, ok := wantPlatforms[pd.GetName()]; !ok {
			t.Fatalf("unexpected platform %q in snapshot", pd.GetName())
		} else if pd.GetType() != exp.GetType() {
			t.Fatalf("platform %q type = %q, want %q", pd.GetName(), pd.GetType(), exp.GetType())
		}
	}

	if len(want.GetNodes()) != len(got.GetNodes()) {
		t.Fatalf("node count = %d, want %d", len(got.GetNodes()), len(want.GetNodes()))
	}
	wantNodes := make(map[string]*resources.NetworkNode)
	for _, n := range want.GetNodes() {
		wantNodes[n.GetNodeId()] = n
	}
	for _, n := range got.GetNodes() {
		exp, ok := wantNodes[n.GetNodeId()]
		if !ok {
			t.Fatalf("unexpected node %q in snapshot", n.GetNodeId())
		}
		if n.GetType() != exp.GetType() {
			t.Fatalf("node %q type = %q, want %q", n.GetNodeId(), n.GetType(), exp.GetType())
		}
		assertInterfacesEquivalent(t, n, exp)
	}

	if len(want.GetLinks()) != len(got.GetLinks()) {
		t.Fatalf("link count = %d, want %d", len(got.GetLinks()), len(want.GetLinks()))
	}
	wantLinks := make(map[string]*resources.BidirectionalLink)
	for _, l := range want.GetLinks() {
		wantLinks[linkKey(l)] = l
	}
	for _, l := range got.GetLinks() {
		key := linkKey(l)
		if _, ok := wantLinks[key]; !ok {
			t.Fatalf("unexpected link key %q in snapshot", key)
		}
	}

	if len(want.GetServiceRequests()) != len(got.GetServiceRequests()) {
		t.Fatalf("service request count = %d, want %d", len(got.GetServiceRequests()), len(want.GetServiceRequests()))
	}
	wantSRs := make(map[string]*resources.ServiceRequest)
	for _, sr := range want.GetServiceRequests() {
		wantSRs[serviceRequestKey(sr)] = sr
	}
	for _, sr := range got.GetServiceRequests() {
		key := serviceRequestKey(sr)
		exp, ok := wantSRs[key]
		if !ok {
			t.Fatalf("unexpected service request key %q in snapshot", key)
		}
		if sr.GetSrcNodeId() != exp.GetSrcNodeId() || sr.GetDstNodeId() != exp.GetDstNodeId() {
			t.Fatalf("service request %q endpoints = (%s,%s), want (%s,%s)", key, sr.GetSrcNodeId(), sr.GetDstNodeId(), exp.GetSrcNodeId(), exp.GetDstNodeId())
		}
		if len(sr.GetRequirements()) != len(exp.GetRequirements()) {
			t.Fatalf("service request %q requirements count = %d, want %d", key, len(sr.GetRequirements()), len(exp.GetRequirements()))
		}
		for i := range sr.GetRequirements() {
			req := sr.GetRequirements()[i]
			wantReq := exp.GetRequirements()[i]
			if req.GetBandwidthBpsRequested() != wantReq.GetBandwidthBpsRequested() ||
				req.GetBandwidthBpsMinimum() != wantReq.GetBandwidthBpsMinimum() {
				t.Fatalf("service request %q requirement %d bandwidth mismatch", key, i)
			}
			if req.GetLatencyMaximum().GetSeconds() != wantReq.GetLatencyMaximum().GetSeconds() {
				t.Fatalf("service request %q requirement %d latency = %v, want %v", key, i, req.GetLatencyMaximum(), wantReq.GetLatencyMaximum())
			}
		}
	}
}

func assertInterfacesEquivalent(t *testing.T, got, want *resources.NetworkNode) {
	t.Helper()

	wantIfaces := make(map[string]*resources.NetworkInterface)
	for _, iface := range want.GetNodeInterface() {
		wantIfaces[interfaceKey(want.GetNodeId(), iface)] = iface
	}
	if len(got.GetNodeInterface()) != len(wantIfaces) {
		t.Fatalf("node %q interface count = %d, want %d", got.GetNodeId(), len(got.GetNodeInterface()), len(wantIfaces))
	}

	for _, iface := range got.GetNodeInterface() {
		key := interfaceKey(got.GetNodeId(), iface)
		exp, ok := wantIfaces[key]
		if !ok {
			t.Fatalf("unexpected interface %q on node %q", key, got.GetNodeId())
		}
		if mediumType(iface) != mediumType(exp) {
			t.Fatalf("interface %q medium = %s, want %s", key, mediumType(iface), mediumType(exp))
		}
	}
}

func interfaceKey(nodeID string, iface *resources.NetworkInterface) string {
	id := iface.GetInterfaceId()
	if strings.Contains(id, "/") {
		return id
	}
	return nodeID + "/" + id
}

func mediumType(iface *resources.NetworkInterface) string {
	switch iface.GetInterfaceMedium().(type) {
	case *resources.NetworkInterface_Wired:
		return "wired"
	case *resources.NetworkInterface_Wireless:
		return "wireless"
	default:
		return "unknown"
	}
}
