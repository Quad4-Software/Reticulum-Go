#!/usr/bin/env python3
"""Unpack a hex RNS packet with Python RNS.Packet and print UNPACK_OK or UNPACK_FAIL.

Usage:
    python3 unpack_oracle.py <raw_hex>
"""

import os
import sys

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

from RNS.Packet import Packet


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: unpack_oracle.py raw_hex", file=sys.stderr)
        return 2
    raw = bytes.fromhex(sys.argv[1])
    pkt = Packet(None, None, create_receipt=False)
    pkt.raw = raw
    try:
        ok = bool(pkt.unpack())
    except Exception:
        ok = False
    if ok:
        sys.stdout.write("UNPACK_OK\n")
        return 0
    sys.stdout.write("UNPACK_FAIL\n")
    return 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
