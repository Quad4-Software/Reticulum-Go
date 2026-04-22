#!/usr/bin/env python3
"""
Python peer for the IFAC interop test. Brings up an RNS instance with an
Interface Access Code configured on the loopback UDP interface, announces a
single SINGLE destination, and stays alive until killed.

Required environment:
  INTEROP_LISTEN_PORT   UDP port to listen on (Python receives here)
  INTEROP_FORWARD_PORT  UDP port to forward to (Python sends here)
  INTEROP_NETNAME       network_name to set on the UDP interface
  INTEROP_NETKEY        passphrase to set on the UDP interface
  INTEROP_IFAC_SIZE     ifac_size in BYTES (will be written to the
                        Reticulum config in bits, since RNS expects ifac_size
                        as bits and divides by 8 internally).

The script writes "READY\\n<dest_hash_hex>\\n" to stdout once the destination
is ready, then announces every 5 seconds so a slowly-starting Go peer still
learns the path.
"""
import os
import sys
import tempfile
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS

INTEROP_APP = "interop_pygo"
INTEROP_ASPECT = "ifacsvc"


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    netname = os.environ["INTEROP_NETNAME"]
    netkey = os.environ["INTEROP_NETKEY"]
    ifac_size_bytes = int(os.environ.get("INTEROP_IFAC_SIZE", "16"))
    ifac_size_bits = ifac_size_bytes * 8

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_ifac_")

    config_path = os.path.join(cfg_dir, "config")
    with open(config_path, "w", encoding="utf-8") as f:
        f.write(
            "\n".join(
                [
                    "[reticulum]",
                    "enable_transport = false",
                    "share_instance = no",
                    "loglevel = 4",
                    "",
                    "[interfaces]",
                    "",
                    "[[interop_ifac_udp]]",
                    "type = UDPInterface",
                    "enabled = yes",
                    "listen_ip = 127.0.0.1",
                    f"listen_port = {listen_port}",
                    "forward_ip = 127.0.0.1",
                    f"forward_port = {forward_port}",
                    f"network_name = {netname}",
                    f"passphrase = {netkey}",
                    f"ifac_size = {ifac_size_bits}",
                    "",
                ]
            )
        )

    RNS.Reticulum(cfg_dir)

    identity = RNS.Identity()
    destination = RNS.Destination(
        identity,
        RNS.Destination.IN,
        RNS.Destination.SINGLE,
        INTEROP_APP,
        INTEROP_ASPECT,
    )
    destination.set_proof_strategy(RNS.Destination.PROVE_ALL)

    h = destination.hash
    sys.stdout.write("READY\n")
    sys.stdout.write(h.hex() + "\n")
    sys.stdout.flush()

    while True:
        destination.announce()
        time.sleep(5.0)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
