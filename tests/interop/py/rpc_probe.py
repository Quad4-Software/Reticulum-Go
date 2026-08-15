#!/usr/bin/env python3
"""Python RPC probe for shared-instance interop. Connects as a shared-instance
client to an already-running Go server and calls get_link_count/get_path_table.
"""

import os
import sys
import tempfile

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS


def main() -> int:
    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_rpc_")

    reticulum = RNS.Reticulum(cfg_dir)
    if not reticulum.is_connected_to_shared_instance:
        sys.stdout.write("NOT_CLIENT\n")
        sys.stdout.flush()
        return 1

    count = reticulum.get_link_count()
    if count is None:
        sys.stdout.write("RPC_FAIL\n")
        sys.stdout.flush()
        return 1

    table = reticulum.get_path_table()
    if table is None:
        sys.stdout.write("PATH_FAIL\n")
        sys.stdout.flush()
        return 1

    sys.stdout.write("OK\n")
    sys.stdout.flush()
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as exc:
        sys.stdout.write(f"ERR {exc}\n")
        sys.stdout.flush()
        sys.exit(1)
