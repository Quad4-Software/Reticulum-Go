#!/usr/bin/env python3
"""RNS BackboneInterface server for live Go interop (Linux only).

Environment:
    INTEROP_PORT       listen port for BackboneInterface
    RETICULUM_PATH     optional RNS source tree on sys.path
"""

import os
import sys
import tempfile
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS  # noqa: E402

INTEROP_APP = "interop_pygo"
INTEROP_ASPECT = "backbone"


def main() -> int:
    port = int(os.environ["INTEROP_PORT"])

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_backbone_")

    config_path = os.path.join(cfg_dir, "config")
    with open(config_path, "w", encoding="utf-8") as f:
        f.write(
            "\n".join(
                [
                    "[reticulum]",
                    "enable_transport = true",
                    "share_instance = no",
                    "loglevel = 2",
                    "",
                    "[interfaces]",
                    "",
                    "[[interop_backbone]]",
                    "type = BackboneInterface",
                    "enabled = yes",
                    "listen_ip = 127.0.0.1",
                    f"listen_port = {port}",
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

    sys.stdout.write("READY\n")
    sys.stdout.write(destination.hash.hex() + "\n")
    sys.stdout.flush()

    destination.announce()

    while True:
        time.sleep(5.0)
        destination.announce()


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
