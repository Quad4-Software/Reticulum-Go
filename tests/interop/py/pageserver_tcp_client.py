#!/usr/bin/env python3
"""Fetch a page from a Reticulum-Go pageserver over a TCP hub.

Environment variables:
    INTEROP_GO_DEST_HASH       hex destination hash of the Go pageserver
    INTEROP_REQUEST_PATH       request path, default /page/index.mu
    INTEROP_TIMEOUT_SEC        overall timeout, default 90
    INTEROP_TCP_HOST           TCP hub host, default rns.beleth.net
    INTEROP_TCP_PORT           TCP hub port, default 4242
    INTEROP_CONFIG_DIR         optional persistent config directory
    RETICULUM_PATH             optional alt RNS install to prepend to sys.path
"""

import os
import sys
import tempfile
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS  # noqa: E402

PAGE_APP = "nomadnetwork"
PAGE_ASPECT = "node"


def write_config(cfg_dir, tcp_host, tcp_port, iface_type="tcp"):
    config_path = os.path.join(cfg_dir, "config")
    iface_name = "Directory Hub"
    if iface_type == "backbone":
        iface_block = [
            f"[[{iface_name}]]",
            "type = BackboneInterface",
            "enabled = yes",
            "remote = " + tcp_host,
            "target_host = " + tcp_host,
            "target_port = " + str(tcp_port),
        ]
    else:
        iface_block = [
            f"[[{iface_name}]]",
            "type = TCPClientInterface",
            "enabled = yes",
            "target_host = " + tcp_host,
            "target_port = " + str(tcp_port),
        ]
    with open(config_path, "w", encoding="utf-8") as f:
        f.write(
            "\n".join(
                [
                    "[reticulum]",
                    "enable_transport = false",
                    "share_instance = no",
                    "loglevel = 7",
                    "",
                    "[interfaces]",
                    "",
                    *iface_block,
                    "",
                ]
            )
        )


def peer_destination(go_hash):
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


def main():
    go_hash_hex = os.environ.get("INTEROP_GO_DEST_HASH", "").strip()
    if not go_hash_hex:
        sys.stderr.write("INTEROP_GO_DEST_HASH is required\n")
        return 2
    go_hash = bytes.fromhex(go_hash_hex)
    request_path = os.environ.get("INTEROP_REQUEST_PATH", "/page/index.mu").strip()
    timeout_sec = float(os.environ.get("INTEROP_TIMEOUT_SEC", "90"))
    tcp_host = os.environ.get("INTEROP_TCP_HOST", "rns.beleth.net").strip()
    tcp_port = int(os.environ.get("INTEROP_TCP_PORT", "4242"))
    iface_type = os.environ.get("INTEROP_HUB_TYPE", "tcp").strip().lower()
    if iface_type not in ("tcp", "backbone"):
        iface_type = "tcp"

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_pageserver_tcp_")
    os.makedirs(cfg_dir, exist_ok=True)
    write_config(cfg_dir, tcp_host, tcp_port, iface_type=iface_type)

    sys.stdout.write("config_dir=" + cfg_dir + "\n")
    sys.stdout.write(
        "connecting tcp_host=" + tcp_host + " tcp_port=" + str(tcp_port) + "\n"
    )
    sys.stdout.flush()

    log_path = os.path.join(cfg_dir, "rns.log")
    RNS.loglevel = 7
    RNS.logdest = RNS.LOG_FILE
    RNS.logfile = log_path
    sys.stdout.write("rns_log=" + log_path + "\n")
    sys.stdout.flush()

    RNS.Reticulum(cfg_dir)

    sys.stdout.write("RNS up; recalling destination " + go_hash_hex + "\n")
    sys.stdout.flush()

    deadline = time.time() + timeout_sec
    dest = None
    last_request = 0.0
    while time.time() < deadline:
        dest = peer_destination(go_hash)
        if dest is not None:
            break
        if time.time() - last_request > 5.0:
            sys.stdout.write("requesting path...\n")
            sys.stdout.flush()
            RNS.Transport.request_path(go_hash)
            last_request = time.time()
        time.sleep(0.25)

    if dest is None:
        sys.stderr.write("timeout: could not recall pageserver identity\n")
        return 1

    sys.stdout.write("destination resolved; establishing link\n")
    sys.stdout.flush()

    state = {"done": False, "ok": False, "response": b""}

    def on_response(receipt):
        try:
            response = receipt.response or b""
            if isinstance(response, str):
                response = response.encode("utf-8")
            state["response"] = response
            state["ok"] = True
            state["done"] = True
            sys.stdout.write(
                "RESPONSE bytes=" + str(len(response)) + "\n--- begin ---\n"
            )
            sys.stdout.write(response.decode("utf-8", errors="replace"))
            sys.stdout.write("\n--- end ---\n")
            sys.stdout.flush()
        except Exception as exc:
            state["ok"] = False
            state["done"] = True
            sys.stderr.write("response callback error: " + str(exc) + "\n")
            sys.stderr.flush()

    def on_link_established(link):
        sys.stdout.write(
            "link established id=" + RNS.prettyhexrep(link.link_id) + "\n"
        )
        sys.stdout.flush()
        try:
            link.request(request_path, b"ping", response_callback=on_response)
        except Exception as exc:
            state["ok"] = False
            state["done"] = True
            sys.stderr.write("request send error: " + str(exc) + "\n")
            sys.stderr.flush()

    def on_link_closed(link):
        if not state["done"]:
            sys.stderr.write("link closed before response\n")
            state["done"] = True

    link = RNS.Link(dest, established_callback=on_link_established, closed_callback=on_link_closed)
    _ = link

    while time.time() < deadline:
        if state["done"]:
            return 0 if state["ok"] else 1
        time.sleep(0.1)

    sys.stderr.write(
        "timeout waiting for response after " + str(timeout_sec) + " seconds\n"
    )
    return 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
