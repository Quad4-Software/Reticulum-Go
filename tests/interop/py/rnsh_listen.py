#!/usr/bin/env python3
"""Python rnsh listener for Go --compat interop tests."""

import asyncio
import os
import sys
import tempfile

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

try:
    from RNS.Utilities.rnsh import listener as rnsh_listener
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
                ],
            ),
        )


async def main_async() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR") or tempfile.mkdtemp(
        prefix="rnsh_listen_"
    )
    command = os.environ.get("INTEROP_COMMAND", "/bin/echo py-rnsh-ok").split()
    write_config(cfg_dir, listen_port, forward_port)
    identity_path = os.path.join(cfg_dir, "rnsh_identity")

    listen_task = asyncio.create_task(
        rnsh_listener.listen(
            configdir=cfg_dir,
            rnsconfigdir=cfg_dir,
            command=command,
            identitypath=identity_path,
            verbosity=0,
            quietness=2,
            allowed=None,
            allowed_file=None,
            disable_auth=True,
            announce_period=None,
            no_remote_command=False,
        ),
    )

    for _ in range(100):
        dest = getattr(rnsh_listener, "_destination", None)
        if dest is not None:
            sys.stdout.write("READY " + dest.hash.hex() + "\n")
            sys.stdout.flush()
            break
        await asyncio.sleep(0.1)
    else:
        sys.stderr.write("rnsh listener destination not ready\n")
        listen_task.cancel()
        return 1

    await listen_task
    return 0


def main() -> int:
    return asyncio.run(main_async())


if __name__ == "__main__":
    raise SystemExit(main())
