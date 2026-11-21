#!/usr/bin/env python3
# SPDX-License-Identifier: LicenseRef-Reticulum
"""Decode a Go-produced LXST codec2 wire frame for interop tests."""

import json
import sys

from RNS.vendor import umsgpack as mp
from LXST.Primitives.Telephony import Profiles


def main() -> int:
    if len(sys.argv) != 2:
        print(json.dumps({"ok": False, "error": "usage: codec2_decode.py <wire_hex>"}))
        return 1
    try:
        wire = bytes.fromhex(sys.argv[1])
        pkt = mp.unpackb(wire)
        frame = pkt[1]
        codec = Profiles.get_codec(Profiles.BANDWIDTH_LOW)
        codec.source = type("S", (), {"samplerate": 8000, "channels": 1})()
        body = bytes(frame[1:])
        pcm = codec.decode(body)
        samples = int(pcm.shape[0])
        print(json.dumps({"ok": True, "samples": samples}))
        return 0
    except Exception as e:
        print(json.dumps({"ok": False, "error": str(e)}))
        return 1


if __name__ == "__main__":
    sys.exit(main())
