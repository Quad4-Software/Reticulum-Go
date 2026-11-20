#!/usr/bin/env python3
"""Known-destinations RPC probe. Connects as a shared-instance client to an
already-running Go server in split mode (own config dir, so the server's
known_destinations.db is not readable) and verifies the RPC fallback loads
the server's table. Prints KD <count> and KD_KEY <hex> for each entry.
"""

import os
import sys
import tempfile

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS
RNS.Transport.DESTINATION_VALUE = float(os.environ.get("DEST_VALUE", "8"))



def main() -> int:
    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_kd_")

    reticulum = RNS.Reticulum(cfg_dir)
    if not reticulum.is_connected_to_shared_instance:
        sys.stdout.write("NOT_CLIENT\n")
        sys.stdout.flush()
        return 1

    kd = reticulum.get_known_destinations()
    if kd is None:
        sys.stdout.write("RPC_FAIL\n")
        sys.stdout.flush()
        return 1

    sys.stdout.write("KD %d\n" % len(kd))
    for h in sorted(kd):
        sys.stdout.write("KD_KEY " + h.hex() + "\n")
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
