package types

import (
	"errors"
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

// InterfaceFromProto converts an Aalyria NetworkInterface into the
// simulator's core.NetworkInterface representation.
func InterfaceFromProto(iface *NetworkInterface) (*core.NetworkInterface, error) {
	if iface == nil {
		return nil, errors.New("nil NetworkInterface proto")
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
		ID:            iface.GetInterfaceId(),
		Name:          iface.GetName(),
		Medium:        medium,
		TransceiverID: transceiverID,
		ParentNodeID:  "",
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

	id := iface.ID
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

	intA := combineInterfaceRef(srcNode, srcIface)
	intB := combineInterfaceRef(dstNode, dstIface)

	return &core.NetworkLink{
		ID:         combineLinkID(intA, intB),
		InterfaceA: intA,
		InterfaceB: intB,
		Medium:     core.MediumWireless,
		IsUp:       true,
		IsStatic:   true,
	}, nil
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

// ServiceRequestFromProto converts an Aalyria ServiceRequest into the
// simulator's domain ServiceRequest representation.
//
// Note: The proto does not carry a stable request_id. We treat the proto
// `type` field as a human-readable label and map it onto ServiceRequest.Type.
// The stable ID is owned by the NBI / ScenarioState layer and is not set here.
func ServiceRequestFromProto(sr *ServiceRequest) (*model.ServiceRequest, error) {
    if sr == nil {
        return nil, errors.New("nil ServiceRequest proto")
    }

    dom := &model.ServiceRequest{
        ID:                    "",              // ID is intentionally NOT derived from proto.type
        Type:                  sr.GetType(),    // type label from proto
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
            RequestedBandwidthMbps: req.GetBandwidthBpsRequested() / 1e6,
            MinBandwidthMbps:       req.GetBandwidthBpsMinimum() / 1e6,
            MaxLatencyMs:           durationToMilliseconds(req.GetLatencyMaximum()),
        }

        if ti := req.GetTimeInterval(); ti != nil {
            fr.ValidFromUnixSec = dateTimeToUnixSeconds(ti.GetStartTime())
            fr.ValidToUnixSec = dateTimeToUnixSeconds(ti.GetEndTime())
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
// We emit the human-readable Type label into proto.type.
// The internal ID is not exposed on the wire here; it is used by NBI
// request messages (request_id) and ScenarioState storage.
func ServiceRequestToProto(sr *model.ServiceRequest) *ServiceRequest {
    if sr == nil {
        return nil
    }

    p := &resources.ServiceRequest{}

    if sr.Type != "" {
        typ := sr.Type
        p.Type = &typ
    }

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

        if fr.RequestedBandwidthMbps != 0 {
            bps := fr.RequestedBandwidthMbps * 1e6
            req.BandwidthBpsRequested = &bps
        }
        if fr.MinBandwidthMbps != 0 {
            min := fr.MinBandwidthMbps * 1e6
            req.BandwidthBpsMinimum = &min
        }
        if fr.MaxLatencyMs != 0 {
            req.LatencyMaximum = millisecondsToDuration(fr.MaxLatencyMs)
        }
        if fr.ValidFromUnixSec != 0 || fr.ValidToUnixSec != 0 {
            req.TimeInterval = &common.TimeInterval{
                StartTime: unixSecondsToDateTime(fr.ValidFromUnixSec),
                EndTime:   unixSecondsToDateTime(fr.ValidToUnixSec),
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

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func durationToMilliseconds(d *durationpb.Duration) float64 {
	if d == nil {
		return 0
	}
	return float64(d.AsDuration()) / float64(time.Millisecond)
}

func millisecondsToDuration(ms float64) *durationpb.Duration {
	if ms == 0 {
		return nil
	}
	return durationpb.New(time.Duration(ms * float64(time.Millisecond)))
}

func dateTimeToUnixSeconds(dt *common.DateTime) int64 {
	if dt == nil {
		return 0
	}
	return dt.GetUnixTimeUsec() / 1_000_000
}

func unixSecondsToDateTime(sec int64) *common.DateTime {
	if sec == 0 {
		return nil
	}
	usec := sec * 1_000_000
	return &common.DateTime{
		UnixTimeUsec: &usec,
	}
}
