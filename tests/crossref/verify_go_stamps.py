#!/usr/bin/env python3
"""Verifies Go-generated stamps and stamped keys against Python RNS.Stamps.

Usage: verify_go_stamps.py <vectors.json>

The JSON file contains {"detached": [{material_hex, cost, stamp_hex}],
"keystamps": [{material_hex, cost, priv_hex, pub_hex}]} produced by the Go
pkg/stamps package. Exits non-zero if any entry fails Python validation.
"""

import sys
import os
import json

_reticulum_path = os.environ.get(
    "RETICULUM_PATH",
    os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", "reticulum-ref")),
)
sys.path.insert(0, _reticulum_path)

import RNS.Stamps as Stamps


def main():
    with open(sys.argv[1]) as f:
        vectors = json.load(f)

    failures = []
    for i, v in enumerate(vectors.get("detached", [])):
        material = bytes.fromhex(v["material_hex"])
        stamp = bytes.fromhex(v["stamp_hex"])
        if not Stamps.stamp_valid(stamp, v["cost"], material):
            failures.append(f"detached[{i}] invalid")
        value = Stamps.stamp_value(material, stamp)
        if value < v["cost"]:
            failures.append(f"detached[{i}] value {value} < cost {v['cost']}")

    for i, v in enumerate(vectors.get("keystamps", [])):
        material = bytes.fromhex(v["material_hex"])
        pub = bytes.fromhex(v["pub_hex"])
        priv = bytes.fromhex(v["priv_hex"])
        if not Stamps.stamp_valid(pub, v["cost"], material):
            failures.append(f"keystamps[{i}] invalid")
        value = Stamps.stamp_value(material, pub)
        if value < v["cost"]:
            failures.append(f"keystamps[{i}] value {value} < cost {v['cost']}")
        from RNS.Cryptography import X25519PrivateKey
        if X25519PrivateKey.from_private_bytes(priv).public_key().public_bytes() != pub:
            failures.append(f"keystamps[{i}] priv/pub mismatch")

    if failures:
        print("\n".join(failures))
        sys.exit(1)
    print(f"verified {len(vectors.get('detached', []))} stamps, {len(vectors.get('keystamps', []))} keystamps")


if __name__ == "__main__":
    main()
