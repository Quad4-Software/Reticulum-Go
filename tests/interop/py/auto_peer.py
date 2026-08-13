#!/usr/bin/env python3
"""Python peer for the AutoInterface live interop test. Brings up an RNS instance
with an AutoInterface on a specified device, announces a single SINGLE
destination, and stays alive until killed.

Required environment:
  INTEROP_DEVICE        Interface name to use (e.g. veth1)
  INTEROP_GROUP_ID      Optional group_id (default: reticulum)
  INTEROP_GO_DEST_HASH  Hex hash of the Go destination to wait for.

The script writes "READY\n" to stdout once the destination is ready, then
announces every 3 seconds and waits until it sees the Go destination.
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
INTEROP_ASPECT = "autosvc"


def main() -> int:
    device = os.environ["INTEROP_DEVICE"]
    group_id = os.environ.get("INTEROP_GROUP_ID", "reticulum")
    go_dest_hash_hex = os.environ.get("INTEROP_GO_DEST_HASH", "")
    go_dest_hash = bytes.fromhex(go_dest_hash_hex) if go_dest_hash_hex else None

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_auto_")

    config_path = os.path.join(cfg_dir, "config")
    with open(config_path, "w", encoding="utf-8") as f:
        f.write(
            "\n".join(
                [
                    "[reticulum]",
                    "enable_transport = false",
                    "share_instance = no",
                    "loglevel = 4",
                    "",
                    "[interfaces]",
                    "",
                    "[[interop_auto]]",
                    "type = AutoInterface",
                    "enabled = yes",
                    f"devices = {device}",
                    f"group_id = {group_id}",
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
    destination.set_proof_strategy(RNS.Destination.PROVE_ALL)

    sys.stdout.write("READY\n")
    sys.stdout.flush()

    if go_dest_hash is None:
        # Server mode: announce and stay alive
        while True:
            destination.announce()
            time.sleep(3.0)
    else:
        # Client mode: wait for path to Go destination
        deadline = time.time() + 60
        while time.time() < deadline:
            destination.announce()
            if RNS.Transport.hasPath(go_dest_hash):
                sys.stdout.write("OK\n")
                sys.stdout.flush()
                return 0
            time.sleep(1.0)
        sys.stdout.write("TIMEOUT\n")
        sys.stdout.flush()
        return 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
