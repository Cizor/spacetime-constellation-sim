// internal/nbi/link_service.go
package nbi

import (
	"context"
	"errors"
	"fmt"
	"sort"

	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
	core "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/nbi/types"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// NetworkLinkService implements the NetworkLinkService gRPC server backed by a
// ScenarioState instance.
type NetworkLinkService struct {
	v1alpha.UnimplementedNetworkLinkServiceServer

	state *sim.ScenarioState
	log   Logger
}

// NewNetworkLinkService constructs a NetworkLinkService bound to ScenarioState.
func NewNetworkLinkService(state *sim.ScenarioState, log Logger) *NetworkLinkService {
	return &NetworkLinkService{
		state: state,
		log:   log,
	}
}

// CreateLink stores a new bidirectional link.
//
// NBI surface uses BidirectionalLink as the primary link abstraction.
// Internally we represent this as two directional core.NetworkLink objects
// (A->B and B->A) where possible.
func (s *NetworkLinkService) CreateLink(
	ctx context.Context,
	in *resources.BidirectionalLink,
) (*resources.BidirectionalLink, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, status.Error(codes.InvalidArgument, "link is required")
	}

	links, err := types.BidirectionalLinkFromProto(in)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.validateLinks(links...); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := s.state.CreateLinks(links...); err != nil {
		switch {
		case errors.Is(err, core.ErrInterfaceMiss):
			// One or more endpoints did not exist in Scope-2 KB.
			return nil, status.Error(codes.InvalidArgument, err.Error())
		case errors.Is(err, core.ErrLinkExists):
			// A link with the same ID already exists.
			return nil, status.Error(codes.AlreadyExists, err.Error())
		default:
			if s.log != nil {
				s.log.Errorw("CreateLink failed", "err", err)
			}
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return types.BidirectionalLinkToProto(links...), nil
}

// GetLink retrieves a link by ID.
//
// We treat the provided ID as a directional link ID and, if a reverse
// partner link exists, reconstruct a BidirectionalLink from both.
func (s *NetworkLinkService) GetLink(
	ctx context.Context,
	req *v1alpha.GetLinkRequest,
) (*resources.BidirectionalLink, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if req == nil || req.GetLinkId() == "" {
		return nil, status.Error(codes.InvalidArgument, "link_id is required")
	}

	link, err := s.state.GetLink(req.GetLinkId())
	if err != nil {
		if errors.Is(err, sim.ErrLinkNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	partner := s.findPartnerLink(link)
	return types.BidirectionalLinkToProto(link, partner), nil
}

// ListLinks returns all links in bidirectional form.
//
// Directional core.NetworkLink entries are grouped into bidirectional
// pairs when possible; unpaired links are still surfaced as a single
// BidirectionalLink with only one direction populated.
func (s *NetworkLinkService) ListLinks(
	ctx context.Context,
	_ *v1alpha.ListLinksRequest,
) (*v1alpha.ListLinksResponse, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	resp := &v1alpha.ListLinksResponse{}
	grouped := groupBidirectionalLinks(s.state.ListLinks())
	resp.Links = append(resp.Links, grouped...)
	return resp, nil
}

// DeleteLink removes a link by ID.
//
// The ID is treated as the underlying core.NetworkLink ID; callers can
// delete a single direction of a bidirectional pair.
func (s *NetworkLinkService) DeleteLink(
	ctx context.Context,
	req *v1alpha.DeleteLinkRequest,
) (*emptypb.Empty, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if req == nil || req.GetLinkId() == "" {
		return nil, status.Error(codes.InvalidArgument, "link_id is required")
	}

	if err := s.state.DeleteLink(req.GetLinkId()); err != nil {
		if errors.Is(err, sim.ErrLinkNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &emptypb.Empty{}, nil
}

// ensureReady verifies the service has been constructed correctly.
func (s *NetworkLinkService) ensureReady() error {
	if s == nil || s.state == nil {
		return status.Error(codes.FailedPrecondition, "scenario state is not configured")
	}
	return nil
}

// validateLinks ensures endpoints exist and are compatible, annotating
// medium and static-ness for wired links.
//
// Semantics:
//   - Both endpoints must exist as interfaces in Scope-2 KB.
//   - Their parent nodes (if set) must exist in Scope-1 KB.
//   - Both endpoints must be either wired or wireless.
//   - Wired links are marked as static, always-up connections.
//   - Wireless links are left dynamic (MediumWireless).
func (s *NetworkLinkService) validateLinks(links ...*core.NetworkLink) error {
	if len(links) == 0 {
		return errors.New("no links provided")
	}

	phys := s.state.PhysicalKB()
	net := s.state.NetworkKB()

	for _, link := range links {
		if link == nil {
			return errors.New("link is nil")
		}
		if link.InterfaceA == "" || link.InterfaceB == "" {
			return errors.New("link endpoints are required")
		}

		ifA := net.GetNetworkInterface(link.InterfaceA)
		if ifA == nil {
			return fmt.Errorf("%w: %q", sim.ErrInterfaceNotFound, link.InterfaceA)
		}
		ifB := net.GetNetworkInterface(link.InterfaceB)
		if ifB == nil {
			return fmt.Errorf("%w: %q", sim.ErrInterfaceNotFound, link.InterfaceB)
		}

		// Ensure referenced parent nodes exist (if set).
		if ifA.ParentNodeID != "" && phys.GetNetworkNode(ifA.ParentNodeID) == nil {
			return fmt.Errorf("%w: %q", sim.ErrNodeNotFound, ifA.ParentNodeID)
		}
		if ifB.ParentNodeID != "" && phys.GetNetworkNode(ifB.ParentNodeID) == nil {
			return fmt.Errorf("%w: %q", sim.ErrNodeNotFound, ifB.ParentNodeID)
		}

		wired := ifA.Medium == core.MediumWired && ifB.Medium == core.MediumWired
		wireless := ifA.Medium == core.MediumWireless && ifB.Medium == core.MediumWireless

		switch {
		case wired:
			// Terrestrial / static fiber: always available.
			link.Medium = core.MediumWired
			link.IsStatic = true
			link.IsUp = true
		case wireless:
			// Dynamic wireless link: leave IsUp/IsStatic for connectivity engine.
			link.Medium = core.MediumWireless
		default:
			return fmt.Errorf(
				"link endpoints must both be wired or both wireless: %q (%s) <-> %q (%s)",
				link.InterfaceA, ifA.Medium, link.InterfaceB, ifB.Medium,
			)
		}
	}

	return nil
}

// findPartnerLink returns the opposite directional link if present.
//
// Given a link A->B, this scans Scope-2 KB for a link B->A and, if
// found, returns it so the pair can be surfaced as a BidirectionalLink.
func (s *NetworkLinkService) findPartnerLink(link *core.NetworkLink) *core.NetworkLink {
	if link == nil {
		return nil
	}

	for _, candidate := range s.state.ListLinks() {
		if candidate == nil || candidate.ID == link.ID {
			continue
		}
		if candidate.InterfaceA == link.InterfaceB && candidate.InterfaceB == link.InterfaceA {
			return candidate
		}
	}
	return nil
}

// groupBidirectionalLinks collapses directional links into BidirectionalLink
// protos. Any links that don't have a matching reverse-direction partner
// are still included as one-ended bidirectional links.
func groupBidirectionalLinks(links []*core.NetworkLink) []*resources.BidirectionalLink {
	pairs := make(map[string][]*core.NetworkLink)

	for _, link := range links {
		if link == nil {
			continue
		}
		key := bidirectionalKey(link.InterfaceA, link.InterfaceB)
		if key == "" {
			continue
		}
		pairs[key] = append(pairs[key], link)
	}

	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]*resources.BidirectionalLink, 0, len(pairs))
	for _, k := range keys {
		out = append(out, types.BidirectionalLinkToProto(pairs[k]...))
	}
	return out
}

// bidirectionalKey produces a stable, order-insensitive key for a link
// based on its interface endpoints.
func bidirectionalKey(a, b string) string {
	switch {
	case a == "" && b == "":
		return ""
	case a < b:
		return a + "|" + b
	default:
		return b + "|" + a
	}
}
