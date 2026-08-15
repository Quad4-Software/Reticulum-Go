#!/usr/bin/env python3
"""Ratchet announce/encrypt peer for Go live interop.

Modes:
  announce  enable_ratchets, announce, decrypt inbound DATA, print DECRYPTED
  encrypt   wait for INTEROP_GO_DEST_HASH, encrypt+send INTEROP_PLAINTEXT
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
                ],
            ),
        )


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    mode = os.environ.get("INTEROP_MODE", "announce").strip()
    app_data = os.environ.get("INTEROP_APP_DATA", "ratchet-live").encode("utf-8")
    plaintext = os.environ.get("INTEROP_PLAINTEXT", "hello-ratchet").encode("utf-8")

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_ratchet_")
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
    ratchet_path = os.path.join(cfg_dir, "dest_ratchets")
    destination.enable_ratchets(ratchet_path)
    destination.enforce_ratchets()

    def on_packet(data, packet):
        sys.stdout.write("DECRYPTED " + data.decode("utf-8", errors="replace") + "\n")
        sys.stdout.flush()

    destination.set_packet_callback(on_packet)

    sys.stdout.write("READY\n")
    sys.stdout.write(destination.hash.hex() + "\n")
    sys.stdout.flush()

    if mode == "encrypt":
        go_hash = bytes.fromhex(os.environ["INTEROP_GO_DEST_HASH"].strip())
        deadline = time.time() + 40.0
        while time.time() < deadline:
            if RNS.Transport.has_path(go_hash):
                break
            RNS.Transport.request_path(go_hash)
            time.sleep(0.25)
        else:
            sys.stdout.write("FAIL no path\n")
            sys.stdout.flush()
            return 1
        recalled = RNS.Identity.recall(go_hash)
        if recalled is None:
            sys.stdout.write("FAIL no identity\n")
            sys.stdout.flush()
            return 1
        out_dest = RNS.Destination(
            recalled,
            RNS.Destination.OUT,
            RNS.Destination.SINGLE,
            INTEROP_APP,
            INTEROP_ASPECT,
        )
        ratchet = RNS.Identity.get_ratchet(out_dest.hash)
        if not ratchet:
            sys.stdout.write("FAIL no ratchet\n")
            sys.stdout.flush()
            return 1
        pkt = RNS.Packet(out_dest, plaintext)
        pkt.send()
        sys.stdout.write("SENT\n")
        sys.stdout.flush()
        while True:
            time.sleep(60.0)
        return 0

    packet = destination.announce(app_data=app_data, send=False)
    packet.pack()
    ctx_flag = (packet.raw[0] >> 5) & 1
    sys.stdout.write("ANNOUNCE_HEX " + packet.raw.hex() + "\n")
    sys.stdout.write("CONTEXT_FLAG " + str(ctx_flag) + "\n")
    sys.stdout.flush()
    packet.send()

    while True:
        time.sleep(60.0)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
