#!/usr/bin/env python3
import os
import sys
import tempfile
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS

INTEROP_APP = "interop_pygo"
INTEROP_ASPECT = "linksvc"

def write_config(cfg_dir: str, listen_port: int, forward_port: int) -> None:
    config_path = os.path.join(cfg_dir, "config")
    with open(config_path, "w", encoding="utf-8") as f:
        f.write(
            "\n".join(
                [
                    "[reticulum]",
                    "enable_transport = false",
                    "share_instance = no",
                    "loglevel = 2",
                    "",
                    "[interfaces]",
                    "",
                    "[[interop_udp]]",
                    "type = UDPInterface",
                    "enabled = yes",
                    "listen_ip = 127.0.0.1",
                    f"listen_port = {listen_port}",
                    "forward_ip = 127.0.0.1",
                    f"forward_port = {forward_port}",
                    "",
                ]
            )
        )


def client_connected(link):
    link.set_link_closed_callback(lambda _: None)
    mode = os.environ.get("INTEROP_LINK_MODE", "echo")

    if mode == "echo":

        def server_packet_received(message, packet):
            RNS.Packet(link, message).send()

        link.set_packet_callback(server_packet_received)
    elif mode == "resource":

        def server_packet_received(message, packet):
            pass

        link.set_packet_callback(server_packet_received)
        link.set_resource_strategy(RNS.Link.ACCEPT_ALL)

        def on_resource_concluded(resource):
            if resource.status != RNS.Resource.COMPLETE:
                sys.stderr.write("resource not complete status=%s\n" % resource.status)
                sys.stderr.flush()
                return
            try:
                if hasattr(resource.data, "read"):
                    if hasattr(resource.data, "seek"):
                        resource.data.seek(0)
                    data = resource.data.read()
                else:
                    data = resource.data
                if isinstance(data, str):
                    data = data.encode("utf-8")
            except Exception as exc:
                sys.stderr.write("resource read: " + str(exc) + "\n")
                data = b""
            expected = os.environ.get("INTEROP_RESOURCE_EXPECT", "interop-resource-payload").encode("utf-8")
            if data == expected:
                sys.stdout.write("RESOURCE_OK\n")
                sys.stdout.flush()
            else:
                sys.stderr.write("resource data mismatch got %r\n" % (data,))
                sys.stderr.flush()

        link.set_resource_concluded_callback(on_resource_concluded)


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_interop_")

    write_config(cfg_dir, listen_port, forward_port)
    RNS.Reticulum(cfg_dir)

    identity = RNS.Identity()
    destination = RNS.Destination(
        identity,
        RNS.Destination.IN,
        RNS.Destination.SINGLE,
        INTEROP_APP,
        INTEROP_ASPECT,
    )
    destination.set_link_established_callback(client_connected)

    h = destination.hash
    sys.stdout.write("READY\n")
    sys.stdout.write(h.hex() + "\n")
    sys.stdout.flush()

    destination.announce()

    while True:
        time.sleep(60.0)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
