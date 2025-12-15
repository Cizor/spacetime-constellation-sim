package controller

import (
	"context"
	"fmt"

	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/model"
)

// FederatedPath represents a complete multi-domain path for a cross-domain ServiceRequest.
type FederatedPath struct {
	Segments   []PathSegment
	DomainHops []string // ordered list of domain IDs visited
}

// PathSegment describes a path that lies entirely within one domain.
type PathSegment struct {
	DomainID    string
	Path        *model.Path
	BorderNodes []string
}

// FindFederatedPath builds a stubbed federated path for cross-domain ServiceRequests.
// Within-domain routing is simplified to single-node hops, and between-domain coordination
// is implicit via the ordered DomainHops slice.
func FindFederatedPath(sr *model.ServiceRequest, state *state.ScenarioState) (*FederatedPath, error) {
	return FindFederatedPathWithClient(context.Background(), sr, state, NewInMemoryFederationClient(logging.Noop()))
}

// FindFederatedPathWithClient builds a stubbed federated path for cross-domain ServiceRequests
// using the provided federation client. It simplifies within-domain routing to single-node hops
// and delegates inter-domain segments to the federation surface.
func FindFederatedPathWithClient(ctx context.Context, sr *model.ServiceRequest, state *state.ScenarioState, client FederationClient) (*FederatedPath, error) {
	if sr == nil || state == nil {
		return nil, fmt.Errorf("service request or state is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		client = NewInMemoryFederationClient(logging.Noop())
	}
	if !sr.CrossDomain {
		domainID, err := state.GetDomainForNode(sr.SrcNodeID)
		if err != nil {
			return nil, fmt.Errorf("unable to resolve domain for node %q: %w", sr.SrcNodeID, err)
		}
		return &FederatedPath{
			Segments: []PathSegment{{
				DomainID:    domainID,
				Path:        pathForSingleNode(sr.SrcNodeID),
				BorderNodes: []string{sr.SrcNodeID},
			}},
			DomainHops: []string{domainID},
		}, nil
	}
	sourceDomain := sr.SourceDomain
	destDomain := sr.DestDomain
	if sourceDomain == "" || destDomain == "" {
		return nil, fmt.Errorf("cross-domain request missing domain assignments")
	}
	req := FederationRequest{
		RequestID:    sr.ID,
		SourceDomain: sourceDomain,
		DestDomain:   destDomain,
		Requirements: sr.FlowRequirements,
		Token:        sr.FederationToken,
	}
	resp, err := client.RequestPathSegment(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("federation request failed: %w", err)
	}
	if resp.Status != FederationStatusOK {
		return nil, fmt.Errorf("federation error: %s", resp.Error)
	}
	if resp.PathSegment == nil {
		return nil, fmt.Errorf("federation response missing path segment")
	}
	segments := []PathSegment{
		{
			DomainID:    sourceDomain,
			Path:        pathForSingleNode(sr.SrcNodeID),
			BorderNodes: []string{sr.SrcNodeID},
		},
		*resp.PathSegment,
	}
	domainHops := []string{sourceDomain, destDomain}
	return &FederatedPath{Segments: segments, DomainHops: domainHops}, nil
}

func pathForSingleNode(nodeID string) *model.Path {
	return &model.Path{Nodes: []string{nodeID}}
}
