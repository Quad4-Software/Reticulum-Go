#!/usr/bin/env python3
"""Chained-node helper for Go-Python interop mesh tests (loopback only).

Environment:
    CHAIN_ROLE       "relay" or "end" (required)
    INTEROP_CONFIG_DIR optional config dir (else mkdtemp)
    RETICULUM_PATH   optional RNS source tree on sys.path

    ROLE=relay:
        UP_PORT      BackboneInterface client target (upstream server port)
        DOWN_PORT    BackboneInterface listen port (downstream server port)

    ROLE=end:
        PORT         BackboneInterface client target
        APP          app name for the announced destination
        ASPECT       aspect for the announced destination
        ANNOUNCE     "1" to re-announce the destination every 5s

Stdin commands (one per line):
    WATCH <hexhash>    start reporting PATH <hash> <hops> lines
    REQUEST <hexhash>  send a single path request for the hash
    ANNOUNCE           announce the destination once

Prints READY, DEST <hash>, PATH <hash> <hops> on stdout.
"""

import os
import sys
import tempfile
import threading
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS  # noqa: E402
RNS.Transport.DESTINATION_VALUE = float(os.environ.get("DEST_VALUE", "8"))
RNS.Transport.DESTINATION_COST = float(os.environ.get("DEST_COST", "8"))

watched = set()


def stdin_loop(destination):
    for line in sys.stdin:
        parts = line.split()
        if not parts:
            continue
        if parts[0] == "WATCH" and len(parts) == 2:
            watched.add(bytes.fromhex(parts[1]))
        elif parts[0] == "REQUEST" and len(parts) == 2:
            RNS.Transport.request_path(bytes.fromhex(parts[1]))
        elif parts[0] == "ANNOUNCE":
            destination.announce()


def write_config(cfg_dir, sections):
    config_path = os.path.join(cfg_dir, "config")
    with open(config_path, "w", encoding="utf-8") as f:
        f.write("\n".join(sections) + "\n")


def base():
    return [
        "[reticulum]",
        "enable_transport = true",
        "share_instance = no",
        "loglevel = 4",
        "",
        "[interfaces]",
        "",
    ]


def main() -> int:
    role = os.environ["CHAIN_ROLE"]
    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_chain_")

    if role == "relay":
        up = int(os.environ["UP_PORT"])
        down = int(os.environ["DOWN_PORT"])
        write_config(
            cfg_dir,
            base()
            + [
                "[[chain_up]]",
                "type = BackboneInterface",
                "enabled = yes",
                "remote = 127.0.0.1",
                f"target_port = {up}",
                "",
                "[[chain_down]]",
                "type = BackboneInterface",
                "enabled = yes",
                "listen_ip = 127.0.0.1",
                f"listen_port = {down}",
            ],
        )
        RNS.Reticulum(cfg_dir)
        sys.stdout.write("READY\n")
        sys.stdout.flush()
        while True:
            time.sleep(60)
    else:
        port = int(os.environ["PORT"])
        write_config(
            cfg_dir,
            base()
            + [
                "[[chain_peer]]",
                "type = BackboneInterface",
                "enabled = yes",
                "remote = 127.0.0.1",
                f"target_port = {port}",
            ],
        )
        RNS.Reticulum(cfg_dir)

        identity = RNS.Identity()
        destination = RNS.Destination(
            identity,
            RNS.Destination.IN,
            RNS.Destination.SINGLE,
            os.environ["APP"],
            os.environ["ASPECT"],
        )
        threading.Thread(target=stdin_loop, args=(destination,), daemon=True).start()

        sys.stdout.write("READY\n")
        sys.stdout.write("DEST " + destination.hash.hex() + "\n")
        sys.stdout.flush()

        last_announce = 0.0
        reported = {}
        while True:
            time.sleep(0.5)
            if os.environ.get("ANNOUNCE") == "1" and time.time() - last_announce > 5.0:
                last_announce = time.time()
                destination.announce()
            for h in list(watched):
                hops = RNS.Transport.hops_to(h)
                if hops is not None and hops < RNS.Transport.PATHFINDER_M and reported.get(h) != hops:
                    reported[h] = hops
                    sys.stdout.write("PATH %s %d\n" % (h.hex(), hops))
                    sys.stdout.flush()


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
