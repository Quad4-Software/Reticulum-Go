#!/usr/bin/env python3
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
                ]
            )
        )


def peer_destination(go_hash: bytes):
    identity = RNS.Identity.recall(go_hash)
    if identity is None:
        return None
    return RNS.Destination(
        identity,
        RNS.Destination.OUT,
        RNS.Destination.SINGLE,
        PAGE_APP,
        PAGE_ASPECT,
    )


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    go_hash = bytes.fromhex(os.environ["INTEROP_GO_DEST_HASH"].strip())
    request_path = os.environ["INTEROP_REQUEST_PATH"].strip()
    expected_contains = os.environ["INTEROP_EXPECT_CONTAINS"].encode("utf-8")
    timeout_sec = float(os.environ.get("INTEROP_TIMEOUT_SEC", "90"))

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_interop_pageserver_")

    write_config(cfg_dir, listen_port, forward_port)
    log_path = os.path.join(cfg_dir, "rns.log")
    RNS.loglevel = 7
    RNS.logdest = RNS.LOG_FILE
    RNS.logfile = log_path
    RNS.Reticulum(cfg_dir)

    sys.stdout.write("READY rns_log=" + log_path + "\n")
    sys.stdout.flush()
    interop_events.emit("ready", detail=log_path)

    deadline = time.time() + timeout_sec
    dest = None
    interop_events.emit("path_wait", detail=go_hash.hex())
    while time.time() < deadline:
        dest = peer_destination(go_hash)
        if dest is not None:
            break
        RNS.Transport.request_path(go_hash)
        time.sleep(0.12)

    if dest is None:
        interop_events.emit(
            "fail",
            kind="identity",
            detail="timeout could not recall pageserver identity",
        )
        sys.stderr.write("timeout: could not recall pageserver identity\n")
        sys.stderr.write("rns_log=" + log_path + "\n")
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

            if expected_contains in response:
                state["ok"] = True
                state["done"] = True
                sys.stdout.write("REQUEST_OK\n")
                sys.stdout.flush()
                interop_events.emit("request_ok", detail=request_path)
            else:
                state["ok"] = False
                state["done"] = True
                interop_events.emit(
                    "fail",
                    kind="request",
                    detail="response mismatch for path " + request_path,
                )
                sys.stderr.write(
                    "response mismatch for path "
                    + request_path
                    + " len="
                    + str(len(response))
                    + "\n"
                )
                sys.stderr.write("rns_log=" + log_path + "\n")
                sys.stderr.flush()
        except Exception as exc:
            state["ok"] = False
            state["done"] = True
            interop_events.emit("fail", kind="request", detail=str(exc))
            sys.stderr.write("response callback error: " + str(exc) + "\n")
            sys.stderr.write("rns_log=" + log_path + "\n")
            sys.stderr.flush()

    def on_link_established(link):
        try:
            link.request(request_path, b"ping", response_callback=on_response)
        except Exception as exc:
            state["ok"] = False
            state["done"] = True
            interop_events.emit("fail", kind="request", detail=str(exc))
            sys.stderr.write("request send error: " + str(exc) + "\n")
            sys.stderr.write("rns_log=" + log_path + "\n")
            sys.stderr.flush()

    RNS.Link(dest, on_link_established)

    while time.time() < deadline:
        if state["done"]:
            return 0 if state["ok"] else 1
        time.sleep(0.1)

    interop_events.emit(
        "fail",
        kind="timeout",
        detail="timeout waiting for request response after " + str(timeout_sec) + " seconds",
    )
    sys.stderr.write(
        "timeout waiting for request response after " + str(timeout_sec) + " seconds\n"
    )
    sys.stderr.write("rns_log=" + log_path + "\n")
    return 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
