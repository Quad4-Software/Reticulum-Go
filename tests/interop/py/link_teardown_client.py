#!/usr/bin/env python3
"""Python link client that reports remote-initiated teardown.

Emits READY then LINK_UP once the link establishes. When the Go side
tears the link down, the link.closed callback emits LINK_CLOSED so the
Go test can assert the teardown packet arrived intact.
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
    go_hash = bytes.fromhex(os.environ["INTEROP_GO_DEST_HASH"].strip())

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_interop_td_")

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
    dest = None
    while time.time() < deadline:
        identity = RNS.Identity.recall(go_hash)
        if identity is not None:
            dest = RNS.Destination(
                identity,
                RNS.Destination.IN,
                RNS.Destination.SINGLE,
                INTEROP_APP,
                INTEROP_ASPECT,
            )
            break
        RNS.Transport.request_path(go_hash)
        time.sleep(0.12)

    if dest is None:
        sys.stderr.write("timeout: could not recall identity\n")
        return 1

    def on_established(link):
        sys.stdout.write("LINK_UP\n")
        sys.stdout.flush()
        link.set_link_closed_callback(
            lambda l: (
                sys.stdout.write("LINK_CLOSED\n"),
                sys.stdout.flush(),
            )
        )

    RNS.Link(dest, on_established)

    while True:
        time.sleep(60.0)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
