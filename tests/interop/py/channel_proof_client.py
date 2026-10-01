#!/usr/bin/env python3
"""Assert Go proves inbound channel packets.

Links to the Go destination, sends a channel envelope, then waits for
the envelope packet receipt to reach DELIVERED, which only happens when
the Go side emits the channel packet proof.
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


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    go_hash = bytes.fromhex(os.environ["INTEROP_GO_DEST_HASH"].strip())

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_interop_cp_")

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
    sys.stdout.write("READY\n")
    sys.stdout.flush()

    deadline = time.time() + 60.0
    dest = None
    while time.time() < deadline:
        identity = RNS.Identity.recall(go_hash)
        if identity is not None:
            dest = RNS.Destination(
                identity,
                RNS.Destination.IN,
                RNS.Destination.SINGLE,
                INTEROP_APP,
                INTEROP_ASPECT,
            )
            break
        RNS.Transport.request_path(go_hash)
        time.sleep(0.15)

    if dest is None:
        sys.stderr.write("could not recall identity\n")
        return 1

    done = []

    def on_established(link):
        ch = link.get_channel()

        class ProofMsg(RNS.Channel.MessageBase):
            MSGTYPE = 0x00A1

            def __init__(self, data=None):
                self.data = data if data is not None else b""

            def pack(self):
                return self.data

            def unpack(self, raw):
                self.data = raw

        ch.register_message_type(ProofMsg)
        env = ch.send(ProofMsg(b"prove-me"))
        sys.stdout.write("SENT\n")
        sys.stdout.flush()

        deadline = time.time() + 20.0
        while time.time() < deadline:
            if env.packet and env.packet.receipt:
                status = env.packet.receipt.get_status()
                if status == RNS.PacketReceipt.DELIVERED:
                    done.append(True)
                    sys.stdout.write("PROVED\n")
                    sys.stdout.flush()
                    return
            time.sleep(0.05)
        sys.stderr.write("receipt never reached DELIVERED\n")

    RNS.Link(dest, on_established)

    end = time.time() + 40.0
    while time.time() < end and not done:
        time.sleep(0.1)

    if not done:
        return 1
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
