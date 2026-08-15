#!/usr/bin/env python3
"""Verify a Go-packed announce frame against Python RNS Identity.validate.

Usage:
    python3 verify_go_announce.py <raw_hex> <public_key_hex> <signed_data_hex> <signature_hex>

Exits 0 on success, 1 on verify failure or bad args.
"""

import sys
import os

_reticulum_path = os.environ.get(
    "RETICULUM_PATH",
    os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", "reticulum-ref")),
)
sys.path.insert(0, _reticulum_path)

from RNS.Identity import Identity
from RNS.Packet import Packet


def unpack_raw(raw: bytes) -> Packet:
    pkt = Packet(None, None, create_receipt=False)
    pkt.raw = raw
    if not pkt.unpack():
        raise RuntimeError("unpack returned false")
    return pkt


def main():
    if len(sys.argv) != 5:
        print("usage: verify_go_announce.py raw_hex pub_hex signed_hex sig_hex", file=sys.stderr)
        return 1
    raw = bytes.fromhex(sys.argv[1])
    pub = bytes.fromhex(sys.argv[2])
    signed = bytes.fromhex(sys.argv[3])
    sig = bytes.fromhex(sys.argv[4])

    try:
        pkt = unpack_raw(raw)
    except Exception as e:
        print(f"python unpack failed: {e}", file=sys.stderr)
        return 1

    if pkt.packet_type != Packet.ANNOUNCE:
        print(f"packet_type={pkt.packet_type} want ANNOUNCE", file=sys.stderr)
        return 1

    identity = Identity(create_keys=False)
    identity.load_public_key(pub)
    if not identity.validate(sig, signed):
        print("python signature verify failed", file=sys.stderr)
        return 1
    print("ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
