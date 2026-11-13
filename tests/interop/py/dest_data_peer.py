#!/usr/bin/env python3
"""UDP SINGLE destination DATA peer for Go live interop.

Announces a non-ratchet SINGLE destination (LXMF shape), receives DATA
packets on it, and can send DATA packets to a Go destination.

Environment:
  INTEROP_LISTEN_PORT    UDP listen port
  INTEROP_FORWARD_PORT   UDP forward port (Go transport listen port)
  INTEROP_GO_DEST_HASH   Hex hash of the Go SINGLE destination
  INTEROP_APP            Destination app name (default: interop_pygo)
  INTEROP_ASPECT         Destination aspect (default: destdata)
  INTEROP_CONFIG_DIR     Optional Reticulum config dir
  INTEROP_TIMEOUT_SEC    Overall timeout (default: 120)

Stdin protocol:
  SEND <hex_payload>   Send a DATA packet to INTEROP_GO_DEST_HASH

Stdout protocol:
  READY
  <dest_hash_hex>
  DATA <hex>           For each DATA packet received on the destination
  SENT <hex>           After a SEND command was transmitted
"""

import os
import select
import sys
import tempfile
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS

INTEROP_APP = "interop_pygo"
INTEROP_ASPECT = "destdata"


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    go_hash_hex = os.environ.get("INTEROP_GO_DEST_HASH", "").strip()
    timeout_sec = float(os.environ.get("INTEROP_TIMEOUT_SEC", "120"))
    app = os.environ.get("INTEROP_APP", INTEROP_APP)
    aspect = os.environ.get("INTEROP_ASPECT", INTEROP_ASPECT)

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_interop_")

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
                ],
            ),
        )

    RNS.Reticulum(cfg_dir)

    identity = RNS.Identity()
    destination = RNS.Destination(
        identity,
        RNS.Destination.IN,
        RNS.Destination.SINGLE,
        app,
        aspect,
    )
    destination.ratchets = None

    def on_data(data, packet):
        sys.stdout.write("DATA " + data.hex() + "\n")
        sys.stdout.flush()

    destination.set_packet_callback(on_data)

    sys.stdout.write("READY\n")
    sys.stdout.write(destination.hash.hex() + "\n")
    sys.stdout.flush()

    destination.announce()

    go_hash = bytes.fromhex(go_hash_hex) if go_hash_hex else None
    go_dest = None
    if go_hash is not None:
        deadline = time.time() + timeout_sec
        while time.time() < deadline:
            if RNS.Transport.has_path(go_hash):
                break
            RNS.Transport.request_path(go_hash)
            time.sleep(0.25)
        if not RNS.Transport.has_path(go_hash):
            sys.stderr.write("no path to go destination\n")
            return 1
        try:
            go_identity = RNS.Identity.recall(go_hash)
        except Exception as e:
            sys.stderr.write("identity recall failed: %s\n" % e)
            return 1
        go_dest = RNS.Destination(
            go_identity, RNS.Destination.OUT, RNS.Destination.SINGLE, app, aspect
        )

    deadline = time.time() + timeout_sec
    while time.time() < deadline:
        ready, _, _ = select.select([sys.stdin], [], [], 0.25)
        if not ready:
            continue
        line = sys.stdin.readline()
        if not line:
            break
        line = line.strip()
        if not line:
            continue
        if line.startswith("SEND ") and go_dest is not None:
            payload = bytes.fromhex(line[5:].strip())
            RNS.Packet(go_dest, payload).send()
            sys.stdout.write("SENT " + payload.hex() + "\n")
            sys.stdout.flush()

    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
