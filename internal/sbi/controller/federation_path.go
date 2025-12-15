package controller

import (
	"fmt"

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
	if sr == nil || state == nil {
		return nil, fmt.Errorf("service request or state is nil")
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
	segments := []PathSegment{
		{
			DomainID:    sourceDomain,
			Path:        pathForSingleNode(sr.SrcNodeID),
			BorderNodes: []string{sr.SrcNodeID},
		},
		{
			DomainID:    destDomain,
			Path:        pathForSingleNode(sr.DstNodeID),
			BorderNodes: []string{sr.DstNodeID},
		},
	}
	return &FederatedPath{
		Segments:   segments,
		DomainHops: []string{sourceDomain, destDomain},
	}, nil
}

func pathForSingleNode(nodeID string) *model.Path {
	return &model.Path{Nodes: []string{nodeID}}
}
