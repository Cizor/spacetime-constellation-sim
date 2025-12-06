package nbi

import (
	"testing"
	"time"

	common "aalyria.com/spacetime/api/common"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestValidatePlatformProto(t *testing.T) {
	name := "platform-valid"
	typ := "SATELLITE"
	motion := common.PlatformDefinition_SPACETRACK_ORG

	valid := &common.PlatformDefinition{
		Name:         &name,
		Type:         &typ,
		MotionSource: &motion,
	}
	if err := ValidatePlatformProto(valid); err != nil {
		t.Fatalf("ValidatePlatformProto(valid) err = %v, want nil", err)
	}

	tests := []struct {
		name string
		pd   *common.PlatformDefinition
	}{
		{name: "nil", pd: nil},
		{name: "missing name", pd: &common.PlatformDefinition{Type: &typ}},
		{name: "missing type", pd: &common.PlatformDefinition{Name: &name}},
		{name: "satellite missing motion", pd: &common.PlatformDefinition{Name: &name, Type: &typ}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidatePlatformProto(tc.pd); err == nil {
				t.Fatalf("ValidatePlatformProto(%s) = nil, want error", tc.name)
			}
		})
	}
}

func TestValidateInterfaceProto(t *testing.T) {
	if err := ValidateInterfaceProto(wiredInterfaceProto("eth0", "plat")); err != nil {
		t.Fatalf("ValidateInterfaceProto(wired) err = %v, want nil", err)
	}
	if err := ValidateInterfaceProto(wirelessInterfaceProto("rf0", "trx", "plat")); err != nil {
		t.Fatalf("ValidateInterfaceProto(wireless) err = %v, want nil", err)
	}

	tests := []struct {
		name  string
		iface *resources.NetworkInterface
	}{
		{name: "nil", iface: nil},
		{
			name: "missing id",
			iface: &resources.NetworkInterface{
				InterfaceMedium: &resources.NetworkInterface_Wired{Wired: &resources.WiredDevice{}},
			},
		},
		{
			name:  "missing medium",
			iface: &resources.NetworkInterface{InterfaceId: strPtr("if0")},
		},
		{
			name:  "wireless missing transceiver",
			iface: wirelessInterfaceProto("rf1", "", "plat"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateInterfaceProto(tc.iface); err == nil {
				t.Fatalf("ValidateInterfaceProto(%s) = nil, want error", tc.name)
			}
		})
	}
}

func TestValidateNodeProto(t *testing.T) {
	valid := &resources.NetworkNode{
		NodeId: strPtr("node-1"),
		NodeInterface: []*resources.NetworkInterface{
			wiredInterfaceProto("eth0", "plat"),
			wirelessInterfaceProto("rf0", "trx", "plat"),
		},
	}
	if err := ValidateNodeProto(valid); err != nil {
		t.Fatalf("ValidateNodeProto(valid) err = %v, want nil", err)
	}

	tests := []struct {
		name string
		node *resources.NetworkNode
	}{
		{name: "nil", node: nil},
		{
			name: "missing node id",
			node: &resources.NetworkNode{
				NodeInterface: []*resources.NetworkInterface{wiredInterfaceProto("eth0", "plat")},
			},
		},
		{
			name: "no interfaces",
			node: &resources.NetworkNode{
				NodeId: strPtr("node-no-iface"),
			},
		},
		{
			name: "duplicate interfaces",
			node: &resources.NetworkNode{
				NodeId: strPtr("node-dup"),
				NodeInterface: []*resources.NetworkInterface{
					wiredInterfaceProto("dup", "plat"),
					wirelessInterfaceProto("dup", "trx", "plat"),
				},
			},
		},
		{
			name: "wireless missing transceiver",
			node: &resources.NetworkNode{
				NodeId: strPtr("node-missing-trx"),
				NodeInterface: []*resources.NetworkInterface{
					wirelessInterfaceProto("rf0", "", "plat"),
				},
			},
		},
		{
			name: "interface on different node",
			node: &resources.NetworkNode{
				NodeId: strPtr("node-wrong"),
				NodeInterface: []*resources.NetworkInterface{
					{
						InterfaceId:     strPtr("other/eth0"),
						InterfaceMedium: &resources.NetworkInterface_Wired{Wired: &resources.WiredDevice{}},
					},
				},
			},
		},
		{
			name: "conflicting platforms",
			node: &resources.NetworkNode{
				NodeId: strPtr("node-conflict"),
				NodeInterface: []*resources.NetworkInterface{
					wiredInterfaceProto("a", "plat-a"),
					wiredInterfaceProto("b", "plat-b"),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateNodeProto(tc.node); err == nil {
				t.Fatalf("ValidateNodeProto(%s) = nil, want error", tc.name)
			}
		})
	}
}

func TestValidateLinkProto(t *testing.T) {
	nodeA := "node-a"
	nodeB := "node-b"
	ifaceA := "ifA"
	ifaceB := "ifB"

	valid := &resources.BidirectionalLink{
		ANetworkNodeId: &nodeA,
		ATxInterfaceId: &ifaceA,
		ARxInterfaceId: &ifaceA,
		BNetworkNodeId: &nodeB,
		BTxInterfaceId: &ifaceB,
		BRxInterfaceId: &ifaceB,
	}
	if err := ValidateLinkProto(valid); err != nil {
		t.Fatalf("ValidateLinkProto(valid) err = %v, want nil", err)
	}

	tests := []struct {
		name string
		link *resources.BidirectionalLink
	}{
		{name: "nil", link: nil},
		{name: "missing endpoints", link: &resources.BidirectionalLink{}},
		{
			name: "self loop",
			link: &resources.BidirectionalLink{
				ANetworkNodeId: &nodeA,
				ATxInterfaceId: &ifaceA,
				ARxInterfaceId: &ifaceA,
				BNetworkNodeId: &nodeA,
				BTxInterfaceId: &ifaceA,
				BRxInterfaceId: &ifaceA,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateLinkProto(tc.link); err == nil {
				t.Fatalf("ValidateLinkProto(%s) = nil, want error", tc.name)
			}
		})
	}
}

func TestValidateServiceRequestProto(t *testing.T) {
	usecStart := time.Now().UnixMicro()
	usecEnd := usecStart + int64(time.Minute/time.Microsecond)
	bw := 1_000_000.0

	valid := &resources.ServiceRequest{
		Type:    strPtr("sr-valid"),
		SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "src"},
		DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "dst"},
		Requirements: []*resources.ServiceRequest_FlowRequirements{
			{
				BandwidthBpsRequested: &bw,
				LatencyMaximum:        durationpb.New(10 * time.Millisecond),
				TimeInterval: &common.TimeInterval{
					StartTime: &common.DateTime{UnixTimeUsec: &usecStart},
					EndTime:   &common.DateTime{UnixTimeUsec: &usecEnd},
				},
			},
		},
	}
	if err := ValidateServiceRequestProto(valid); err != nil {
		t.Fatalf("ValidateServiceRequestProto(valid) err = %v, want nil", err)
	}

	negative := -1.0
	earlyEnd := usecStart - int64(time.Minute/time.Microsecond)

	tests := []struct {
		name string
		req  *resources.ServiceRequest
	}{
		{name: "nil", req: nil},
		{
			name: "missing src",
			req: &resources.ServiceRequest{
				DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "dst"},
				Requirements: []*resources.ServiceRequest_FlowRequirements{
					{BandwidthBpsRequested: &bw},
				},
			},
		},
		{
			name: "missing dst",
			req: &resources.ServiceRequest{
				SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "src"},
				Requirements: []*resources.ServiceRequest_FlowRequirements{
					{BandwidthBpsRequested: &bw},
				},
			},
		},
		{
			name: "no requirements",
			req: &resources.ServiceRequest{
				SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "src"},
				DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "dst"},
			},
		},
		{
			name: "negative bandwidth",
			req: &resources.ServiceRequest{
				SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "src"},
				DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "dst"},
				Requirements: []*resources.ServiceRequest_FlowRequirements{
					{BandwidthBpsRequested: &negative},
				},
			},
		},
		{
			name: "negative latency",
			req: &resources.ServiceRequest{
				SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "src"},
				DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "dst"},
				Requirements: []*resources.ServiceRequest_FlowRequirements{
					{LatencyMaximum: durationpb.New(-1 * time.Second)},
				},
			},
		},
		{
			name: "end before start",
			req: &resources.ServiceRequest{
				SrcType: &resources.ServiceRequest_SrcNodeId{SrcNodeId: "src"},
				DstType: &resources.ServiceRequest_DstNodeId{DstNodeId: "dst"},
				Requirements: []*resources.ServiceRequest_FlowRequirements{
					{
						BandwidthBpsRequested: &bw,
						TimeInterval: &common.TimeInterval{
							StartTime: &common.DateTime{UnixTimeUsec: &usecStart},
							EndTime:   &common.DateTime{UnixTimeUsec: &earlyEnd},
						},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateServiceRequestProto(tc.req); err == nil {
				t.Fatalf("ValidateServiceRequestProto(%s) = nil, want error", tc.name)
			}
		})
	}
}
