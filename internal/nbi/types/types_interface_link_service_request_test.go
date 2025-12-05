package types

import (
	"math"
	"testing"
	"time"

	core "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/model"

	common "aalyria.com/spacetime/api/common"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
)

func TestInterfaceMappingRoundTrip(t *testing.T) {
	orig := &core.NetworkInterface{
		ID:            "node-123/if-1",
		Name:          "Wireless0",
		Medium:        core.MediumWireless,
		TransceiverID: "trx-123",
		ParentNodeID:  "node-123",
		IsOperational: false,
		MACAddress:    "01:23:45:67:89:ab",
		IPAddress:     "10.0.0.1/24",
	}

	p := InterfaceToProto(orig)
	if p == nil {
		t.Fatalf("InterfaceToProto returned nil")
	}

	back, err := InterfaceFromProto(orig.ParentNodeID, p)
	if err != nil {
		t.Fatalf("InterfaceFromProto returned error: %v", err)
	}

	if back.ID != orig.ID {
		t.Errorf("ID mismatch: got %q, want %q", back.ID, orig.ID)
	}
	if back.Medium != orig.Medium {
		t.Errorf("Medium mismatch: got %q, want %q", back.Medium, orig.Medium)
	}
	if back.TransceiverID != orig.TransceiverID {
		t.Errorf("TransceiverID mismatch: got %q, want %q", back.TransceiverID, orig.TransceiverID)
	}
	if back.ParentNodeID != orig.ParentNodeID {
		t.Errorf("ParentNodeID mismatch: got %q, want %q", back.ParentNodeID, orig.ParentNodeID)
	}
	if back.IPAddress != orig.IPAddress {
		t.Errorf("IPAddress mismatch: got %q, want %q", back.IPAddress, orig.IPAddress)
	}
	if back.MACAddress != orig.MACAddress {
		t.Errorf("MACAddress mismatch: got %q, want %q", back.MACAddress, orig.MACAddress)
	}
	if back.IsOperational != orig.IsOperational {
		t.Errorf("IsOperational mismatch: got %v, want %v", back.IsOperational, orig.IsOperational)
	}
}

func TestLinkMappingRoundTrip(t *testing.T) {
	srcNode := "node-a"
	dstNode := "node-b"
	srcIface := "if-a"
	dstIface := "if-b"
	srcFull := combineInterfaceRef(srcNode, srcIface)
	dstFull := combineInterfaceRef(dstNode, dstIface)

	p := &resources.NetworkLink{
		SrcNetworkNodeId: &srcNode,
		DstNetworkNodeId: &dstNode,
		SrcInterfaceId:   &srcIface,
		DstInterfaceId:   &dstIface,
	}

	dom, err := LinkFromProto(p)
	if err != nil {
		t.Fatalf("LinkFromProto returned error: %v", err)
	}

	if dom.InterfaceA != "node-a/if-a" {
		t.Errorf("InterfaceA mismatch: got %q", dom.InterfaceA)
	}
	if dom.InterfaceB != "node-b/if-b" {
		t.Errorf("InterfaceB mismatch: got %q", dom.InterfaceB)
	}
	if dom.ID != directionalLinkID(srcFull, dstFull) {
		t.Errorf("ID mismatch: got %q, want %q", dom.ID, directionalLinkID(srcFull, dstFull))
	}

	p2 := LinkToProto(dom)
	if got := p2.GetSrcNetworkNodeId(); got != srcNode {
		t.Errorf("SrcNetworkNodeId mismatch: got %q, want %q", got, srcNode)
	}
	if got := p2.GetDstNetworkNodeId(); got != dstNode {
		t.Errorf("DstNetworkNodeId mismatch: got %q, want %q", got, dstNode)
	}
	if got := p2.GetSrcInterfaceId(); got != srcIface {
		t.Errorf("SrcInterfaceId mismatch: got %q, want %q", got, srcIface)
	}
	if got := p2.GetDstInterfaceId(); got != dstIface {
		t.Errorf("DstInterfaceId mismatch: got %q, want %q", got, dstIface)
	}
}

func TestServiceRequestMappingRoundTrip(t *testing.T) {
	orig := &model.ServiceRequest{
		// ID is intentionally NOT part of the proto mapping; it is owned by NBI.
		// For this roundtrip test we leave it empty.
		SrcNodeID:             "node-1",
		DstNodeID:             "node-2",
		Priority:              3,
		IsDisruptionTolerant:  true,
		AllowPartnerResources: true,
		FlowRequirements: []model.FlowRequirement{
			{
				RequestedBandwidth: 150_000_000,
				MinBandwidth:       50_000_000,
				MaxLatency:         0.025,
				ValidFrom:          time.Unix(1_000, 0),
				ValidTo:            time.Unix(2_000, 0),
			},
		},
	}

	p := ServiceRequestToProto(orig)
	if p == nil {
		t.Fatalf("ServiceRequestToProto returned nil")
	}

	back, err := ServiceRequestFromProto(p)
	if err != nil {
		t.Fatalf("ServiceRequestFromProto returned error: %v", err)
	}

	// ID is not derived from the proto; we do NOT assert on back.ID here.

	if back.SrcNodeID != orig.SrcNodeID {
		t.Errorf("SrcNodeID mismatch: got %q, want %q", back.SrcNodeID, orig.SrcNodeID)
	}
	if back.DstNodeID != orig.DstNodeID {
		t.Errorf("DstNodeID mismatch: got %q, want %q", back.DstNodeID, orig.DstNodeID)
	}
	if back.Priority != orig.Priority {
		t.Errorf("Priority mismatch: got %d, want %d", back.Priority, orig.Priority)
	}
	if back.AllowPartnerResources != orig.AllowPartnerResources {
		t.Errorf("AllowPartnerResources mismatch: got %v, want %v", back.AllowPartnerResources, orig.AllowPartnerResources)
	}
	if back.IsDisruptionTolerant != orig.IsDisruptionTolerant {
		t.Errorf("IsDisruptionTolerant mismatch: got %v, want %v", back.IsDisruptionTolerant, orig.IsDisruptionTolerant)
	}

	if len(back.FlowRequirements) != 1 {
		t.Fatalf("expected 1 flow requirement, got %d", len(back.FlowRequirements))
	}

	got := back.FlowRequirements[0]
	want := orig.FlowRequirements[0]

	if diff := math.Abs(got.RequestedBandwidth - want.RequestedBandwidth); diff > 1e-6 {
		t.Errorf("RequestedBandwidth mismatch: got %f, want %f", got.RequestedBandwidth, want.RequestedBandwidth)
	}
	if diff := math.Abs(got.MinBandwidth - want.MinBandwidth); diff > 1e-6 {
		t.Errorf("MinBandwidth mismatch: got %f, want %f", got.MinBandwidth, want.MinBandwidth)
	}
	if diff := math.Abs(got.MaxLatency - want.MaxLatency); diff > 1e-6 {
		t.Errorf("MaxLatency mismatch: got %f, want %f", got.MaxLatency, want.MaxLatency)
	}
	if !got.ValidFrom.Equal(want.ValidFrom) {
		t.Errorf("ValidFrom mismatch: got %v, want %v", got.ValidFrom, want.ValidFrom)
	}
	if !got.ValidTo.Equal(want.ValidTo) {
		t.Errorf("ValidTo mismatch: got %v, want %v", got.ValidTo, want.ValidTo)
	}
}

func TestPlatformMappingRoundTrip(t *testing.T) {
	name := "platform-1"
	typ := "SATELLITE"
	category := "LEO"
	var norad uint32 = 12345
	ms := common.PlatformDefinition_SPACETRACK_ORG

	x := 1000.0
	y := 2000.0
	z := 3000.0

	// Start from proto → domain → proto, as per issue text.
	p := &PlatformDefinition{
		Name:         &name,
		Type:         &typ,
		CategoryTag:  &category,
		NoradId:      &norad,
		MotionSource: &ms,
		Coordinates: &common.Motion{
			Type: &common.Motion_EcefFixed{
				EcefFixed: &common.PointAxes{
					Point: &common.Cartesian{
						XM: &x,
						YM: &y,
						ZM: &z,
					},
				},
			},
		},
	}

	dom, err := PlatformFromProto(p)
	if err != nil {
		t.Fatalf("PlatformFromProto returned error: %v", err)
	}

	// Domain checks.
	if dom.ID != name {
		t.Errorf("ID mismatch: got %q, want %q", dom.ID, name)
	}
	if dom.Name != name {
		t.Errorf("Name mismatch: got %q, want %q", dom.Name, name)
	}
	if dom.Type != typ {
		t.Errorf("Type mismatch: got %q, want %q", dom.Type, typ)
	}
	if dom.CategoryTag != category {
		t.Errorf("CategoryTag mismatch: got %q, want %q", dom.CategoryTag, category)
	}
	if dom.NoradID != norad {
		t.Errorf("NoradID mismatch: got %d, want %d", dom.NoradID, norad)
	}
	if dom.MotionSource != model.MotionSourceSpacetrack {
		t.Errorf("MotionSource mismatch: got %v, want %v", dom.MotionSource, model.MotionSourceSpacetrack)
	}
	if diff := math.Abs(dom.Coordinates.X - x); diff > 1e-6 {
		t.Errorf("X mismatch: got %f, want %f", dom.Coordinates.X, x)
	}
	if diff := math.Abs(dom.Coordinates.Y - y); diff > 1e-6 {
		t.Errorf("Y mismatch: got %f, want %f", dom.Coordinates.Y, y)
	}
	if diff := math.Abs(dom.Coordinates.Z - z); diff > 1e-6 {
		t.Errorf("Z mismatch: got %f, want %f", dom.Coordinates.Z, z)
	}

	// Roundtrip back to proto.
	p2 := PlatformToProto(dom)
	if p2 == nil {
		t.Fatalf("PlatformToProto returned nil")
	}

	if got := p2.GetName(); got != name {
		t.Errorf("Name roundtrip mismatch: got %q, want %q", got, name)
	}
	if got := p2.GetType(); got != typ {
		t.Errorf("Type roundtrip mismatch: got %q, want %q", got, typ)
	}
	if got := p2.GetCategoryTag(); got != category {
		t.Errorf("CategoryTag roundtrip mismatch: got %q, want %q", got, category)
	}
	if got := p2.GetNoradId(); got != norad {
		t.Errorf("NoradID roundtrip mismatch: got %d, want %d", got, norad)
	}
	if got := p2.GetMotionSource(); got != ms {
		t.Errorf("MotionSource roundtrip mismatch: got %v, want %v", got, ms)
	}

	coords := p2.GetCoordinates().GetEcefFixed().GetPoint()
	if coords == nil {
		t.Fatalf("Coordinates missing after roundtrip")
	}
	if diff := math.Abs(coords.GetXM() - x); diff > 1e-6 {
		t.Errorf("XM roundtrip mismatch: got %f, want %f", coords.GetXM(), x)
	}
	if diff := math.Abs(coords.GetYM() - y); diff > 1e-6 {
		t.Errorf("YM roundtrip mismatch: got %f, want %f", coords.GetYM(), y)
	}
	if diff := math.Abs(coords.GetZM() - z); diff > 1e-6 {
		t.Errorf("ZM roundtrip mismatch: got %f, want %f", coords.GetZM(), z)
	}
}

func TestNetworkNodeMappingRoundTrip(t *testing.T) {
	id := "node-1"
	name := "Node-1"
	typ := "ROUTER"

	// Start from proto → domain → proto.
	p := &NetworkNode{
		NodeId: &id,
		Name:   &name,
		Type:   &typ,
	}

	dom, err := NodeFromProto(p)
	if err != nil {
		t.Fatalf("NodeFromProto returned error: %v", err)
	}

	// Domain checks.
	if dom.ID != id {
		t.Errorf("ID mismatch: got %q, want %q", dom.ID, id)
	}
	if dom.Name != name {
		t.Errorf("Name mismatch: got %q, want %q", dom.Name, name)
	}
	if dom.Type != typ {
		t.Errorf("Type mismatch: got %q, want %q", dom.Type, typ)
	}
	// PlatformID is intentionally not mapped from the proto.
	if dom.PlatformID != "" {
		t.Errorf("expected PlatformID to be empty, got %q", dom.PlatformID)
	}

	// Roundtrip back to proto.
	p2 := NodeToProto(dom)
	if p2 == nil {
		t.Fatalf("NodeToProto returned nil")
	}

	if got := p2.GetNodeId(); got != id {
		t.Errorf("NodeId roundtrip mismatch: got %q, want %q", got, id)
	}
	if got := p2.GetName(); got != name {
		t.Errorf("Name roundtrip mismatch: got %q, want %q", got, name)
	}
	if got := p2.GetType(); got != typ {
		t.Errorf("Type roundtrip mismatch: got %q, want %q", got, typ)
	}
}

func TestBidirectionalLinkFromProto(t *testing.T) {
	aNode := "node-a"
	bNode := "node-b"
	aTx := "if-a-tx"
	aRx := "if-a-rx"
	bTx := "if-b-tx"
	bRx := "if-b-rx"

	p := &resources.BidirectionalLink{
		ANetworkNodeId: &aNode,
		BNetworkNodeId: &bNode,
		ATxInterfaceId: &aTx,
		ARxInterfaceId: &aRx,
		BTxInterfaceId: &bTx,
		BRxInterfaceId: &bRx,
	}

	links, err := BidirectionalLinkFromProto(p)
	if err != nil {
		t.Fatalf("BidirectionalLinkFromProto returned error: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 directional links, got %d", len(links))
	}

	expected := map[string]struct {
		src string
		dst string
	}{
		directionalLinkID(combineInterfaceRef(aNode, aTx), combineInterfaceRef(bNode, bRx)): {combineInterfaceRef(aNode, aTx), combineInterfaceRef(bNode, bRx)},
		directionalLinkID(combineInterfaceRef(bNode, bTx), combineInterfaceRef(aNode, aRx)): {combineInterfaceRef(bNode, bTx), combineInterfaceRef(aNode, aRx)},
	}

	for _, l := range links {
		want, ok := expected[l.ID]
		if !ok {
			t.Fatalf("unexpected link ID %q", l.ID)
		}
		if l.InterfaceA != want.src {
			t.Errorf("InterfaceA mismatch for %q: got %q, want %q", l.ID, l.InterfaceA, want.src)
		}
		if l.InterfaceB != want.dst {
			t.Errorf("InterfaceB mismatch for %q: got %q, want %q", l.ID, l.InterfaceB, want.dst)
		}
	}
}

func TestBidirectionalLinkToProto(t *testing.T) {
	linkAB := newDirectionalLink("node-a/a-tx", "node-b/b-rx")
	linkBA := newDirectionalLink("node-b/b-tx", "node-a/a-rx")

	p := BidirectionalLinkToProto(linkAB, linkBA)
	if p == nil {
		t.Fatalf("BidirectionalLinkToProto returned nil")
	}

	if got := p.GetANetworkNodeId(); got != "node-a" {
		t.Errorf("ANetworkNodeId = %q, want node-a", got)
	}
	if got := p.GetBNetworkNodeId(); got != "node-b" {
		t.Errorf("BNetworkNodeId = %q, want node-b", got)
	}
	if got := p.GetATxInterfaceId(); got != "a-tx" {
		t.Errorf("ATxInterfaceId = %q, want a-tx", got)
	}
	if got := p.GetARxInterfaceId(); got != "a-rx" {
		t.Errorf("ARxInterfaceId = %q, want a-rx", got)
	}
	if got := p.GetBTxInterfaceId(); got != "b-tx" {
		t.Errorf("BTxInterfaceId = %q, want b-tx", got)
	}
	if got := p.GetBRxInterfaceId(); got != "b-rx" {
		t.Errorf("BRxInterfaceId = %q, want b-rx", got)
	}
}
