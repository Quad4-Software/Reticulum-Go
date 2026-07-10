#!/usr/bin/env python3
"""Python rnx client against a Go (or Python) rnx.execute listener."""

import os
import sys
import tempfile
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS


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


def spin(until, timeout):
    deadline = time.time() + timeout
    while time.time() < deadline:
        if until():
            return True
        time.sleep(0.1)
    return False


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    dest_hex = os.environ["INTEROP_DEST_HASH"].strip()
    command = os.environ.get("INTEROP_COMMAND", "echo ok")
    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR") or tempfile.mkdtemp(prefix="rnx_client_")
    write_config(cfg_dir, listen_port, forward_port)

    RNS.Reticulum(cfg_dir)
    dest_hash = bytes.fromhex(dest_hex)

    if not RNS.Transport.has_path(dest_hash):
        RNS.Transport.request_path(dest_hash)
        if not spin(lambda: RNS.Transport.has_path(dest_hash), 30):
            sys.stderr.write("Path not found\n")
            return 242

    identity = RNS.Identity.recall(dest_hash)
    if identity is None:
        sys.stderr.write("Could not recall identity\n")
        return 242
    listener = RNS.Destination(
        identity,
        RNS.Destination.OUT,
        RNS.Destination.SINGLE,
        "rnx",
        "execute",
    )
    link = RNS.Link(listener)
    if not spin(lambda: link.status == RNS.Link.ACTIVE, 30):
        sys.stderr.write("Could not establish link\n")
        return 243

    timeout = 30.0
    request_data = [
        command.encode("utf-8"),
        timeout,
        None,
        None,
        None,
    ]
    rexec_timeout = timeout + link.rtt * 4 + 2.0
    done = {"ok": False}

    def on_done(receipt):
        done["ok"] = True
        done["receipt"] = receipt

    receipt = link.request(
        path="command",
        data=request_data,
        response_callback=on_done,
        failed_callback=on_done,
        timeout=rexec_timeout,
    )
    if receipt is None:
        sys.stderr.write("request failed\n")
        return 244

    if not spin(lambda: done["ok"] or receipt.status == RNS.RequestReceipt.FAILED, rexec_timeout + 5):
        sys.stderr.write("timeout waiting for result\n")
        return 245

    if receipt.status == RNS.RequestReceipt.FAILED or receipt.response is None:
        sys.stderr.write("no result\n")
        return 245

    executed = receipt.response[0]
    stdout = receipt.response[2] or b""
    stderr = receipt.response[3] or b""
    if executed:
        if stdout:
            sys.stdout.buffer.write(stdout)
            sys.stdout.buffer.flush()
        if stderr:
            sys.stderr.buffer.write(stderr)
            sys.stderr.buffer.flush()
        link.teardown()
        return 0
    sys.stderr.write("Remote could not execute command\n")
    return 248


if __name__ == "__main__":
    raise SystemExit(main())
