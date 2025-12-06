// internal/nbi/node_service.go
package nbi

import (
	"context"
	"errors"
	"fmt"

	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
	"github.com/signalsfoundry/constellation-simulator/internal/nbi/types"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// NetworkNodeService implements the NetworkNodeService gRPC server backed by a
// ScenarioState instance.
type NetworkNodeService struct {
	v1alpha.UnimplementedNetworkNodeServiceServer

	state *sim.ScenarioState
	log   Logger
}

// NewNetworkNodeService constructs a NetworkNodeService bound to ScenarioState.
func NewNetworkNodeService(state *sim.ScenarioState, log Logger) *NetworkNodeService {
	return &NetworkNodeService{
		state: state,
		log:   log,
	}
}

// CreateNode stores a new node and its interfaces.
func (s *NetworkNodeService) CreateNode(
	ctx context.Context,
	in *resources.NetworkNode,
) (*resources.NetworkNode, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if err := ValidateNodeProto(in); err != nil {
		return nil, ToStatusError(err)
	}

	node, interfaces, err := types.NodeWithInterfacesFromProto(in)
	if err != nil {
		return nil, ToStatusError(fmt.Errorf("%w: %v", ErrInvalidEntity, err))
	}

	// Derive a single platform_id from the interfaces, if present.
	if platformID, err := platformIDFromInterfaces(in); err != nil {
		return nil, ToStatusError(fmt.Errorf("%w: %v", ErrInvalidEntity, err))
	} else if platformID != "" {
		node.PlatformID = platformID
	}

	if err := s.state.CreateNode(node, interfaces); err != nil {
		if errors.Is(err, sim.ErrPlatformNotFound) {
			// Missing references during creation are treated as invalid input.
			return nil, ToStatusError(fmt.Errorf("%w: %v", ErrInvalidEntity, err))
		}
		return nil, ToStatusError(err)
	}

	return types.NodeToProtoWithInterfaces(node, interfaces), nil
}

// GetNode retrieves a node by ID.
func (s *NetworkNodeService) GetNode(
	ctx context.Context,
	req *v1alpha.GetNodeRequest,
) (*resources.NetworkNode, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if req == nil || req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}

	node, ifaces, err := s.state.GetNode(req.GetNodeId())
	if err != nil {
		return nil, ToStatusError(err)
	}

	return types.NodeToProtoWithInterfaces(node, ifaces), nil
}

// ListNodes returns all nodes with their interfaces.
func (s *NetworkNodeService) ListNodes(
	ctx context.Context,
	_ *v1alpha.ListNodesRequest,
) (*v1alpha.ListNodesResponse, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	resp := &v1alpha.ListNodesResponse{}
	for _, node := range s.state.ListNodes() {
		ifaces := s.state.ListInterfacesForNode(node.ID)
		resp.Nodes = append(resp.Nodes, types.NodeToProtoWithInterfaces(node, ifaces))
	}
	return resp, nil
}

// UpdateNode replaces a node definition and its interfaces.
func (s *NetworkNodeService) UpdateNode(
	ctx context.Context,
	req *v1alpha.UpdateNodeRequest,
) (*resources.NetworkNode, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if req == nil || req.GetNode() == nil {
		return nil, status.Error(codes.InvalidArgument, "node is required")
	}

	if err := ValidateNodeProto(req.GetNode()); err != nil {
		return nil, ToStatusError(err)
	}

	node, interfaces, err := types.NodeWithInterfacesFromProto(req.GetNode())
	if err != nil {
		return nil, ToStatusError(fmt.Errorf("%w: %v", ErrInvalidEntity, err))
	}

	// Re-derive platform_id from interfaces on update.
	if platformID, err := platformIDFromInterfaces(req.GetNode()); err != nil {
		return nil, ToStatusError(fmt.Errorf("%w: %v", ErrInvalidEntity, err))
	} else if platformID != "" {
		node.PlatformID = platformID
	}

	if err := s.state.UpdateNode(node, interfaces); err != nil {
		if errors.Is(err, sim.ErrPlatformNotFound) {
			return nil, ToStatusError(fmt.Errorf("%w: %v", ErrInvalidEntity, err))
		}
		return nil, ToStatusError(err)
	}

	return types.NodeToProtoWithInterfaces(node, interfaces), nil
}

// DeleteNode removes a node by ID, refusing to delete nodes that are still
// referenced by links or service requests.
func (s *NetworkNodeService) DeleteNode(
	ctx context.Context,
	req *v1alpha.DeleteNodeRequest,
) (*emptypb.Empty, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if req == nil || req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}

	if err := s.state.DeleteNode(req.GetNodeId()); err != nil {
		return nil, ToStatusError(err)
	}

	return &emptypb.Empty{}, nil
}

func (s *NetworkNodeService) ensureReady() error {
	if s == nil || s.state == nil {
		return status.Error(codes.FailedPrecondition, "scenario state is not configured")
	}
	return nil
}
