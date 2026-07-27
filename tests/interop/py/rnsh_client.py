#!/usr/bin/env python3
"""Python rnsh initiator against a Go --compat listener."""

import asyncio
import os
import sys
import tempfile

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

try:
    from RNS.Utilities.rnsh import initiator as rnsh_initiator
except ImportError as e:
    sys.stderr.write(f"rnsh import failed: {e}\n")
    raise SystemExit(77)


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
                ]
            )
        )


async def main_async() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    dest_hex = os.environ["INTEROP_DEST_HASH"].strip()
    command = os.environ.get("INTEROP_COMMAND", "/bin/echo py-client-ok").split()
    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR") or tempfile.mkdtemp(prefix="rnsh_client_")
    write_config(cfg_dir, listen_port, forward_port)
    identity_path = os.path.join(cfg_dir, "rnsh_client_identity")

    code = await rnsh_initiator.initiate(
        configdir=cfg_dir,
        rnsconfigdir=cfg_dir,
        identitypath=identity_path,
        logfile=None,
        verbosity=0,
        quietness=2,
        noid=True,
        destination=dest_hex,
        timeout=30.0,
        command=command,
    )
    return int(code or 0)


def main() -> int:
    return asyncio.run(main_async())


if __name__ == "__main__":
    raise SystemExit(main())
