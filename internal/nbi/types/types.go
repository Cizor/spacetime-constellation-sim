package types

import (
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
// Mapping function signatures.
//
// These will be fully implemented in later Scope-3 chunks, but we
// declare them now so the rest of the NBI layer can depend on them.
//

// PlatformFromProto converts an Aalyria PlatformDefinition into the
// simulator's domain model representation.
func PlatformFromProto(pd *PlatformDefinition) (*model.PlatformDefinition, error) {
	// TODO: implement in Scope 3 mapping chunk.
	panic("PlatformFromProto not implemented yet")
}

// PlatformToProto converts a domain PlatformDefinition back into the
// Aalyria proto form.
func PlatformToProto(m *model.PlatformDefinition) *PlatformDefinition {
	// TODO: implement in Scope 3 mapping chunk.
	panic("PlatformToProto not implemented yet")
}

// NodeFromProto converts an Aalyria NetworkNode into the simulator's
// domain NetworkNode representation.
func NodeFromProto(n *NetworkNode) (*model.NetworkNode, error) {
	// TODO: implement in Scope 3 mapping chunk.
	panic("NodeFromProto not implemented yet")
}

// NodeToProto converts a domain NetworkNode back into the Aalyria
// NetworkNode proto.
func NodeToProto(n *model.NetworkNode) *NetworkNode {
	// TODO: implement in Scope 3 mapping chunk.
	panic("NodeToProto not implemented yet")
}

// InterfaceFromProto converts an Aalyria NetworkInterface into the
// simulator's core.NetworkInterface representation.
func InterfaceFromProto(iface *NetworkInterface) (*core.NetworkInterface, error) {
	// TODO: implement in Scope 3 mapping chunk.
	panic("InterfaceFromProto not implemented yet")
}

// InterfaceToProto converts a core.NetworkInterface back into the
// Aalyria NetworkInterface proto.
func InterfaceToProto(iface *core.NetworkInterface) *NetworkInterface {
	// TODO: implement in Scope 3 mapping chunk.
	panic("InterfaceToProto not implemented yet")
}

// LinkFromProto converts an Aalyria NetworkLink into the simulator's
// core.NetworkLink representation.
func LinkFromProto(link *NetworkLink) (*core.NetworkLink, error) {
	// TODO: implement in Scope 3 mapping chunk.
	panic("LinkFromProto not implemented yet")
}

// LinkToProto converts a core.NetworkLink back into the Aalyria
// NetworkLink proto.
func LinkToProto(link *core.NetworkLink) *NetworkLink {
	// TODO: implement in Scope 3 mapping chunk.
	panic("LinkToProto not implemented yet")
}

// ServiceRequestFromProto converts an Aalyria ServiceRequest into the
// simulator's domain ServiceRequest representation.
func ServiceRequestFromProto(sr *ServiceRequest) (*model.ServiceRequest, error) {
	// TODO: implement in Scope 3 mapping chunk.
	panic("ServiceRequestFromProto not implemented yet")
}

// ServiceRequestToProto converts a domain ServiceRequest back into the
// Aalyria ServiceRequest proto.
func ServiceRequestToProto(sr *model.ServiceRequest) *ServiceRequest {
	// TODO: implement in Scope 3 mapping chunk.
	panic("ServiceRequestToProto not implemented yet")
}
