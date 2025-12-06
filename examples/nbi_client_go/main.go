package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	common "aalyria.com/spacetime/api/common"
	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	endpoint := flag.String("endpoint", "localhost:50051", "NBI gRPC endpoint (host:port)")
	transceiver := flag.String("transceiver-id", "trx-ku", "Transceiver model ID configured on the server")
	clear := flag.Bool("clear", true, "Call ScenarioService.ClearScenario before creating entities")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, *endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial %s: %v", *endpoint, err)
	}
	defer func() { _ = conn.Close() }()

	platforms := v1alpha.NewPlatformServiceClient(conn)
	nodes := v1alpha.NewNetworkNodeServiceClient(conn)
	links := v1alpha.NewNetworkLinkServiceClient(conn)
	scenarios := v1alpha.NewScenarioServiceClient(conn)

	if *clear {
		if _, err := scenarios.ClearScenario(ctx, &v1alpha.ClearScenarioRequest{}); err != nil {
			log.Fatalf("clear scenario: %v", err)
		}
		log.Println("cleared existing scenario")
	}

	groundID := "platform-ground"
	satID := "platform-sat"
	groundNode := "node-ground"
	satNode := "node-sat"
	groundIface := "if-ground"
	satIface := "if-sat"

	groundPos := cartesian(6372_000, 0, 0) // near Earth's surface, in meters
	satPos := cartesian(6_871_000, 0, 0)   // simple static satellite position

	log.Println("creating platforms...")
	if _, err := platforms.CreatePlatform(ctx, platformProto(groundID, "GROUND_STATION", groundPos)); err != nil {
		log.Fatalf("create platform %s: %v", groundID, err)
	}
	if _, err := platforms.CreatePlatform(ctx, platformProto(satID, "SATELLITE", satPos)); err != nil {
		log.Fatalf("create platform %s: %v", satID, err)
	}

	log.Println("creating nodes and interfaces...")
	if _, err := nodes.CreateNode(ctx, wirelessNodeProto(groundNode, groundID, groundIface, *transceiver)); err != nil {
		log.Fatalf("create node %s: %v", groundNode, err)
	}
	if _, err := nodes.CreateNode(ctx, wirelessNodeProto(satNode, satID, satIface, *transceiver)); err != nil {
		log.Fatalf("create node %s: %v", satNode, err)
	}

	log.Println("linking interfaces...")
	if _, err := links.CreateLink(ctx, bidirectionalLinkProto(groundNode, groundIface, satNode, satIface)); err != nil {
		log.Fatalf("create link: %v", err)
	}

	log.Println("fetching scenario snapshot...")
	snapshot, err := scenarios.GetScenario(ctx, &v1alpha.GetScenarioRequest{})
	if err != nil {
		log.Fatalf("get scenario: %v", err)
	}

	fmt.Println("\nScenario snapshot")
	printPlatforms(snapshot.GetPlatforms())
	printNodes(snapshot.GetNodes())
	printLinks(snapshot.GetLinks())

	_ = ctx // prevent unused in case flags change
	_ = links
}

func platformProto(name, typ string, pos *common.Cartesian) *common.PlatformDefinition {
	return &common.PlatformDefinition{
		Name:         stringPtr(name),
		Type:         stringPtr(typ),
		MotionSource: common.PlatformDefinition_UNKNOWN_SOURCE.Enum(),
		Coordinates: &common.Motion{
			Type: &common.Motion_EcefFixed{
				EcefFixed: &common.PointAxes{
					Point: pos,
				},
			},
		},
	}
}

func wirelessNodeProto(nodeID, platformID, ifaceID, trxID string) *resources.NetworkNode {
	nodeType := "ROUTER"
	return &resources.NetworkNode{
		NodeId: stringPtr(nodeID),
		Type:   &nodeType,
		NodeInterface: []*resources.NetworkInterface{
			{
				InterfaceId: stringPtr(ifaceID),
				InterfaceMedium: &resources.NetworkInterface_Wireless{
					Wireless: &resources.WirelessDevice{
						Platform: stringPtr(platformID),
						TransceiverModelId: &common.TransceiverModelId{
							TransceiverModelId: stringPtr(trxID),
						},
					},
				},
			},
		},
	}
}

func bidirectionalLinkProto(aNode, aIface, bNode, bIface string) *resources.BidirectionalLink {
	return &resources.BidirectionalLink{
		ANetworkNodeId: stringPtr(aNode),
		ATxInterfaceId: stringPtr(aIface),
		ARxInterfaceId: stringPtr(aIface),
		BNetworkNodeId: stringPtr(bNode),
		BTxInterfaceId: stringPtr(bIface),
		BRxInterfaceId: stringPtr(bIface),
	}
}

func cartesian(x, y, z float64) *common.Cartesian {
	return &common.Cartesian{
		XM: &x,
		YM: &y,
		ZM: &z,
	}
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func printPlatforms(platforms []*common.PlatformDefinition) {
	fmt.Println("Platforms:")
	for _, p := range platforms {
		if p == nil {
			continue
		}
		coords := "n/a"
		if p.GetCoordinates() != nil && p.GetCoordinates().GetEcefFixed() != nil && p.GetCoordinates().GetEcefFixed().GetPoint() != nil {
			pt := p.GetCoordinates().GetEcefFixed().GetPoint()
			coords = fmt.Sprintf("(%.1f, %.1f, %.1f) m", pt.GetXM(), pt.GetYM(), pt.GetZM())
		}
		fmt.Printf("- %s [%s] coords=%s\n", p.GetName(), p.GetType(), coords)
	}
}

func printNodes(nodes []*resources.NetworkNode) {
	fmt.Println("Nodes:")
	for _, n := range nodes {
		if n == nil {
			continue
		}
		fmt.Printf("- %s [%s]\n", n.GetNodeId(), n.GetType())
		for _, iface := range n.GetNodeInterface() {
			if iface == nil || iface.GetWireless() == nil {
				continue
			}
			wireless := iface.GetWireless()
			fmt.Printf("  interface %s platform=%s trx=%s\n",
				iface.GetInterfaceId(), wireless.GetPlatform(), wireless.GetTransceiverModelId().GetTransceiverModelId())
		}
	}
}

func printLinks(links []*resources.BidirectionalLink) {
	fmt.Println("Links:")
	for _, l := range links {
		if l == nil {
			continue
		}
		fmt.Printf("- %s/%s <-> %s/%s\n",
			l.GetANetworkNodeId(), l.GetATxInterfaceId(),
			l.GetBNetworkNodeId(), l.GetBTxInterfaceId())
	}
}
