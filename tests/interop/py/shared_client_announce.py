#!/usr/bin/env python3
"""Shared-instance client that announces a destination.

Environment:
  INTEROP_CONFIG_DIR  shared-instance config directory
"""

import os
import sys
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS

INTEROP_APP = "interop_pygo"
INTEROP_ASPECT = "sharedclient"


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

    identity = RNS.Identity()
    destination = RNS.Destination(
        identity,
        RNS.Destination.IN,
        RNS.Destination.SINGLE,
        INTEROP_APP,
        INTEROP_ASPECT,
    )
    sys.stdout.write(destination.hash.hex() + "\n")
    sys.stdout.flush()

    destination.announce(app_data=b"shared-client")
    sys.stdout.write("ANNOUNCED\n")
    sys.stdout.flush()

    while True:
        time.sleep(2.0)
        destination.announce(app_data=b"shared-client")


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
    except Exception as exc:
        sys.stdout.write("ERR %s\n" % exc)
        sys.stdout.flush()
        raise
