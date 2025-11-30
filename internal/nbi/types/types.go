package types

import (
	"errors"

	core "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/model"

	common "aalyria.com/spacetime/api/common"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
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
	// TODO: implement in later Scope 3 chunk.
	panic("InterfaceFromProto not implemented yet")
}

// InterfaceToProto converts a core.NetworkInterface back into the
// Aalyria NetworkInterface proto.
func InterfaceToProto(iface *core.NetworkInterface) *NetworkInterface {
	// TODO: implement in later Scope 3 chunk.
	panic("InterfaceToProto not implemented yet")
}

// LinkFromProto converts an Aalyria NetworkLink into the simulator's
// core.NetworkLink representation.
func LinkFromProto(link *NetworkLink) (*core.NetworkLink, error) {
	// TODO: implement in later Scope 3 chunk.
	panic("LinkFromProto not implemented yet")
}

// LinkToProto converts a core.NetworkLink back into the Aalyria
// NetworkLink proto.
func LinkToProto(link *core.NetworkLink) *NetworkLink {
	// TODO: implement in later Scope 3 chunk.
	panic("LinkToProto not implemented yet")
}

// ServiceRequestFromProto converts an Aalyria ServiceRequest into the
// simulator's domain ServiceRequest representation.
func ServiceRequestFromProto(sr *ServiceRequest) (*model.ServiceRequest, error) {
	// TODO: implement in later Scope 3 chunk.
	panic("ServiceRequestFromProto not implemented yet")
}

// ServiceRequestToProto converts a domain ServiceRequest back into the
// Aalyria ServiceRequest proto.
func ServiceRequestToProto(sr *model.ServiceRequest) *ServiceRequest {
	// TODO: implement in later Scope 3 chunk.
	panic("ServiceRequestToProto not implemented yet")
}
