#!/usr/bin/env python3
import hashlib
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

NOMADNET_APP = "nomadnetwork"
NOMADNET_ASPECT = "node"


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
                    "[[relay_udp]]",
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


def is_nomadnet_node(identity, dest_hash: bytes) -> bool:
    name_hash = hashlib.sha256(f"{NOMADNET_APP}.{NOMADNET_ASPECT}".encode("utf-8")).digest()[:10]
    identity_hash = RNS.Identity.truncated_hash(identity.get_public_key())
    expected = hashlib.sha256(name_hash + identity_hash).digest()[:16]
    return dest_hash == expected


class NomadnetCollector:
    # RNS expects a destination name string, not a list.
    aspect_filter = f"{NOMADNET_APP}.{NOMADNET_ASPECT}"
    receive_path_responses = True

    def __init__(self):
        self.nodes = {}

    def received_announce(self, dest_hash, announced_identity, app_data, announce_hops=0, *_args):
        if announced_identity is None or len(dest_hash) != 16:
            return
        if not is_nomadnet_node(announced_identity, dest_hash):
            return
        key = dest_hash.hex()
        if key in self.nodes:
            return
        self.nodes[key] = {
            "hash": dest_hash,
            "identity": announced_identity,
            "hops": announce_hops,
        }


def wait_for_nodes(collector, target: int, deadline: float):
    while time.time() < deadline:
        if len(collector.nodes) >= target:
            return list(collector.nodes.values())
        time.sleep(0.25)
    return list(collector.nodes.values())


def wait_for_path(dest_hash: bytes, deadline: float) -> bool:
    while time.time() < deadline:
        if RNS.Transport.has_path(dest_hash):
            return True
        RNS.Transport.request_path(dest_hash)
        time.sleep(0.2)
    return False


def wait_for_identity(dest_hash: bytes, deadline: float):
    while time.time() < deadline:
        identity = RNS.Identity.recall(dest_hash)
        if identity is not None:
            return identity
        RNS.Transport.request_path(dest_hash)
        time.sleep(0.2)
    return None


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    announce_wait = float(os.environ.get("INTEROP_NOMADNET_ANNOUNCE_WAIT_SEC", "90"))
    node_target = int(os.environ.get("INTEROP_NOMADNET_NODE_TARGET", "1"))
    page_path = os.environ.get("INTEROP_NOMADNET_PAGE_PATH", "/page/index.mu")
    link_timeout = float(os.environ.get("INTEROP_NOMADNET_LINK_TIMEOUT_SEC", "60"))
    request_timeout = float(os.environ.get("INTEROP_NOMADNET_REQUEST_TIMEOUT_SEC", "30"))
    preset_hash_hex = os.environ.get("INTEROP_NOMADNET_DEST_HASH", "").strip()

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_nomadnet_relay_")

    write_config(cfg_dir, listen_port, forward_port)
    RNS.Reticulum(cfg_dir)

    collector = NomadnetCollector()
    RNS.Transport.register_announce_handler(collector)

    sys.stdout.write("READY\n")
    sys.stdout.flush()
    interop_events.emit("ready")

    dest_hash = None
    identity = None
    if preset_hash_hex:
        dest_hash = bytes.fromhex(preset_hash_hex)
        if len(dest_hash) != 16:
            interop_events.emit("fail", kind="identity", detail="invalid INTEROP_NOMADNET_DEST_HASH")
            sys.stderr.write("invalid INTEROP_NOMADNET_DEST_HASH\n")
            return 1
        sys.stdout.write("NODE " + dest_hash.hex() + "\n")
        sys.stdout.flush()
        interop_events.emit("node", detail=dest_hash.hex())
        identity = wait_for_identity(dest_hash, time.time() + 45.0)
        if identity is None:
            interop_events.emit(
                "fail",
                kind="identity",
                detail="timeout could not recall nomadnet identity via path response",
            )
            sys.stderr.write("timeout: could not recall nomadnet identity via path response\n")
            return 1
    else:
        nodes = wait_for_nodes(collector, node_target, time.time() + announce_wait)
        if not nodes:
            interop_events.emit("fail", kind="announce", detail="timeout no nomadnet node announces observed")
            sys.stderr.write("timeout: no nomadnet node announces observed\n")
            return 1
        node = nodes[0]
        dest_hash = node["hash"]
        identity = node["identity"]
        sys.stdout.write("NODE " + dest_hash.hex() + "\n")
        sys.stdout.flush()
        interop_events.emit("node", detail=dest_hash.hex())

    if not wait_for_path(dest_hash, time.time() + 45.0):
        interop_events.emit("fail", kind="path", detail="timeout no path to nomadnet node")
        sys.stderr.write("timeout: no path to nomadnet node\n")
        return 1

    sys.stdout.write("PATH_OK\n")
    sys.stdout.flush()
    interop_events.emit("path_ok")

    link_result = {"ok": False, "err": ""}

    def on_link_established(link):
        def on_response(receipt):
            try:
                if receipt.response and len(receipt.response) > 0:
                    link_result["ok"] = True
                    sys.stdout.write("NOMADNET_LINK_OK\n")
                    sys.stdout.flush()
                    interop_events.emit("link_ok")
                else:
                    link_result["err"] = "empty page response"
            except Exception as exc:
                link_result["err"] = str(exc)

        try:
            link.request(page_path, b"crawl", response_callback=on_response, timeout=request_timeout)
        except Exception as exc:
            link_result["err"] = str(exc)

    dest = RNS.Destination(
        identity,
        RNS.Destination.OUT,
        RNS.Destination.SINGLE,
        NOMADNET_APP,
        NOMADNET_ASPECT,
    )
    RNS.Link(dest, on_link_established)

    deadline = time.time() + link_timeout
    while time.time() < deadline and not link_result["ok"] and not link_result["err"]:
        time.sleep(0.2)

    if link_result["ok"]:
        return 0
    if link_result["err"]:
        interop_events.emit("fail", kind="request", detail=link_result["err"])
        sys.stderr.write(link_result["err"] + "\n")
        return 1
    interop_events.emit("fail", kind="timeout", detail="link or page request did not complete")
    sys.stderr.write("timeout: link or page request did not complete\n")
    return 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
