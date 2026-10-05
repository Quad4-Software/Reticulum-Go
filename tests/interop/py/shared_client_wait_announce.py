#!/usr/bin/env python3
"""Shared-instance client that waits for a path without sending path requests.

Environment:
  INTEROP_CONFIG_DIR  shared-instance config directory
  INTEROP_PEER_HASH   destination hash to wait for
"""

import os
import sys
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS


def main() -> int:
    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        sys.stdout.write("ERR no INTEROP_CONFIG_DIR\n")
        sys.stdout.flush()
        return 1

    reticulum = RNS.Reticulum(cfg_dir)
    if not reticulum.is_connected_to_shared_instance:
        sys.stdout.write("NOT_CLIENT\n")
        sys.stdout.flush()
        return 1

    sys.stdout.write("CONNECTED\n")
    sys.stdout.flush()

    peer_hash_hex = os.environ.get("INTEROP_PEER_HASH")
    if not peer_hash_hex:
        sys.stdout.write("ERR no INTEROP_PEER_HASH\n")
        sys.stdout.flush()
        return 1

    peer_hash = bytes.fromhex(peer_hash_hex)
    if len(peer_hash) != RNS.Identity.TRUNCATED_HASHLENGTH // 8:
        sys.stdout.write("ERR bad hash len %d\n" % len(peer_hash))
        sys.stdout.flush()
        return 1

    sys.stdout.write("WAITING %s\n" % peer_hash_hex)
    sys.stdout.flush()

    deadline = time.time() + 60
    while time.time() < deadline:
        if RNS.Transport.has_path(peer_hash):
            sys.stdout.write("PATH_FOUND hops=%s\n" % RNS.Transport.hops_to(peer_hash))
            sys.stdout.flush()
            return 0
        time.sleep(0.2)

    sys.stdout.write("PATH_TIMEOUT\n")
    sys.stdout.flush()
    return 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as exc:
        sys.stdout.write("ERR %s\n" % exc)
        sys.stdout.flush()
        raise
