#!/usr/bin/env python3
"""Live interop: Python shared-instance client requests a path through a Go
shared-instance server to a destination that is served by a Python announce
peer on UDP. The announce peer does NOT announce proactively; it only
responds to path requests by announcing.

This tests the is_from_local_client forwarding branch: the Go server must
forward the Python client's path request onto its UDP interface even though
the local client interface has IFModeFull (not in DiscoverPathsFor).

Environment:
  INTEROP_CONFIG_DIR   - Go server config directory (shared instance TCP)
  INTEROP_PEER_HASH    - Destination hash to request
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
        sys.stdout.write(f"ERR bad hash len {len(peer_hash)}\n")
        sys.stdout.flush()
        return 1

    sys.stdout.write(f"REQUESTING {peer_hash_hex}\n")
    sys.stdout.flush()

    # Repeatedly request the path. The Go server forwards each PR on its UDP
    # interface. The announce peer responds with an announce. The Go server
    # registers the path and the Python client eventually sees it.
    deadline = time.time() + 60
    while time.time() < deadline:
        if RNS.Transport.has_path(peer_hash):
            sys.stdout.write("PATH_FOUND\n")
            sys.stdout.flush()
            return 0
        RNS.Transport.request_path(peer_hash)
        time.sleep(2.0)

    sys.stdout.write("PATH_TIMEOUT\n")
    sys.stdout.flush()
    return 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as exc:
        sys.stdout.write(f"ERR {exc}\n")
        sys.stdout.flush()
        sys.exit(1)
