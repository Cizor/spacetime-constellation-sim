package types

import (
	"errors"
	"fmt"
	"strings"
	"time"

	core "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/model"

	common "aalyria.com/spacetime/api/common"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
	"google.golang.org/protobuf/types/known/durationpb"
)

//
// Type aliases to Aalyria-generated protobuf messages.
//
// These keep the rest of the simulator code decoupled from the exact
// import paths of the generated code and make it clear which protos
// are considered “first-class” NBI entities for this project.
//

// PlatformDefinition is the Aalyria proto for a physical platform
// (satellite, aircraft, ground station, etc.).
type PlatformDefinition = common.PlatformDefinition

// NetworkNode represents a logical device in the network (router,
// satellite payload, ground station node, etc.).
type NetworkNode = resources.NetworkNode

// NetworkInterface represents a logical interface on a NetworkNode.
type NetworkInterface = resources.NetworkInterface

// NetworkLink is the directional link resource.
type NetworkLink = resources.NetworkLink

// BidirectionalLink pairs two directional links into a conceptual
// bidirectional relationship.
type BidirectionalLink = resources.BidirectionalLink

// ServiceRequest describes a requested flow between endpoints with
// QoS / timing constraints.
type ServiceRequest = resources.ServiceRequest

//
// Mapping functions.
//
// Platform + NetworkNode are implemented here; the others are declared
// and will be filled out in later Scope-3 chunks.
//

// PlatformFromProto converts an Aalyria PlatformDefinition into the
// simulator's domain model representation.
//
// Conventions:
//   - We treat proto `name` as the stable ID and map it to ID and Name.
//   - We currently pull ECEF position from Motion.ecef_fixed.point.x_m/y_m/z_m.
//   - MotionSource is mapped onto the domain MotionSource enum.
func PlatformFromProto(pd *PlatformDefinition) (*model.PlatformDefinition, error) {
	if pd == nil {
		return nil, errors.New("nil PlatformDefinition proto")
	}

	dom := &model.PlatformDefinition{
		ID:          pd.GetName(),
		Name:        pd.GetName(),
		Type:        pd.GetType(),
		CategoryTag: pd.GetCategoryTag(),
		NoradID:     pd.GetNoradId(),
	}

	switch pd.GetMotionSource() {
	case common.PlatformDefinition_SPACETRACK_ORG:
		dom.MotionSource = model.MotionSourceSpacetrack
	default:
		dom.MotionSource = model.MotionSourceUnknown
	}

	// Extract ECEF coordinates if present.
	if m := pd.GetCoordinates(); m != nil {
		if pa := m.GetEcefFixed(); pa != nil {
			if c := pa.GetPoint(); c != nil {
				dom.Coordinates = model.Motion{
					X: c.GetXM(),
					Y: c.GetYM(),
					Z: c.GetZM(),
				}
			}
		}
	}

	return dom, nil
}

// PlatformToProto converts a domain PlatformDefinition back into the
// Aalyria proto form.
//
// Important: the Aalyria codegen uses pointer fields for scalars
// (e.g. *string, *uint32, *float64), so we take addresses of locals.
func PlatformToProto(dom *model.PlatformDefinition) *PlatformDefinition {
	if dom == nil {
		return nil
	}

	name := dom.Name
	typ := dom.Type
	category := dom.CategoryTag
	norad := dom.NoradID

	pd := &PlatformDefinition{
		Name:        &name,
		Type:        &typ,
		CategoryTag: &category,
		NoradId:     &norad,
	}

	// MotionSource mapping – pointer field.
	switch dom.MotionSource {
	case model.MotionSourceSpacetrack:
		ms := common.PlatformDefinition_SPACETRACK_ORG
		pd.MotionSource = &ms
	default:
		ms := common.PlatformDefinition_UNKNOWN_SOURCE
		pd.MotionSource = &ms
	}

	// Emit coordinates as an ECEF_FIXED Motion with a Cartesian point.
	x := dom.Coordinates.X
	y := dom.Coordinates.Y
	z := dom.Coordinates.Z

	pd.Coordinates = &common.Motion{
		Type: &common.Motion_EcefFixed{
			EcefFixed: &common.PointAxes{
				Point: &common.Cartesian{
					XM: &x,
					YM: &y,
					ZM: &z,
				},
				// Axes: nil = default ECEF axes.
			},
		},
	}

	return pd
}

// NodeFromProto converts an Aalyria NetworkNode into the simulator's
// domain NetworkNode representation.
//
// We map:
//   - node_id -> ID
//   - name    -> Name
//   - type    -> Type
//
// Platform association is carried via NetworkInterface.*Device in Aalyria;
// PlatformID is left empty here and wired at a higher level.
func NodeFromProto(n *NetworkNode) (*model.NetworkNode, error) {
	if n == nil {
		return nil, errors.New("nil NetworkNode proto")
	}

	dom := &model.NetworkNode{
		ID:   n.GetNodeId(),
		Name: n.GetName(),
		Type: n.GetType(),
		// PlatformID intentionally left empty here.
		PlatformID: "",
	}

	return dom, nil
}

// NodeWithInterfacesFromProto converts a NetworkNode and any embedded
// NetworkInterface messages into the domain representations.
//
// Optional proto fields that are not represented in the domain model
// (category_tag, routing_config, SDN agent, storage, power budgets)
// are intentionally ignored here.
func NodeWithInterfacesFromProto(n *NetworkNode) (*model.NetworkNode, []*core.NetworkInterface, error) {
	node, err := NodeFromProto(n)
	if err != nil {
		return nil, nil, err
	}

	var ifaces []*core.NetworkInterface
	for _, iface := range n.GetNodeInterface() {
		if iface == nil {
			continue
		}
		domIF, err := InterfaceFromProto(node.ID, iface)
		if err != nil {
			return nil, nil, err
		}
		ifaces = append(ifaces, domIF)
	}

	return node, ifaces, nil
}

// NodeToProto converts a domain NetworkNode back into the Aalyria
// NetworkNode proto.
//
// Only ID/Name/Type are round-tripped here; platform association is
// expected to be encoded via interfaces in the NBI layer.
func NodeToProto(dom *model.NetworkNode) *NetworkNode {
	if dom == nil {
		return nil
	}

	id := dom.ID
	name := dom.Name
	typ := dom.Type

	return &NetworkNode{
		NodeId: &id,
		Name:   &name,
		Type:   &typ,
	}
}

// NodeToProtoWithInterfaces emits a NetworkNode proto and populates
// embedded NetworkInterface messages for interfaces that belong to the
// provided node.
func NodeToProtoWithInterfaces(node *model.NetworkNode, ifaces []*core.NetworkInterface) *NetworkNode {
	p := NodeToProto(node)
	if p == nil {
		return nil
	}

	for _, iface := range ifaces {
		if iface == nil {
			continue
		}

		if node != nil && node.ID != "" {
			if parent := iface.ParentNodeID; parent != "" && parent != node.ID {
				continue
			}
			if nodePart, _ := splitInterfaceRef(iface.ID); nodePart != "" && nodePart != node.ID {
				continue
			}
		}

		if pif := InterfaceToProto(iface); pif != nil {
			p.NodeInterface = append(p.NodeInterface, pif)
		}
	}

	return p
}

// InterfaceFromProto converts an Aalyria NetworkInterface into the
// simulator's core.NetworkInterface representation.
func InterfaceFromProto(parentNodeID string, iface *NetworkInterface) (*core.NetworkInterface, error) {
	if iface == nil {
		return nil, errors.New("nil NetworkInterface proto")
	}

	// Derive node/interface IDs. Aalyria specifies interface_id as
	// node-unique; we combine it with the parent node ID to make it
	// globally unique in the core layer.
	localID := iface.GetInterfaceId()
	if idNode, idLocal := splitInterfaceRef(localID); idNode != "" {
		parentNodeID = idNode
		localID = idLocal
	}
	if parentNodeID == "" && localID == "" {
		return nil, errors.New("interface_id is required")
	}
	fullID := combineInterfaceRef(parentNodeID, localID)
	if fullID == "" {
		fullID = localID
	}

	medium := core.MediumType("")
	transceiverID := ""

	switch m := iface.GetInterfaceMedium().(type) {
	case *resources.NetworkInterface_Wired:
		medium = core.MediumWired
	case *resources.NetworkInterface_Wireless:
		medium = core.MediumWireless
		if m.Wireless != nil && m.Wireless.TransceiverModelId != nil {
			transceiverID = m.Wireless.TransceiverModelId.GetTransceiverModelId()
		}
	}

	isOperational := len(iface.GetOperationalImpairment()) == 0

	return &core.NetworkInterface{
		ID:            fullID,
		Name:          iface.GetName(),
		Medium:        medium,
		TransceiverID: transceiverID,
		ParentNodeID:  parentNodeID,
		IsOperational: isOperational,
		MACAddress:    iface.GetEthernetAddress(),
		IPAddress:     iface.GetIpAddress(),
	}, nil
}

// InterfaceToProto converts a core.NetworkInterface back into the
// Aalyria NetworkInterface proto.
func InterfaceToProto(iface *core.NetworkInterface) *NetworkInterface {
	if iface == nil {
		return nil
	}

	_, id := splitInterfaceRef(iface.ID)
	if id == "" {
		id = iface.ID
	}
	if id == "" {
		return nil
	}

	name := iface.Name
	ip := iface.IPAddress
	mac := iface.MACAddress

	p := &resources.NetworkInterface{
		InterfaceId: &id,
	}

	if name != "" {
		p.Name = &name
	}
	if ip != "" {
		p.IpAddress = &ip
	}
	if mac != "" {
		p.EthernetAddress = &mac
	}

	switch iface.Medium {
	case core.MediumWired:
		p.InterfaceMedium = &resources.NetworkInterface_Wired{
			Wired: &resources.WiredDevice{},
		}
	default:
		wd := &resources.WirelessDevice{}
		if iface.TransceiverID != "" {
			trx := iface.TransceiverID
			wd.TransceiverModelId = &common.TransceiverModelId{
				TransceiverModelId: &trx,
			}
		}
		p.InterfaceMedium = &resources.NetworkInterface_Wireless{
			Wireless: wd,
		}
	}

	if !iface.IsOperational {
		imp := resources.NetworkInterface_Impairment_DEFAULT_UNUSABLE
		p.OperationalImpairment = []*resources.NetworkInterface_Impairment{
			{Type: &imp},
		}
	}

	return p
}

// LinkFromProto converts an Aalyria NetworkLink into the simulator's
// core.NetworkLink representation.
func LinkFromProto(link *NetworkLink) (*core.NetworkLink, error) {
	if link == nil {
		return nil, errors.New("nil NetworkLink proto")
	}

	srcNode := link.GetSrcNetworkNodeId()
	dstNode := link.GetDstNetworkNodeId()
	srcIface := link.GetSrcInterfaceId()
	dstIface := link.GetDstInterfaceId()

	// Support deprecated src/dst fields as a fallback.
	if srcIface == "" && link.GetSrc() != nil {
		srcNode = link.GetSrc().GetNodeId()
		srcIface = link.GetSrc().GetInterfaceId()
	}
	if dstIface == "" && link.GetDst() != nil {
		dstNode = link.GetDst().GetNodeId()
		dstIface = link.GetDst().GetInterfaceId()
	}

	intA := normalizeInterfaceRef(srcNode, srcIface)
	intB := normalizeInterfaceRef(dstNode, dstIface)

	if intA == "" || intB == "" {
		return nil, fmt.Errorf("link endpoints are incomplete: src=%q dst=%q", intA, intB)
	}

	return newDirectionalLink(intA, intB), nil
}

// LinkToProto converts a core.NetworkLink back into the Aalyria
// NetworkLink proto.
func LinkToProto(link *core.NetworkLink) *NetworkLink {
	if link == nil {
		return nil
	}

	srcNode, srcIface := splitInterfaceRef(link.InterfaceA)
	dstNode, dstIface := splitInterfaceRef(link.InterfaceB)

	p := &resources.NetworkLink{}

	if srcNode != "" || srcIface != "" {
		p.SrcNetworkNodeId = stringPtr(srcNode)
		p.SrcInterfaceId = stringPtr(srcIface)
	}
	if dstNode != "" || dstIface != "" {
		p.DstNetworkNodeId = stringPtr(dstNode)
		p.DstInterfaceId = stringPtr(dstIface)
	}

	// Populate deprecated fields for compatibility if we have both halves.
	if srcNode != "" && srcIface != "" {
		p.Src = &common.NetworkInterfaceId{
			NodeId:      &srcNode,
			InterfaceId: &srcIface,
		}
	}
	if dstNode != "" && dstIface != "" {
		p.Dst = &common.NetworkInterfaceId{
			NodeId:      &dstNode,
			InterfaceId: &dstIface,
		}
	}

	return p
}

// BidirectionalLinkFromProto converts an Aalyria BidirectionalLink into
// a single core.NetworkLink object representing the undirected pair.
// Missing endpoints yield an error.
func BidirectionalLinkFromProto(link *BidirectionalLink) ([]*core.NetworkLink, error) {
	if link == nil {
		return nil, errors.New("nil BidirectionalLink proto")
	}

	endA := extractBidirectionalEndpoint(
		link.GetANetworkNodeId(),
		link.GetATxInterfaceId(),
		link.GetARxInterfaceId(),
		link.GetA(),
	)
	endB := extractBidirectionalEndpoint(
		link.GetBNetworkNodeId(),
		link.GetBTxInterfaceId(),
		link.GetBRxInterfaceId(),
		link.GetB(),
	)

	aEndpoint := normalizeInterfaceRef(endA.nodeID, endA.txInterface())
	bEndpoint := normalizeInterfaceRef(endB.nodeID, endB.txInterface())

	if aEndpoint == "" || bEndpoint == "" {
		return nil, errors.New("bidirectional link endpoints are incomplete")
	}

	return []*core.NetworkLink{newBidirectionalLink(aEndpoint, bEndpoint)}, nil
}

// BidirectionalLinkToProto reconstructs an Aalyria BidirectionalLink
// from one or two directional core.NetworkLink objects.
func BidirectionalLinkToProto(links ...*core.NetworkLink) *BidirectionalLink {
	filtered := make([]*core.NetworkLink, 0, len(links))
	for _, l := range links {
		if l != nil {
			filtered = append(filtered, l)
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	primary := filtered[0]
	aNode, aIface := splitInterfaceRef(primary.InterfaceA)
	bNode, bIface := splitInterfaceRef(primary.InterfaceB)

	p := &resources.BidirectionalLink{
		ANetworkNodeId: stringPtr(aNode),
		BNetworkNodeId: stringPtr(bNode),
		ATxInterfaceId: stringPtr(aIface),
		ARxInterfaceId: stringPtr(aIface),
		BTxInterfaceId: stringPtr(bIface),
		BRxInterfaceId: stringPtr(bIface),
	}

	if aNode != "" && aIface != "" {
		p.A = &resources.LinkEnd{
			Id: &common.NetworkInterfaceId{
				NodeId:      &aNode,
				InterfaceId: &aIface,
			},
		}
	}
	if bNode != "" && bIface != "" {
		p.B = &resources.LinkEnd{
			Id: &common.NetworkInterfaceId{
				NodeId:      &bNode,
				InterfaceId: &bIface,
			},
		}
	}

	// If we have a second directional link, try to infer distinct Tx/Rx.
	if len(filtered) > 1 {
		for _, l := range filtered[1:] {
			if l == nil {
				continue
			}
			srcNode, srcIface := splitInterfaceRef(l.InterfaceA)
			dstNode, dstIface := splitInterfaceRef(l.InterfaceB)

			switch {
			case srcNode == bNode && dstNode == aNode:
				// B → A direction.
				p.BTxInterfaceId = stringPtr(srcIface)
				p.ARxInterfaceId = stringPtr(dstIface)
			case srcNode == aNode && dstNode == bNode:
				// A → B direction.
				p.ATxInterfaceId = stringPtr(srcIface)
				p.BRxInterfaceId = stringPtr(dstIface)
			}
		}
	}

	return p
}

// ServiceRequestFromProto converts an Aalyria ServiceRequest into the
// simulator's domain ServiceRequest representation.
func ServiceRequestFromProto(sr *ServiceRequest) (*model.ServiceRequest, error) {
	if sr == nil {
		return nil, errors.New("nil ServiceRequest proto")
	}

	dom := &model.ServiceRequest{
		ID:                    "", // ID is intentionally NOT derived from the proto
		SrcNodeID:             sr.GetSrcNodeId(),
		DstNodeID:             sr.GetDstNodeId(),
		Priority:              int32(sr.GetPriority()),
		AllowPartnerResources: sr.GetAllowPartnerResources(),
	}

	for _, req := range sr.GetRequirements() {
		if req == nil {
			continue
		}

		fr := model.FlowRequirement{
			RequestedBandwidth: req.GetBandwidthBpsRequested(),
			MinBandwidth:       req.GetBandwidthBpsMinimum(),
			MaxLatency:         durationToSeconds(req.GetLatencyMaximum()),
		}

		if ti := req.GetTimeInterval(); ti != nil {
			fr.ValidFrom = dateTimeToTime(ti.GetStartTime())
			fr.ValidTo = dateTimeToTime(ti.GetEndTime())
		}

		if req.GetIsDisruptionTolerant() {
			dom.IsDisruptionTolerant = true
		}

		dom.FlowRequirements = append(dom.FlowRequirements, fr)
	}

	return dom, nil
}

// ServiceRequestToProto converts a domain ServiceRequest back into the
// Aalyria ServiceRequest proto.
//
// The internal ID is not exposed on the wire here; it is used by NBI
// request messages (request_id) and ScenarioState storage.
func ServiceRequestToProto(sr *model.ServiceRequest) *ServiceRequest {
	if sr == nil {
		return nil
	}

	p := &resources.ServiceRequest{}

	if sr.SrcNodeID != "" {
		src := sr.SrcNodeID
		p.SrcType = &resources.ServiceRequest_SrcNodeId{SrcNodeId: src}
	}
	if sr.DstNodeID != "" {
		dst := sr.DstNodeID
		p.DstType = &resources.ServiceRequest_DstNodeId{DstNodeId: dst}
	}

	if sr.Priority != 0 {
		pr := float64(sr.Priority)
		p.Priority = &pr
	}

	for _, fr := range sr.FlowRequirements {
		req := &resources.ServiceRequest_FlowRequirements{}

		if fr.RequestedBandwidth != 0 {
			bps := fr.RequestedBandwidth
			req.BandwidthBpsRequested = &bps
		}
		if fr.MinBandwidth != 0 {
			min := fr.MinBandwidth
			req.BandwidthBpsMinimum = &min
		}
		if fr.MaxLatency != 0 {
			req.LatencyMaximum = secondsToDuration(fr.MaxLatency)
		}
		if !fr.ValidFrom.IsZero() || !fr.ValidTo.IsZero() {
			req.TimeInterval = &common.TimeInterval{
				StartTime: timeToDateTime(fr.ValidFrom),
				EndTime:   timeToDateTime(fr.ValidTo),
			}
		}
		if sr.IsDisruptionTolerant {
			dtn := sr.IsDisruptionTolerant
			req.IsDisruptionTolerant = &dtn
		}

		p.Requirements = append(p.Requirements, req)
	}

	if sr.AllowPartnerResources {
		apr := sr.AllowPartnerResources
		p.AllowPartnerResources = &apr
	}

	return p
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
		return nodeID + "/" + ifaceID
	}
}

func splitInterfaceRef(ref string) (string, string) {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", ref
}

func combineLinkID(a, b string) string {
	if a == "" && b == "" {
		return ""
	}
	return a + "<->" + b
}

// normalizeInterfaceRef ensures we have a "node/iface" form.
// If nodeID is empty, it will be inferred from ifaceID if possible.
func normalizeInterfaceRef(nodeID, ifaceID string) string {
	if nodeID == "" {
		if n, local := splitInterfaceRef(ifaceID); n != "" {
			nodeID = n
			ifaceID = local
		}
	}
	return combineInterfaceRef(nodeID, ifaceID)
}

// directionalLinkID produces a stable ID that relates both directions
// of a link via a shared base while still uniquely identifying the
// direction.
//
// Examples:
//
//	src="a/1", dst="b/2" → "a/1<->b/2|a/1->b/2"
func directionalLinkID(src, dst string) string {
	if src == "" && dst == "" {
		return ""
	}
	dir := src + "->" + dst
	base := combineLinkID(src, dst)
	if base == "" || base == dir {
		return dir
	}
	return base + "|" + dir
}

// newDirectionalLink constructs a directional NetworkLink between two
// interface IDs using a default Medium and link flags.
func newDirectionalLink(src, dst string) *core.NetworkLink {
	return &core.NetworkLink{
		ID:         directionalLinkID(src, dst),
		InterfaceA: src,
		InterfaceB: dst,
		Medium:     core.MediumWireless, // TODO: refine if/when NBI exposes link medium
		IsUp:       true,
		IsStatic:   true,
	}
}

// newBidirectionalLink constructs an undirected NetworkLink using a stable ID.
// For wireless links, IsUp and IsStatic will be set by validateLinks based on medium type.
func newBidirectionalLink(a, b string) *core.NetworkLink {
	return &core.NetworkLink{
		ID:         combineLinkID(a, b),
		InterfaceA: a,
		InterfaceB: b,
		Medium:     core.MediumWireless,
		IsUp:       false, // Will be set by validateLinks or connectivity engine
		IsStatic:   false, // Will be set by validateLinks based on medium
		// Status defaults to LinkStatusUnknown (0), which allows auto-activation
	}
}

type bidirectionalEndpoint struct {
	nodeID string
	txID   string
	rxID   string
}

func extractBidirectionalEndpoint(nodeID, txID, rxID string, end *resources.LinkEnd) bidirectionalEndpoint {
	if end != nil && end.Id != nil {
		if nodeID == "" {
			nodeID = end.Id.GetNodeId()
		}
		if txID == "" {
			txID = end.Id.GetInterfaceId()
		}
		if rxID == "" {
			rxID = end.Id.GetInterfaceId()
		}
	}

	return bidirectionalEndpoint{
		nodeID: nodeID,
		txID:   txID,
		rxID:   rxID,
	}
}

func (e bidirectionalEndpoint) txInterface() string {
	if e.txID != "" {
		return e.txID
	}
	return e.rxID
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func durationToSeconds(d *durationpb.Duration) float64 {
	if d == nil {
		return 0
	}
	return d.AsDuration().Seconds()
}

func secondsToDuration(sec float64) *durationpb.Duration {
	if sec == 0 {
		return nil
	}
	return durationpb.New(time.Duration(sec * float64(time.Second)))
}

func dateTimeToTime(dt *common.DateTime) time.Time {
	if dt == nil {
		return time.Time{}
	}
	return time.UnixMicro(dt.GetUnixTimeUsec())
}

func timeToDateTime(t time.Time) *common.DateTime {
	if t.IsZero() {
		return nil
	}
	usec := t.UnixMicro()
	return &common.DateTime{
		UnixTimeUsec: &usec,
	}
}
