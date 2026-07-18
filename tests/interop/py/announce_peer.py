#!/usr/bin/env python3
"""UDP announce peer for Go live interop.

Emits READY, destination hash hex, then ANNOUNCE_HEX of the packed announce
before sending it. Set INTEROP_NO_RATCHET=1 to omit the ratchet field (context
flag unset), matching common RNS Destination.announce wire shapes.
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
    no_ratchet = os.environ.get("INTEROP_NO_RATCHET", "").strip() in ("1", "true", "yes")
    app_data = os.environ.get("INTEROP_APP_DATA", "live-announce").encode("utf-8")

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
                ]
            )
        )

    RNS.Reticulum(cfg_dir)

    identity = RNS.Identity()
    destination = RNS.Destination(
        identity,
        RNS.Destination.IN,
        RNS.Destination.SINGLE,
        INTEROP_APP,
        INTEROP_ASPECT,
    )
    if no_ratchet:
        destination.ratchets = None

    h = destination.hash
    sys.stdout.write("READY\n")
    sys.stdout.write(h.hex() + "\n")
    sys.stdout.flush()

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
