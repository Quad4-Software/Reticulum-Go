#!/usr/bin/env python3
"""Wait for a path to a destination and report its hop count.

Emits READY, then PATH_HOPS <n> when the target appears in the local
path table, where n is the stored hops field. Used to assert multi-hop
announce progression through chained Go relays.
"""

import os
import sys
import tempfile
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    target = bytes.fromhex(os.environ["INTEROP_TARGET_HASH"].strip())

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_interop_ph_")

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
    while time.time() < deadline:
        entry = RNS.Transport.path_table.get(target)
        if entry is not None:
            sys.stdout.write("PATH_HOPS " + str(entry[2]) + "\n")
            sys.stdout.flush()
            return 0
        RNS.Transport.request_path(target)
        time.sleep(0.5)

    sys.stderr.write("no path learned\n")
    return 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
