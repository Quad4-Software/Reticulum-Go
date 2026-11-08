#!/usr/bin/env python3
"""Validates a Go-built stamped path request and stamped LR key via Python
RNS.Stamps / RNS.Transport.validate_pr_stamp."""

import sys
import os
import json

_reticulum_path = os.environ.get(
    "RETICULUM_PATH",
    os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", "reticulum-ref")),
)
sys.path.insert(0, _reticulum_path)

import RNS
import RNS.Stamps as Stamps
from RNS.Transport import Transport
from RNS.Cryptography import X25519PrivateKey

Transport.PR_COST = 8

with open(sys.argv[1]) as f:
    v = json.load(f)

dest_hash = bytes.fromhex(v["dest_hash_hex"])
tag = bytes.fromhex(v["tag_hex"])
stamp = bytes.fromhex(v["pr_stamp_hex"])
sig_pub = bytes.fromhex(v["sig_pub_hex"])
lr_priv = bytes.fromhex(v["lr_priv_hex"])
lr_pub = bytes.fromhex(v["lr_pub_hex"])
cost = float(v["cost"])

if not Transport.validate_pr_stamp(stamp, dest_hash, tag, required_value=cost):
    sys.exit("pr stamp invalid")

material = dest_hash + sig_pub
if not Stamps.stamp_valid(lr_pub, cost, material):
    sys.exit("lr stamp invalid")
if Stamps.stamp_value(material, lr_pub) < cost:
    sys.exit("lr stamp value below cost")
if X25519PrivateKey.from_private_bytes(lr_priv).public_key().public_bytes() != lr_pub:
    sys.exit("lr priv/pub mismatch")

print("OK")
