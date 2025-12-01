package types

import (
	"math"
	"testing"

	core "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/model"

	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
)

func TestInterfaceMappingRoundTrip(t *testing.T) {
	orig := &core.NetworkInterface{
		ID:            "if-1",
		Name:          "Wireless0",
		Medium:        core.MediumWireless,
		TransceiverID: "trx-123",
		IsOperational: false,
		MACAddress:    "01:23:45:67:89:ab",
		IPAddress:     "10.0.0.1/24",
	}

	p := InterfaceToProto(orig)
	if p == nil {
		t.Fatalf("InterfaceToProto returned nil")
	}

	back, err := InterfaceFromProto(p)
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
	if dom.ID != "node-a/if-a<->node-b/if-b" {
		t.Errorf("ID mismatch: got %q", dom.ID)
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
        Type:                  "sr-type",
        SrcNodeID:             "node-1",
        DstNodeID:             "node-2",
        Priority:              3,
        IsDisruptionTolerant:  true,
        AllowPartnerResources: true,
        FlowRequirements: []model.FlowRequirement{
            {
                RequestedBandwidthMbps: 150,
                MinBandwidthMbps:       50,
                MaxLatencyMs:           25,
                ValidFromUnixSec:       1_000,
                ValidToUnixSec:         2_000,
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

    // Type should roundtrip through the proto.
    if back.Type != orig.Type {
        t.Errorf("Type mismatch: got %q, want %q", back.Type, orig.Type)
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

    if diff := math.Abs(got.RequestedBandwidthMbps - want.RequestedBandwidthMbps); diff > 1e-6 {
        t.Errorf("RequestedBandwidth mismatch: got %f, want %f", got.RequestedBandwidthMbps, want.RequestedBandwidthMbps)
    }
    if diff := math.Abs(got.MinBandwidthMbps - want.MinBandwidthMbps); diff > 1e-6 {
        t.Errorf("MinBandwidth mismatch: got %f, want %f", got.MinBandwidthMbps, want.MinBandwidthMbps)
    }
    if diff := math.Abs(got.MaxLatencyMs - want.MaxLatencyMs); diff > 1e-6 {
        t.Errorf("MaxLatencyMs mismatch: got %f, want %f", got.MaxLatencyMs, want.MaxLatencyMs)
    }
    if got.ValidFromUnixSec != want.ValidFromUnixSec {
        t.Errorf("ValidFromUnixSec mismatch: got %d, want %d", got.ValidFromUnixSec, want.ValidFromUnixSec)
    }
    if got.ValidToUnixSec != want.ValidToUnixSec {
        t.Errorf("ValidToUnixSec mismatch: got %d, want %d", got.ValidToUnixSec, want.ValidToUnixSec)
    }
}
