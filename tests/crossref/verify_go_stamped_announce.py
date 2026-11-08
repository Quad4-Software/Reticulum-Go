#!/usr/bin/env python3
"""Validates a Go-built stamped announce via RNS.Identity.validate_announce.

Usage: verify_go_stamped_announce.py <packet_raw_hex> <required_cost>

Builds a stub packet object with the attributes validate_announce reads and
prints the recorded known_destinations entry as JSON on success.
"""

import sys
import os
import json

_reticulum_path = os.environ.get(
    "RETICULUM_PATH",
    os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", "reticulum-ref")),
)
sys.path.insert(0, _reticulum_path)

import RNS


def main():
    raw = bytes.fromhex(sys.argv[1])
    required = float(sys.argv[2])

    class Iface:
        destination_cost = required

        def __str__(self):
            return "stub"

    p = type("P", (), {})()
    # HeaderType1 wire: header(2) + dest_hash(16) + context(1) + data
    p.packet_type = RNS.Packet.ANNOUNCE
    p.destination_hash = raw[2:18]
    context_flag = (raw[0] >> 5) & 0x01
    p.context_flag = RNS.Packet.FLAG_SET if context_flag == 1 else RNS.Packet.FLAG_UNSET
    p.data = raw[19:]
    p.hops = raw[1]
    p.packet_hash = RNS.Identity.full_hash(raw)
    p.receiving_interface = Iface()
    p.transport_id = None
    p.rssi = None
    p.snr = None
    p.q = None
    p.announce_signature_validated = False

    ok = RNS.Identity.validate_announce(p)
    if not ok:
        print("validate_announce failed", file=sys.stderr)
        sys.exit(1)

    dest_hash = raw[2:18]
    entry = RNS.Identity.known_destinations.get(dest_hash)
    if entry is None:
        print("destination not remembered", file=sys.stderr)
        sys.exit(1)
    print(json.dumps({
        "app_data": (entry[3] or b"").hex(),
        "stamp": (entry[5] or b"").hex(),
        "stamp_value": entry[6],
        "lr_cost": entry[7],
    }))


if __name__ == "__main__":
    main()
