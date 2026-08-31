#!/usr/bin/env python3
"""Python rnsh initiator against a Go --compat listener."""

import os
import subprocess
import sys
import tempfile

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))


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
    """Drive the rnsh CLI under a PTY so asyncio stdin registration works."""
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    dest_hex = os.environ["INTEROP_DEST_HASH"].strip()
    command = os.environ.get("INTEROP_COMMAND", "/bin/echo py-client-ok")
    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR") or tempfile.mkdtemp(
        prefix="rnsh_client_"
    )
    write_config(cfg_dir, listen_port, forward_port)

    rnsh = os.environ.get("RNSH_BIN", "rnsh")
    argv = [
        rnsh,
        "--config",
        cfg_dir,
        "--rnsconfig",
        cfg_dir,
        "-N",
        "-m",
        "-w",
        "30",
        dest_hex,
        "--",
        *command.split(),
    ]
    # Hardened kernels may EPERM asyncio add_reader on pipes. script(1) gives a PTY.
    wrapped = ["script", "-q", "-e", "-c", subprocess.list2cmdline(argv), "/dev/null"]
    try:
        proc = subprocess.run(wrapped, check=False, capture_output=True)
    except FileNotFoundError:
        proc = subprocess.run(argv, check=False, capture_output=True)
    sys.stdout.buffer.write(proc.stdout)
    sys.stderr.buffer.write(proc.stderr)
    return int(proc.returncode)


if __name__ == "__main__":
    raise SystemExit(main())
