#!/usr/bin/env python3
"""GROUP destination Token peer for Go live interop.

Modes:
  listen  IN GROUP, decrypt inbound DATA, print DECRYPTED
  send    OUT GROUP, encrypt+send INTEROP_PLAINTEXT
"""

import os
import sys
import tempfile
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS

INTEROP_APP = "interop_pygo"
INTEROP_ASPECT = "groupsvc"


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
                ],
            ),
        )


def load_identity() -> RNS.Identity:
    raw = bytes.fromhex(os.environ["INTEROP_IDENTITY_HEX"].strip())
    identity = RNS.Identity()
    identity.load_private_key(raw)
    return identity


def load_group_dest(identity: RNS.Identity, direction: int) -> RNS.Destination:
    dest = RNS.Destination(
        identity,
        direction,
        RNS.Destination.GROUP,
        INTEROP_APP,
        INTEROP_ASPECT,
    )
    dest.load_private_key(bytes.fromhex(os.environ["INTEROP_GROUP_KEY_HEX"].strip()))
    return dest


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    mode = os.environ.get("INTEROP_MODE", "listen").strip()
    plaintext = os.environ.get("INTEROP_PLAINTEXT", "hello-group").encode("utf-8")

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_group_")
    write_config(cfg_dir, listen_port, forward_port)
    RNS.Reticulum(cfg_dir)

    identity = load_identity()
    if mode == "send":
        dest = load_group_dest(identity, RNS.Destination.OUT)
        sys.stdout.write("READY\n")
        sys.stdout.write(dest.hash.hex() + "\n")
        sys.stdout.flush()
        pkt = RNS.Packet(dest, plaintext)
        pkt.send()
        sys.stdout.write("SENT\n")
        sys.stdout.flush()
        while True:
            time.sleep(60.0)
        return 0

    dest = load_group_dest(identity, RNS.Destination.IN)

    def on_packet(data, packet):
        sys.stdout.write("DECRYPTED " + data.decode("utf-8", errors="replace") + "\n")
        sys.stdout.flush()

    dest.set_packet_callback(on_packet)
    sys.stdout.write("READY\n")
    sys.stdout.write(dest.hash.hex() + "\n")
    sys.stdout.flush()
    while True:
        time.sleep(60.0)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
