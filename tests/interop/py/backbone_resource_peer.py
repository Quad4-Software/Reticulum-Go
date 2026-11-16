#!/usr/bin/env python3
"""RNS resource peer over a Backbone client interface, for live e2e tests.

Connects to a public backbone as a client, announces a destination, and
either receives a resource from an established link (mode=recv) or links
to a remote destination and sends one (mode=send).

Environment:
    INTEROP_MODE            recv (default) or send
    INTEROP_BACKBONE_HOST   backbone server host
    INTEROP_BACKBONE_PORT   backbone server port
    INTEROP_TARGET_HASH     remote destination hex hash (mode=send)
    INTEROP_PAYLOAD_BYTES   payload size (mode=send, default 64000)
"""

import hashlib
import os
import sys
import tempfile
import threading
import time

import RNS
from RNS.Interfaces.BackboneInterface import BackboneClientInterface

APP_NAME = "vegaapp"
ASPECT = "resource"
TIMEOUT = 300


def log(msg):
    sys.stdout.write(msg + "\n")
    sys.stdout.flush()


def build_backbone_client(host, port):
    # Standalone (non-spawned) backbone clients never run the config path
    # that normally sets ifac_size, and the server starts pushing packets
    # during __init__ — before any instance attribute could be set. Patch
    # the class default first.
    if not hasattr(BackboneClientInterface, "ifac_size"):
        BackboneClientInterface.ifac_size = 0
    # Attributes the config path would normally set; Transport reads them
    # for inbound announce rate limiting.
    if not hasattr(BackboneClientInterface, "announce_rate_target"):
        BackboneClientInterface.announce_rate_target = None
    client = BackboneClientInterface(
        RNS.Transport,
        {
            "name": "vega_client",
            "target_host": host,
            "target_port": port,
            "connect_timeout": 20,
        },
    )
    client.OUT = True
    return client


def wait_online(client, timeout=60):
    deadline = time.time() + timeout
    while time.time() < deadline:
        if getattr(client, "online", False):
            return True
        time.sleep(0.5)
    return False


def recv_mode(client):
    got = threading.Event()
    state = {}

    def established(link):
        link.set_resource_strategy(RNS.Link.ACCEPT_ALL)

        def concluded(resource):
            data = resource.data if hasattr(resource, "data") else resource
            digest = hashlib.sha256(bytes(data)).hexdigest()
            state["len"] = len(data)
            state["sha256"] = digest
            log("RESOURCE_OK %d %s" % (len(data), digest))
            got.set()

        link.set_resource_concluded_callback(concluded)

    identity = RNS.Identity()
    dest = RNS.Destination(identity, RNS.Destination.IN, RNS.Destination.SINGLE, APP_NAME, ASPECT)
    dest.accepts_links(True)
    dest.set_link_established_callback(established)
    dest.announce()
    log("READY")
    log("HASH " + dest.hexhash)
    if not got.wait(TIMEOUT):
        log("RESOURCE_TIMEOUT")
        return 1
    return 0


def send_mode(client, target_hex):
    identity = RNS.Identity()
    own = RNS.Destination(identity, RNS.Destination.IN, RNS.Destination.SINGLE, APP_NAME, ASPECT)
    own.accepts_links(True)
    own.announce()
    log("READY")
    log("HASH " + own.hexhash)

    target_hash = bytes.fromhex(target_hex)
    deadline = time.time() + TIMEOUT
    while time.time() < deadline:
        if RNS.Transport.has_path(target_hash):
            break
        time.sleep(1)
    else:
        log("PATH_TIMEOUT")
        return 1

    remote_identity = RNS.Identity.recall(target_hash)
    if remote_identity is None:
        log("RECALL_FAIL")
        return 1
    remote = RNS.Destination(remote_identity, RNS.Destination.OUT, RNS.Destination.SINGLE, APP_NAME, ASPECT)
    link = RNS.Link(remote)
    established = threading.Event()
    link.set_link_established_callback(lambda _: established.set())
    if not established.wait(TIMEOUT):
        log("LINK_TIMEOUT")
        return 1

    payload = os.urandom(int(os.environ.get("INTEROP_PAYLOAD_BYTES", "64000")))
    digest = hashlib.sha256(payload).hexdigest()
    resource = RNS.Resource(payload, link)
    deadline = time.time() + TIMEOUT
    while time.time() < deadline:
        if resource.status >= RNS.Resource.COMPLETE:
            break
        time.sleep(0.5)
    if resource.status < RNS.Resource.COMPLETE:
        log("RESOURCE_TIMEOUT")
        return 1
    log("RESOURCE_OK %d %s" % (len(payload), digest))
    return 0


def main():
    mode = os.environ.get("INTEROP_MODE", "recv")
    host = os.environ["INTEROP_BACKBONE_HOST"]
    port = int(os.environ["INTEROP_BACKBONE_PORT"])

    cfg_dir = tempfile.mkdtemp(prefix="rns_vega_peer_")
    # RNS loglevel: 0=critical .. 5=debug, 6=verbose.
    rns_loglevel = int(os.environ.get("INTEROP_RNS_LOGLEVEL", "4"))
    logdest = None
    if os.environ.get("INTEROP_RNS_LOG_STDOUT") == "1":
        logdest = lambda msg: (sys.stdout.write(str(msg) + "\n"), sys.stdout.flush())
    RNS.Reticulum(cfg_dir, loglevel=rns_loglevel, logdest=logdest)

    client = build_backbone_client(host, port)
    RNS.Transport.interfaces.append(client)
    if not wait_online(client):
        log("CONNECT_FAIL")
        return 1

    if mode == "send":
        return send_mode(client, os.environ["INTEROP_TARGET_HASH"])
    return recv_mode(client)


if __name__ == "__main__":
    sys.exit(main())
