//go:build perf || perf_large

package perf

import (
	"context"
	"fmt"
	"testing"

	common "aalyria.com/spacetime/api/common"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
	core "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/nbi"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
)

type perfConfig struct {
	Platforms         int
	Nodes             int
	InterfacesPerNode int
	Links             int
	ServiceRequests   int
}

func benchmarkPlatforms(b *testing.B, cfg perfConfig) {
	ctx := context.Background()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		state := newScenarioState()
		svc := nbi.NewPlatformService(state, nil, logging.Noop())

		b.ResetTimer()
		for j := 0; j < cfg.Platforms; j++ {
			id := fmt.Sprintf("platform-%d-%d", i, j)
			typ := "GROUND_STATION"
			if _, err := svc.CreatePlatform(ctx, &common.PlatformDefinition{
				Name: &id,
				Type: &typ,
			}); err != nil {
				b.Fatalf("CreatePlatform(%s): %v", id, err)
			}
		}
		b.StopTimer()
	}
}

func benchmarkNodes(b *testing.B, cfg perfConfig) {
	ctx := context.Background()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		state := newScenarioState()
		svc := nbi.NewNetworkNodeService(state, logging.Noop())

		b.ResetTimer()
		for j := 0; j < cfg.Nodes; j++ {
			nodeID := fmt.Sprintf("node-%d-%d", i, j)
			if _, err := svc.CreateNode(ctx, nodeProto(nodeID, cfg.InterfacesPerNode)); err != nil {
				b.Fatalf("CreateNode(%s): %v", nodeID, err)
			}
		}
		b.StopTimer()
	}
}

func benchmarkLinks(b *testing.B, cfg perfConfig) {
	ctx := context.Background()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		state := newScenarioState()
		nodeSvc := nbi.NewNetworkNodeService(state, logging.Noop())
		linkSvc := nbi.NewNetworkLinkService(state, logging.Noop())

		nodeIDs := make([]string, 0, max(cfg.Nodes, cfg.Links+1))
		for j := 0; j < cap(nodeIDs); j++ {
			nodeID := fmt.Sprintf("link-node-%d-%d", i, j)
			if _, err := nodeSvc.CreateNode(ctx, nodeProto(nodeID, 1)); err != nil {
				b.Fatalf("seed CreateNode(%s): %v", nodeID, err)
			}
			nodeIDs = append(nodeIDs, nodeID)
		}

		b.ResetTimer()
		for j := 0; j < cfg.Links; j++ {
			a := nodeIDs[j%len(nodeIDs)]
			bID := nodeIDs[(j+1)%len(nodeIDs)]
			if a == bID {
				b.Fatalf("node ID reuse detected")
			}
			if _, err := linkSvc.CreateLink(ctx, linkProto(a, bID)); err != nil {
				b.Fatalf("CreateLink(%s,%s): %v", a, bID, err)
			}
		}
		b.StopTimer()
	}
}

func benchmarkServiceRequests(b *testing.B, cfg perfConfig) {
	ctx := context.Background()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		state := newScenarioState()
		nodeSvc := nbi.NewNetworkNodeService(state, logging.Noop())
		srSvc := nbi.NewServiceRequestService(state, logging.Noop())

		nodeIDs := make([]string, 0, max(cfg.Nodes, cfg.ServiceRequests+1))
		for j := 0; j < cap(nodeIDs); j++ {
			nodeID := fmt.Sprintf("sr-node-%d-%d", i, j)
			if _, err := nodeSvc.CreateNode(ctx, nodeProto(nodeID, 1)); err != nil {
				b.Fatalf("seed CreateNode(%s): %v", nodeID, err)
			}
			nodeIDs = append(nodeIDs, nodeID)
		}

		b.ResetTimer()
		for j := 0; j < cfg.ServiceRequests; j++ {
			src := nodeIDs[j%len(nodeIDs)]
			dst := nodeIDs[(j+1)%len(nodeIDs)]
			if _, err := srSvc.CreateServiceRequest(ctx, serviceRequestProto(src, dst, j%5)); err != nil {
				b.Fatalf("CreateServiceRequest(%s->%s): %v", src, dst, err)
			}
		}
		b.StopTimer()
	}
}

func newScenarioState() *sim.ScenarioState {
	return sim.NewScenarioState(kb.NewKnowledgeBase(), core.NewKnowledgeBase(), logging.Noop())
}

func nodeProto(nodeID string, interfaces int) *resources.NetworkNode {
	n := &resources.NetworkNode{
		NodeId: stringPtr(nodeID),
		Name:   stringPtr(nodeID),
	}
	for i := 0; i < interfaces; i++ {
		ifaceID := fmt.Sprintf("if%d", i)
		n.NodeInterface = append(n.NodeInterface, &resources.NetworkInterface{
			InterfaceId: stringPtr(ifaceID),
			InterfaceMedium: &resources.NetworkInterface_Wired{
				Wired: &resources.WiredDevice{},
			},
		})
	}
	return n
}

func linkProto(aNode, bNode string) *resources.BidirectionalLink {
	iface := "if0"
	return &resources.BidirectionalLink{
		ANetworkNodeId: stringPtr(aNode),
		ATxInterfaceId: stringPtr(iface),
		ARxInterfaceId: stringPtr(iface),
		BNetworkNodeId: stringPtr(bNode),
		BTxInterfaceId: stringPtr(iface),
		BRxInterfaceId: stringPtr(iface),
		// Populate deprecated fields to keep coverage of both formats.
		A: &resources.LinkEnd{
			Id: &common.NetworkInterfaceId{
				NodeId:      stringPtr(aNode),
				InterfaceId: stringPtr(iface),
			},
		},
		B: &resources.LinkEnd{
			Id: &common.NetworkInterfaceId{
				NodeId:      stringPtr(bNode),
				InterfaceId: stringPtr(iface),
			},
		},
	}
}

func serviceRequestProto(src, dst string, priority int) *resources.ServiceRequest {
	pr := float64(priority)
	bw := float64(10_000_000)
	minBw := float64(5_000_000)
	id := fmt.Sprintf("sr-%s-%s-%d", src, dst, priority)

	return &resources.ServiceRequest{
		Type:     stringPtr(id),
		SrcType:  &resources.ServiceRequest_SrcNodeId{SrcNodeId: src},
		DstType:  &resources.ServiceRequest_DstNodeId{DstNodeId: dst},
		Priority: &pr,
		Requirements: []*resources.ServiceRequest_FlowRequirements{
			{
				BandwidthBpsRequested: &bw,
				BandwidthBpsMinimum:   &minBw,
			},
		},
	}
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
