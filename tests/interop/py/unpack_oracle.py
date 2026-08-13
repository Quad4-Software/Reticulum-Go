#!/usr/bin/env python3
"""Unpack or round-trip a hex RNS packet with Python RNS.Packet.

Usage:
    python3 unpack_oracle.py <raw_hex>
    python3 unpack_oracle.py roundtrip <raw_hex>

unpack prints UNPACK_OK or UNPACK_FAIL.
roundtrip unpacks then packs and prints PACKED <hex>, or UNPACK_FAIL.
"""

import os
import sys

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

from RNS.Packet import Packet


def read_raw(arg: str) -> bytes:
    if arg == "-":
        return bytes.fromhex(sys.stdin.read().strip())
    if arg.startswith("@"):
        with open(arg[1:], "r", encoding="utf-8") as f:
            return bytes.fromhex(f.read().strip())
    return bytes.fromhex(arg)


def unpack_raw(raw: bytes):
    pkt = Packet(None, None, create_receipt=False)
    pkt.raw = raw
    try:
        ok = bool(pkt.unpack())
    except Exception:
        return None
    if not ok:
        return None
    return pkt


class _StubDest:
    def __init__(self, dest_hash, dest_type):
        self.hash = dest_hash
        self.type = dest_type

    def encrypt(self, data):
        return data


def pack_unpacked(pkt) -> bytes:
    pkt.destination = _StubDest(pkt.destination_hash, pkt.destination_type)
    pkt.ciphertext = pkt.data
    pkt.packed = False
    pkt.pack()
    return pkt.raw


def main() -> int:
    if len(sys.argv) == 2:
        raw = read_raw(sys.argv[1])
        pkt = unpack_raw(raw)
        if pkt is None:
            sys.stdout.write("UNPACK_FAIL\n")
            return 1
        sys.stdout.write("UNPACK_OK\n")
        return 0
    if len(sys.argv) == 3 and sys.argv[1] == "roundtrip":
        raw = read_raw(sys.argv[2])
        pkt = unpack_raw(raw)
        if pkt is None:
            sys.stdout.write("UNPACK_FAIL\n")
            return 1
        packed = pack_unpacked(pkt)
        sys.stdout.write("PACKED %s\n" % packed.hex())
        return 0
    print("usage: unpack_oracle.py raw_hex | unpack_oracle.py roundtrip raw_hex", file=sys.stderr)
    return 2


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
