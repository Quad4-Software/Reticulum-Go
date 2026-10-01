#!/usr/bin/env python3
"""Announce twice with different app_data for re-announce coverage.

Emits READY, hash, ANN1, then ANN2 after a delay so the Go side can
assert both app_data payloads arrived in order.
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
    app_data_1 = os.environ.get("INTEROP_APP_DATA_1", "announce-v1").encode("utf-8")
    app_data_2 = os.environ.get("INTEROP_APP_DATA_2", "announce-v2").encode("utf-8")
    gap = float(os.environ.get("INTEROP_REANNOUNCE_GAP", "2.0"))

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_interop_ra_")

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
        INTEROP_APP,
        INTEROP_ASPECT,
    )

    sys.stdout.write("READY\n")
    sys.stdout.write(destination.hash.hex() + "\n")
    sys.stdout.flush()

    destination.announce(app_data=app_data_1)
    sys.stdout.write("ANN1\n")
    sys.stdout.flush()
    time.sleep(gap)
    destination.announce(app_data=app_data_2)
    sys.stdout.write("ANN2\n")
    sys.stdout.flush()

    while True:
        time.sleep(60.0)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
