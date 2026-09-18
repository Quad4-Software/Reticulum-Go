#!/usr/bin/env python3
"""NomadNet-style page request client for rngit page interop tests.

Establishes a link to a nomadnetwork.node destination and issues one
request whose data is a dict of var_* fields. Prints REQUEST_OK when the
response contains INTEROP_EXPECT_CONTAINS.
"""

import json
import os
import sys
import tempfile
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import interop_events

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS

PAGE_APP = "nomadnetwork"
PAGE_ASPECT = "node"


def write_config(cfg_dir: str, listen_port: int, forward_port: int) -> None:
    with open(os.path.join(cfg_dir, "config"), "w", encoding="utf-8") as f:
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


def peer_destination(dest_hash: bytes):
    identity = RNS.Identity.recall(dest_hash)
    if identity is None:
        return None
    return RNS.Destination(identity, RNS.Destination.OUT, RNS.Destination.SINGLE, PAGE_APP, PAGE_ASPECT)


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    dest_hash = bytes.fromhex(os.environ["INTEROP_DEST_HASH"].strip())
    request_path = os.environ["INTEROP_REQUEST_PATH"].strip()
    expect = os.environ["INTEROP_EXPECT_CONTAINS"].encode("utf-8")
    page_vars = json.loads(os.environ.get("INTEROP_PAGE_VARS", "{}")) or {}
    timeout_sec = float(os.environ.get("INTEROP_TIMEOUT_SEC", "90"))

    data = {"var_" + k: v for k, v in page_vars.items()}

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR") or tempfile.mkdtemp(prefix="rngit_page_client_")
    write_config(cfg_dir, listen_port, forward_port)
    log_path = os.path.join(cfg_dir, "rns.log")
    RNS.loglevel = 7
    RNS.logdest = RNS.LOG_FILE
    RNS.logfile = log_path
    RNS.Reticulum(cfg_dir)

    sys.stdout.write("READY\n")
    sys.stdout.flush()
    interop_events.emit("ready", detail=log_path)

    deadline = time.time() + timeout_sec
    dest = None
    while time.time() < deadline:
        dest = peer_destination(dest_hash)
        if dest is not None:
            break
        RNS.Transport.request_path(dest_hash)
        time.sleep(0.12)
    if dest is None:
        interop_events.emit("fail", kind="identity", detail="could not recall page destination")
        sys.stderr.write("timeout: could not recall destination\n")
        return 1

    state = {"done": False, "ok": False}

    def on_response(receipt):
        try:
            response = receipt.response or b""
            if isinstance(response, (list, tuple)) and len(response) >= 2:
                response = response[1]
            if isinstance(response, str):
                response = response.encode("utf-8")
            elif not isinstance(response, (bytes, bytearray)):
                response = str(response).encode("utf-8", errors="replace")
            else:
                response = bytes(response)
            if expect in response:
                state["ok"] = True
                state["done"] = True
                sys.stdout.write("REQUEST_OK\n")
                sys.stdout.flush()
                interop_events.emit("request_ok", detail=request_path)
            else:
                state["done"] = True
                interop_events.emit("fail", kind="request", detail="response mismatch " + request_path)
                sys.stderr.write("response mismatch for " + request_path +
                                 " len=" + str(len(response)) + "\n")
                sys.stderr.write(response[:400].decode("utf-8", errors="replace") + "\n")
                sys.stderr.flush()
        except Exception as exc:
            state["done"] = True
            interop_events.emit("fail", kind="request", detail=str(exc))
            sys.stderr.write("response callback error: " + str(exc) + "\n")
            sys.stderr.flush()

    def on_link_established(link):
        try:
            link.request(request_path, data, response_callback=on_response)
        except Exception as exc:
            state["done"] = True
            interop_events.emit("fail", kind="request", detail=str(exc))
            sys.stderr.write("request send error: " + str(exc) + "\n")
            sys.stderr.flush()

    RNS.Link(dest, on_link_established)

    while time.time() < deadline:
        if state["done"]:
            return 0 if state["ok"] else 1
        time.sleep(0.1)

    interop_events.emit("fail", kind="timeout", detail="no response")
    sys.stderr.write("timeout waiting for page response\n")
    return 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
