#!/usr/bin/env python3
"""Remote management client: link + identify + request /status.

Links to the Go rnstransport.remote.management destination, identifies
with the identity in INTEROP_IDENTITY_FILE, requests /status (or the
path in INTEROP_MGMT_PATH), and prints RESP_LEN n on a decoded msgpack
response. Exit 1 if the request is denied or times out.
"""

import os
import sys
import tempfile
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS

INTEROP_APP = "rnstransport"
INTEROP_ASPECT_A = "remote"
INTEROP_ASPECT_B = "management"


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    mgmt_hash = bytes.fromhex(os.environ["INTEROP_MGMT_HASH"].strip())
    id_file = os.environ["INTEROP_IDENTITY_FILE"]
    req_path = os.environ.get("INTEROP_MGMT_PATH", "/status")

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_interop_mg_")

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
                ],
            ),
        )

    RNS.Reticulum(cfg_dir)

    identity = RNS.Identity.from_file(id_file)
    if identity is None:
        sys.stderr.write("could not load identity\n")
        return 1

    sys.stdout.write("READY\n")
    sys.stdout.write(identity.hash.hex() + "\n")
    sys.stdout.flush()

    deadline = time.time() + 60.0
    dest = None
    while time.time() < deadline:
        peer_id = RNS.Identity.recall(mgmt_hash)
        if peer_id is not None:
            dest = RNS.Destination(
                peer_id,
                RNS.Destination.IN,
                RNS.Destination.SINGLE,
                INTEROP_APP,
                INTEROP_ASPECT_A,
                INTEROP_ASPECT_B,
            )
            break
        RNS.Transport.request_path(mgmt_hash)
        time.sleep(0.15)

    if dest is None:
        sys.stderr.write("could not recall mgmt identity\n")
        return 1

    done = []

    def on_established(link):
        link.identify(identity)
        sys.stdout.write("LINK_UP\n")
        sys.stdout.flush()

        def on_resp(receipt):
            done.append(len(receipt.response or b""))
            sys.stdout.write("RESP_LEN " + str(len(receipt.response or b"")) + "\n")
            sys.stdout.flush()

        if req_path == "/status":
            payload = [False, False]
        else:
            payload = ["table", None, 16]
        link.request(req_path, data=payload, response_callback=on_resp)

    RNS.Link(dest, on_established)

    end = time.time() + 20.0
    while time.time() < end and not done:
        time.sleep(0.1)

    if not done:
        sys.stderr.write("no response\n")
        return 1
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
