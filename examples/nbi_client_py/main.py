"""
Minimal Python example client for the NBI gRPC server.

It connects to an endpoint, builds a small scenario (2 platforms, 2 nodes,
1 bidirectional link), then fetches and prints a ScenarioSnapshot.

Dependencies: see requirements.txt (grpcio + protobuf). This script uses the
prebuilt descriptor set at the repo root (nbi_descriptor.pb) so no code
generation is required.
"""

from __future__ import annotations

import argparse
import pathlib
from typing import Iterable, Tuple

import grpc
from google.protobuf import descriptor_pb2, descriptor_pool, empty_pb2, json_format, message_factory


class DynamicNBI:
    """Tiny helper that builds messages and invokes RPCs from a descriptor set."""

    def __init__(self, channel: grpc.Channel, descriptor_path: pathlib.Path, timeout: float = 10.0) -> None:
        self.channel = channel
        self.timeout = timeout

        pool = descriptor_pool.DescriptorPool()
        fds = descriptor_pb2.FileDescriptorSet()
        fds.ParseFromString(descriptor_path.read_bytes())
        for fd in fds.file:
            pool.Add(fd)

        factory = message_factory.MessageFactory(pool)
        self.pool = pool
        self.factory = factory

        # Message prototypes we care about
        self.PlatformDefinition = self._msg("aalyria.spacetime.api.common.PlatformDefinition")
        self.NetworkNode = self._msg("aalyria.spacetime.api.nbi.v1alpha.resources.NetworkNode")
        self.NetworkInterface = self._msg("aalyria.spacetime.api.nbi.v1alpha.resources.NetworkInterface")
        self.BidirectionalLink = self._msg("aalyria.spacetime.api.nbi.v1alpha.resources.BidirectionalLink")
        self.TransceiverModelId = self._msg("aalyria.spacetime.api.common.TransceiverModelId")
        self.ClearScenarioRequest = self._msg("aalyria.spacetime.api.nbi.v1alpha.ClearScenarioRequest")
        self.GetScenarioRequest = self._msg("aalyria.spacetime.api.nbi.v1alpha.GetScenarioRequest")
        self.ScenarioSnapshot = self._msg("aalyria.spacetime.api.nbi.v1alpha.ScenarioSnapshot")

        motion_enum = pool.FindEnumTypeByName("aalyria.spacetime.api.common.PlatformDefinition.MotionSource")
        self.motion_unknown = motion_enum.values_by_name["UNKNOWN_SOURCE"].number

    def _msg(self, name: str):
        return self.factory.GetPrototype(self.pool.FindMessageTypeByName(name))

    def _parse(self, cls, data: bytes):
        msg = cls()
        msg.ParseFromString(data)
        return msg

    def _unary(self, method: str, request, response_cls):
        call = self.channel.unary_unary(
            method,
            request_serializer=request.SerializeToString,
            response_deserializer=lambda data: self._parse(response_cls, data),
        )
        return call(request, timeout=self.timeout)

    def clear_scenario(self) -> None:
        self._unary(
            "/aalyria.spacetime.api.nbi.v1alpha.ScenarioService/ClearScenario",
            self.ClearScenarioRequest(),
            empty_pb2.Empty,
        )

    def create_platform(self, name: str, typ: str, coords: Tuple[float, float, float]):
        plat = self.PlatformDefinition()
        plat.name = name
        plat.type = typ
        plat.motion_source = self.motion_unknown
        plat.coordinates.ecef_fixed.point.x_m = coords[0]
        plat.coordinates.ecef_fixed.point.y_m = coords[1]
        plat.coordinates.ecef_fixed.point.z_m = coords[2]
        return self._unary(
            "/aalyria.spacetime.api.nbi.v1alpha.PlatformService/CreatePlatform",
            plat,
            self.PlatformDefinition,
        )

    def create_node(self, node_id: str, platform_id: str, iface_id: str, transceiver_id: str):
        node = self.NetworkNode()
        node.node_id = node_id
        node.type = "ROUTER"
        iface = self.NetworkInterface()
        iface.interface_id = iface_id
        iface.wireless.platform = platform_id
        iface.wireless.transceiver_model_id.transceiver_model_id = transceiver_id
        node.node_interface.append(iface)
        return self._unary(
            "/aalyria.spacetime.api.nbi.v1alpha.NetworkNodeService/CreateNode",
            node,
            self.NetworkNode,
        )

    def create_link(self, a_node: str, a_iface: str, b_node: str, b_iface: str):
        link = self.BidirectionalLink()
        link.a_network_node_id = a_node
        link.a_tx_interface_id = a_iface
        link.a_rx_interface_id = a_iface
        link.b_network_node_id = b_node
        link.b_tx_interface_id = b_iface
        link.b_rx_interface_id = b_iface
        return self._unary(
            "/aalyria.spacetime.api.nbi.v1alpha.NetworkLinkService/CreateLink",
            link,
            self.BidirectionalLink,
        )

    def get_scenario(self):
        return self._unary(
            "/aalyria.spacetime.api.nbi.v1alpha.ScenarioService/GetScenario",
            self.GetScenarioRequest(),
            self.ScenarioSnapshot,
        )


def print_platforms(platforms: Iterable, indent: str = "") -> None:
    print(f"{indent}Platforms:")
    for p in platforms:
        pt = getattr(getattr(getattr(p, "coordinates", None), "ecef_fixed", None), "point", None)
        coord = "n/a"
        if pt:
            coord = f"({pt.x_m:.1f}, {pt.y_m:.1f}, {pt.z_m:.1f}) m"
        print(f"{indent}- {p.name} [{p.type}] coords={coord}")


def print_nodes(nodes: Iterable, indent: str = "") -> None:
    print(f"{indent}Nodes:")
    for n in nodes:
        print(f"{indent}- {n.node_id} [{n.type}]")
        for iface in n.node_interface:
            wireless = iface.wireless
            trx = getattr(getattr(wireless, "transceiver_model_id", None), "transceiver_model_id", "")
            print(f"{indent}  interface {iface.interface_id} platform={wireless.platform} trx={trx}")


def print_links(links: Iterable, indent: str = "") -> None:
    print(f"{indent}Links:")
    for l in links:
        print(
            f"{indent}- {l.a_network_node_id}/{l.a_tx_interface_id} <-> "
            f"{l.b_network_node_id}/{l.b_tx_interface_id}"
        )


def main() -> None:
    parser = argparse.ArgumentParser(description="Minimal NBI Python client example")
    parser.add_argument("--endpoint", default="localhost:50051", help="NBI gRPC endpoint")
    parser.add_argument("--transceiver-id", default="trx-ku", help="Transceiver model ID configured on the server")
    parser.add_argument(
        "--descriptor",
        type=pathlib.Path,
        default=pathlib.Path(__file__).resolve().parent.parent.parent / "nbi_descriptor.pb",
        help="Path to nbi_descriptor.pb",
    )
    parser.add_argument("--timeout", type=float, default=10.0, help="Per-RPC timeout in seconds")
    parser.add_argument("--skip-clear", action="store_true", help="Do not call ClearScenario first")
    args = parser.parse_args()

    channel = grpc.insecure_channel(args.endpoint)
    client = DynamicNBI(channel, args.descriptor, timeout=args.timeout)

    if not args.skip_clear:
        client.clear_scenario()

    client.create_platform("platform-ground", "GROUND_STATION", (6_372_000.0, 0.0, 0.0))
    client.create_platform("platform-sat", "SATELLITE", (6_871_000.0, 0.0, 0.0))
    client.create_node("node-ground", "platform-ground", "if-ground", args.transceiver_id)
    client.create_node("node-sat", "platform-sat", "if-sat", args.transceiver_id)
    client.create_link("node-ground", "if-ground", "node-sat", "if-sat")

    snapshot = client.get_scenario()
    print("\nScenario snapshot:")
    print_platforms(snapshot.platforms, indent="  ")
    print_nodes(snapshot.nodes, indent="  ")
    print_links(snapshot.links, indent="  ")

    # For debugging, you can dump the whole snapshot:
    # print(json_format.MessageToJson(snapshot, including_default_value_fields=False, indent=2))


if __name__ == "__main__":
    main()
