package types

import (
	"testing"

	core "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/model"

	common "aalyria.com/spacetime/api/common"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
)

func TestPlatformMappingRoundTrip_Domain(t *testing.T) {
	orig := &model.PlatformDefinition{
		ID:           "ISS",
		Name:         "ISS",
		Type:         "SATELLITE",
		CategoryTag:  "demo-leo",
		NoradID:      25544,
		MotionSource: model.MotionSourceSpacetrack,
		Coordinates: model.Motion{
			X: 1234.5,
			Y: -9876.5,
			Z: 42.0,
		},
	}

	p := PlatformToProto(orig)
	if p == nil {
		t.Fatalf("PlatformToProto returned nil")
	}

	back, err := PlatformFromProto(p)
	if err != nil {
		t.Fatalf("PlatformFromProto returned error: %v", err)
	}

	if back.ID != orig.ID {
		t.Errorf("ID mismatch: got %q, want %q", back.ID, orig.ID)
	}
	if back.Name != orig.Name {
		t.Errorf("Name mismatch: got %q, want %q", back.Name, orig.Name)
	}
	if back.Type != orig.Type {
		t.Errorf("Type mismatch: got %q, want %q", back.Type, orig.Type)
	}
	if back.CategoryTag != orig.CategoryTag {
		t.Errorf("CategoryTag mismatch: got %q, want %q", back.CategoryTag, orig.CategoryTag)
	}
	if back.NoradID != orig.NoradID {
		t.Errorf("NoradID mismatch: got %d, want %d", back.NoradID, orig.NoradID)
	}
	if back.MotionSource != orig.MotionSource {
		t.Errorf("MotionSource mismatch: got %v, want %v", back.MotionSource, orig.MotionSource)
	}

	if back.Coordinates.X != orig.Coordinates.X ||
		back.Coordinates.Y != orig.Coordinates.Y ||
		back.Coordinates.Z != orig.Coordinates.Z {
		t.Errorf("Coordinates mismatch: got (%f,%f,%f), want (%f,%f,%f)",
			back.Coordinates.X, back.Coordinates.Y, back.Coordinates.Z,
			orig.Coordinates.X, orig.Coordinates.Y, orig.Coordinates.Z)
	}
}

func TestNetworkNodeMappingRoundTrip_Domain(t *testing.T) {
	orig := &model.NetworkNode{
		ID:         "node-1",
		Name:       "gw-1",
		Type:       "GROUND",
		PlatformID: "platform-1",
	}

	p := NodeToProto(orig)
	if p == nil {
		t.Fatalf("NodeToProto returned nil")
	}

	back, err := NodeFromProto(p)
	if err != nil {
		t.Fatalf("NodeFromProto returned error: %v", err)
	}

	if back.ID != orig.ID {
		t.Errorf("ID mismatch: got %q, want %q", back.ID, orig.ID)
	}
	if back.Name != orig.Name {
		t.Errorf("Name mismatch: got %q, want %q", back.Name, orig.Name)
	}
	if back.Type != orig.Type {
		t.Errorf("Type mismatch: got %q, want %q", back.Type, orig.Type)
	}

	// PlatformID is not encoded in the proto, so we expect it to be empty
	// when we come back from NodeFromProto.
	if back.PlatformID != "" {
		t.Errorf("expected PlatformID to be empty after roundtrip, got %q", back.PlatformID)
	}
}

func TestNodeWithInterfacesFromProto(t *testing.T) {
	nodeID := "node-42"
	name := "router-42"
	typ := "ROUTER"

	ifIDWired := "eth0"
	ifIDWireless := "rf0"
	mac := "aa:bb:cc:dd:ee:ff"
	ip := "192.0.2.1/24"
	trx := "trx-1"
	unusable := resources.NetworkInterface_Impairment_DEFAULT_UNUSABLE

	p := &NetworkNode{
		NodeId: &nodeID,
		Name:   &name,
		Type:   &typ,
		NodeInterface: []*NetworkInterface{
			{
				InterfaceId: &ifIDWired,
				Name:        stringPtr("wired-primary"),
				InterfaceMedium: &resources.NetworkInterface_Wired{
					Wired: &resources.WiredDevice{
						PlatformId: stringPtr("platform-ignored-here"),
					},
				},
				EthernetAddress: &mac,
				IpAddress:       &ip,
			},
			{
				InterfaceId: &ifIDWireless,
				Name:        stringPtr("wireless-primary"),
				InterfaceMedium: &resources.NetworkInterface_Wireless{
					Wireless: &resources.WirelessDevice{
						TransceiverModelId: &common.TransceiverModelId{
							TransceiverModelId: &trx,
						},
					},
				},
				OperationalImpairment: []*resources.NetworkInterface_Impairment{
					{Type: &unusable},
				},
			},
		},
	}

	node, ifaces, err := NodeWithInterfacesFromProto(p)
	if err != nil {
		t.Fatalf("NodeWithInterfacesFromProto returned error: %v", err)
	}

	if node.ID != nodeID || node.Name != name || node.Type != typ {
		t.Fatalf("unexpected node mapping: %+v", node)
	}

	if len(ifaces) != 2 {
		t.Fatalf("expected 2 interfaces, got %d", len(ifaces))
	}

	got := make(map[string]*core.NetworkInterface)
	for _, iface := range ifaces {
		got[iface.ID] = iface
	}

	wired, ok := got["node-42/"+ifIDWired]
	if !ok {
		t.Fatalf("wired interface not found in mapped slice: %+v", got)
	}
	if wired.ParentNodeID != nodeID {
		t.Errorf("wired ParentNodeID = %q, want %q", wired.ParentNodeID, nodeID)
	}
	if wired.Medium != core.MediumWired {
		t.Errorf("wired Medium = %q, want %q", wired.Medium, core.MediumWired)
	}
	if wired.MACAddress != mac {
		t.Errorf("wired MACAddress = %q, want %q", wired.MACAddress, mac)
	}
	if wired.IPAddress != ip {
		t.Errorf("wired IPAddress = %q, want %q", wired.IPAddress, ip)
	}
	if wired.Name != "wired-primary" {
		t.Errorf("wired Name = %q, want wired-primary", wired.Name)
	}
	if !wired.IsOperational {
		t.Errorf("wired IsOperational = false, want true")
	}

	wireless, ok := got["node-42/"+ifIDWireless]
	if !ok {
		t.Fatalf("wireless interface not found in mapped slice: %+v", got)
	}
	if wireless.ParentNodeID != nodeID {
		t.Errorf("wireless ParentNodeID = %q, want %q", wireless.ParentNodeID, nodeID)
	}
	if wireless.Medium != core.MediumWireless {
		t.Errorf("wireless Medium = %q, want %q", wireless.Medium, core.MediumWireless)
	}
	if wireless.TransceiverID != trx {
		t.Errorf("wireless TransceiverID = %q, want %q", wireless.TransceiverID, trx)
	}
	if wireless.IsOperational {
		t.Errorf("wireless IsOperational = true, want false due to impairment")
	}
}

func TestNodeWithInterfacesRoundTrip(t *testing.T) {
	node := &model.NetworkNode{
		ID:   "node-99",
		Name: "node-99-name",
		Type: "GROUND",
	}

	ifaceA := &core.NetworkInterface{
		ID:            "node-99/if-a",
		Name:          "if-a",
		Medium:        core.MediumWired,
		ParentNodeID:  "node-99",
		MACAddress:    "00:11:22:33:44:55",
		IPAddress:     "10.1.0.1/24",
		IsOperational: true,
	}
	ifaceB := &core.NetworkInterface{
		ID:            "node-99/if-b",
		Name:          "if-b",
		Medium:        core.MediumWireless,
		ParentNodeID:  "node-99",
		TransceiverID: "trx-55",
		IsOperational: false,
	}

	p := NodeToProtoWithInterfaces(node, []*core.NetworkInterface{ifaceA, ifaceB})
	if p == nil {
		t.Fatalf("NodeToProtoWithInterfaces returned nil")
	}

	backNode, backIfaces, err := NodeWithInterfacesFromProto(p)
	if err != nil {
		t.Fatalf("NodeWithInterfacesFromProto returned error: %v", err)
	}

	if backNode.ID != node.ID || backNode.Name != node.Name || backNode.Type != node.Type {
		t.Fatalf("unexpected node mapping: %+v", backNode)
	}

	if len(backIfaces) != 2 {
		t.Fatalf("expected 2 interfaces after roundtrip, got %d", len(backIfaces))
	}

	roundtripped := make(map[string]*core.NetworkInterface)
	for _, iface := range backIfaces {
		roundtripped[iface.ID] = iface
	}

	rtA, ok := roundtripped["node-99/if-a"]
	if !ok {
		t.Fatalf("missing iface A in roundtrip map")
	}
	if rtA.ParentNodeID != "node-99" || rtA.Medium != core.MediumWired {
		t.Errorf("iface A mismatch: %+v", rtA)
	}
	if rtA.MACAddress != ifaceA.MACAddress || rtA.IPAddress != ifaceA.IPAddress {
		t.Errorf("iface A addressing mismatch: %+v", rtA)
	}

	rtB, ok := roundtripped["node-99/if-b"]
	if !ok {
		t.Fatalf("missing iface B in roundtrip map")
	}
	if rtB.ParentNodeID != "node-99" || rtB.Medium != core.MediumWireless {
		t.Errorf("iface B mismatch: %+v", rtB)
	}
	if rtB.TransceiverID != ifaceB.TransceiverID {
		t.Errorf("iface B TransceiverID mismatch: got %q, want %q", rtB.TransceiverID, ifaceB.TransceiverID)
	}
	if rtB.IsOperational != ifaceB.IsOperational {
		t.Errorf("iface B IsOperational mismatch: got %v, want %v", rtB.IsOperational, ifaceB.IsOperational)
	}
}
