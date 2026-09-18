#!/usr/bin/env python3
"""Python rngit management client for Go interop tests.

Opens a link to a git.repositories destination, identifies, and issues a
sequence of management requests. Prints MGMT_OK when all checks pass.
"""

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

IDX_REPOSITORY = 0x00
IDX_GROUP = 0x02
RES_OK = 0x00

GIT_APP = "git"
GIT_ASPECT = "repositories"


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
    return RNS.Destination(identity, RNS.Destination.OUT, RNS.Destination.SINGLE, GIT_APP, GIT_ASPECT)


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    dest_hash = bytes.fromhex(os.environ["INTEROP_DEST_HASH"].strip())
    repo_path = os.environ.get("INTEROP_REPO_PATH", "public/demo")
    group = os.environ.get("INTEROP_GROUP", "public")
    timeout_sec = float(os.environ.get("INTEROP_TIMEOUT_SEC", "90"))

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR") or tempfile.mkdtemp(prefix="rngit_mgmt_")
    write_config(cfg_dir, listen_port, forward_port)
    log_path = os.path.join(cfg_dir, "rns.log")
    RNS.loglevel = 7
    RNS.logdest = RNS.LOG_FILE
    RNS.logfile = log_path
    RNS.Reticulum(cfg_dir)

    client_identity = RNS.Identity()

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
        interop_events.emit("fail", kind="identity", detail="could not recall git destination")
        sys.stderr.write("timeout: could not recall destination\n")
        return 1

    state = {"link": None, "done": False, "ok": False, "step": "link"}

    checks = [
        ("/mgmt/release", {IDX_REPOSITORY: repo_path, "operation": "list"}),
        ("/mgmt/perms", {IDX_GROUP: group, "operation": "gperms", "step": "get"}),
        ("/mgmt/work", {IDX_REPOSITORY: repo_path, "operation": "list", "scope": "all"}),
    ]
    state["checks"] = list(checks)

    def run_next(link):
        if not state["checks"]:
            state["done"] = True
            state["ok"] = True
            sys.stdout.write("MGMT_OK\n")
            sys.stdout.flush()
            interop_events.emit("request_ok", detail="all")
            return
        path, data = state["checks"].pop(0)
        state["step"] = path

        def on_response(receipt):
            try:
                response = receipt.response
                ok = False
                if isinstance(response, (bytes, bytearray)) and len(response) > 0:
                    ok = response[0] == RES_OK
                elif isinstance(response, (list, tuple)) and len(response) > 0:
                    ok = True  # unpacked payload responses
                if not ok:
                    state["done"] = True
                    interop_events.emit("fail", kind="request",
                                        detail=path + " -> " + repr(response)[:200])
                    sys.stderr.write("check failed " + path + ": " + repr(response)[:200] + "\n")
                    sys.stderr.flush()
                    return
                run_next(link)
            except Exception as exc:
                state["done"] = True
                interop_events.emit("fail", kind="request", detail=str(exc))
                sys.stderr.write("response error: " + str(exc) + "\n")
                sys.stderr.flush()

        def on_failed(receipt=None):
            state["done"] = True
            interop_events.emit("fail", kind="request", detail=path + " request failed")
            sys.stderr.write("request failed " + path + "\n")
            sys.stderr.flush()

        try:
            link.request(path, data, response_callback=on_response, failed_callback=on_failed)
        except Exception as exc:
            state["done"] = True
            interop_events.emit("fail", kind="request", detail=str(exc))
            sys.stderr.write("request send error " + path + ": " + str(exc) + "\n")
            sys.stderr.flush()

    def on_link_established(link):
        try:
            link.identify(client_identity)
        except Exception as exc:
            sys.stderr.write("identify error: " + str(exc) + "\n")
        # The identify packet must reach the responder before the first
        # request or the handler sees an unidentified peer.
        time.sleep(0.5)
        run_next(link)

    RNS.Link(dest, on_link_established)

    while time.time() < deadline:
        if state["done"]:
            return 0 if state["ok"] else 1
        time.sleep(0.1)

    interop_events.emit("fail", kind="timeout", detail="stuck at " + str(state["step"]))
    sys.stderr.write("timeout at step " + str(state["step"]) + "\n")
    return 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
