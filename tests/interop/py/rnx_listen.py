#!/usr/bin/env python3
"""Python rnx.execute listener for Go↔Python interop tests."""

import os
import sys
import tempfile
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS
from RNS.Utilities import rnx as rnx_mod


def write_config(cfg_dir: str, listen_port: int, forward_port: int) -> None:
    with open(os.path.join(cfg_dir, "config"), "w", encoding="utf-8") as f:
        f.write(
            "\n".join(
                [
                    "[reticulum]",
                    "enable_transport = yes",
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


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR") or tempfile.mkdtemp(
        prefix="rnx_listen_"
    )
    write_config(cfg_dir, listen_port, forward_port)

    reticulum = RNS.Reticulum(cfg_dir)
    _ = reticulum
    rnx_mod.prepare_identity(None)
    destination = RNS.Destination(
        rnx_mod.identity,
        RNS.Destination.IN,
        RNS.Destination.SINGLE,
        rnx_mod.APP_NAME,
        "execute",
    )
    rnx_mod.allow_all = True
    destination.set_link_established_callback(rnx_mod.command_link_established)
    destination.register_request_handler(
        path="command",
        response_generator=rnx_mod.execute_received_command,
        allow=RNS.Destination.ALLOW_ALL,
    )
    destination.announce()
    sys.stdout.write("READY " + destination.hash.hex() + "\n")
    sys.stdout.flush()
    while True:
        time.sleep(1)


if __name__ == "__main__":
    raise SystemExit(main())
