#!/usr/bin/env python3
"""Generic Python RNS peer for Go interop tests.

Uses a pre-written Reticulum config directory (INTEROP_CONFIG_DIR) so the Go
test controls the interface type (UDP, TCPClient, TCPServer, Serial, ...).
Announces a SINGLE destination with PROVE_ALL, accepts links, echoes link
packets, and accepts resources.

Environment:
  INTEROP_CONFIG_DIR   Reticulum config dir containing a "config" file.
  INTEROP_APP          Destination app name (default: interop_pygo)
  INTEROP_ASPECT       Destination aspect (default: linksvc)
  INTEROP_CLIENT_TO    Optional hex destination hash: after announcing, open a
                       link to it, send one packet, and print ECHO_OK on reply.
  INTEROP_TIMEOUT_SEC  Overall timeout (default: 90)

Stdout protocol:
  READY
  <dest_hash_hex>
  ECHO_OK        (client mode only, after echo reply)
"""

import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import interop_events

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS


def main() -> int:
    cfg_dir = os.environ["INTEROP_CONFIG_DIR"]
    app = os.environ.get("INTEROP_APP", "interop_pygo")
    aspect = os.environ.get("INTEROP_ASPECT", "linksvc")
    client_to = os.environ.get("INTEROP_CLIENT_TO", "").strip()
    timeout_sec = float(os.environ.get("INTEROP_TIMEOUT_SEC", "90"))

    RNS.Reticulum(cfg_dir)

    identity = RNS.Identity()
    destination = RNS.Destination(
        identity,
        RNS.Destination.IN,
        RNS.Destination.SINGLE,
        app,
        aspect,
    )
    destination.set_proof_strategy(RNS.Destination.PROVE_ALL)

    state = {"echoed": False}

    def on_link(link):
        link.set_resource_strategy(RNS.Link.ACCEPT_ALL)

        def on_packet(message, packet):
            RNS.Packet(packet.link, message).send()

        link.set_packet_callback(on_packet)

    destination.set_link_established_callback(on_link)

    sys.stdout.write("READY\n")
    sys.stdout.write(destination.hash.hex() + "\n")
    sys.stdout.flush()
    interop_events.emit("ready", detail=destination.hash.hex())

    destination.announce()

    if client_to:
        go_hash = bytes.fromhex(client_to)
        deadline = time.time() + timeout_sec
        while time.time() < deadline:
            if RNS.Transport.has_path(go_hash):
                break
            RNS.Transport.request_path(go_hash)
            time.sleep(0.25)
        if not RNS.Transport.has_path(go_hash):
            interop_events.emit("fail", kind="path", detail="no path to go dest")
            sys.stderr.write("no path to go destination\n")
            return 1

        go_identity = RNS.Identity.recall(go_hash)
        go_dest = RNS.Destination(
            go_identity, RNS.Destination.OUT, RNS.Destination.SINGLE, app, aspect
        )

        def on_established(link):
            payload = b"python-echo-probe"

            def on_packet(message, packet):
                if message == payload and not state["echoed"]:
                    state["echoed"] = True
                    sys.stdout.write("ECHO_OK\n")
                    sys.stdout.flush()
                    interop_events.emit("request_ok", detail="client_echo")

            link.set_packet_callback(on_packet)
            RNS.Packet(link, payload).send()

        RNS.Link(go_dest, on_established)

    deadline = time.time() + timeout_sec
    while time.time() < deadline:
        if client_to and state["echoed"]:
            return 0
        if not client_to:
            destination.announce()
        time.sleep(3.0)

    if client_to and not state["echoed"]:
        interop_events.emit("fail", kind="timeout", detail="echo not returned")
        sys.stderr.write("timeout: no echo reply\n")
        return 1
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
