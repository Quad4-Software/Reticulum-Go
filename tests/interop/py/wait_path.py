#!/usr/bin/env python3
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
    go_hash_hex = os.environ["INTEROP_GO_DEST_HASH"].strip()
    go_hash = bytes.fromhex(go_hash_hex)

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

    sys.stdout.write("READY\n")
    sys.stdout.flush()

    deadline = time.time() + 45.0
    while time.time() < deadline:
        if RNS.Transport.has_path(go_hash):
            sys.stdout.write("OK\n")
            sys.stdout.flush()
            return 0
        RNS.Transport.request_path(go_hash)
        time.sleep(0.15)

    sys.stderr.write("timeout waiting for path to Go destination\n")
    return 1


if __name__ == "__main__":
    sys.exit(main())
