# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

import json
import os
import unittest
from pathlib import Path

import lxmf

ROOT = Path(__file__).resolve().parents[3]
FIELDS_PATH = ROOT / "pkg" / "lxmf" / "testdata" / "messaging_interop_fields.json"

MESSAGING_FIELD_KEYS = [
    "0x08",
    "0x30",
    "0x31",
    "0x0f",
    "0x41",
    "0x42",
    "0x40",
    "0xfb",
    "0xfc",
    "0xfd",
    "0x0c",
    "0x07",
    "0x06",
    "0x04",
    "0x05",
    "0x02",
    "0x03",
    "0x09",
    "0x0a",
    "0x0b",
    "0x0d",
    "0x0e",
    "0xfe",
    "0xff",
]


def load_fields() -> dict:
    return json.loads(FIELDS_PATH.read_text(encoding="utf-8"))


class LXMFInteropTest(unittest.TestCase):
    def test_messaging_fields_roundtrip(self) -> None:
        fields = load_fields()
        identity = lxmf.identity_generate()
        try:
            dest = lxmf.identity_delivery_hash(identity)
            source = lxmf.identity_delivery_hash(identity)
            msg = lxmf.message_create(dest, source, "interop", "messaging fields")
            try:
                lxmf.message_set_fields_json(msg, fields)
                lxmf.identity_register_recall(identity)
                packed = lxmf.message_pack(msg, identity)
            finally:
                lxmf.message_destroy(msg)

            lxmf.identity_register_recall(identity)
            got = lxmf.message_unpack_verified(packed, identity)
            try:
                self.assertEqual(lxmf.message_get_content(got), "messaging fields")
                self.assertEqual(lxmf.message_get_title(got), "interop")
                self.assertGreaterEqual(lxmf.message_field_count(got), len(MESSAGING_FIELD_KEYS))
                got_fields = lxmf.message_fields_json(got)
                for key in MESSAGING_FIELD_KEYS:
                    self.assertIn(key, got_fields, key)
            finally:
                lxmf.message_destroy(got)
        finally:
            lxmf.identity_destroy(identity)

    def test_icon_and_attachments(self) -> None:
        fields = {
            "0x04": {
                "0x00": "hex:656d6f6a69",
                "0x01": "hex:89504e47",
            },
            "0x05": [
                {
                    "0x00": "hex:6e6f7465732e747874",
                    "0x01": "hex:66696c652d626f6479",
                }
            ],
        }
        identity = lxmf.identity_generate()
        try:
            dest = lxmf.identity_delivery_hash(identity)
            source = lxmf.identity_delivery_hash(identity)
            msg = lxmf.message_create(dest, source, "attach", "see files")
            try:
                lxmf.message_set_fields_json(msg, fields)
                lxmf.identity_register_recall(identity)
                packed = lxmf.message_pack(msg, identity)
            finally:
                lxmf.message_destroy(msg)

            lxmf.identity_register_recall(identity)
            got = lxmf.message_unpack_verified(packed, identity)
            try:
                got_fields = lxmf.message_fields_json(got)
                self.assertIn("0x04", got_fields)
                self.assertIn("0x05", got_fields)
            finally:
                lxmf.message_destroy(got)
        finally:
            lxmf.identity_destroy(identity)

    def test_go_fixture_env(self) -> None:
        fixture = os.environ.get("GO_LXMF_INTEROP_FIXTURE")
        if not fixture:
            self.skipTest("GO_LXMF_INTEROP_FIXTURE not set")
        data = json.loads(fixture)
        lxmf.identity_register_recall_source(
            bytes.fromhex(data["source_hash"]),
            bytes.fromhex(data["public_key"]),
        )
        got = lxmf.message_unpack_verified(bytes.fromhex(data["packed"]))
        try:
            self.assertEqual(lxmf.message_get_content(got), data["content"])
            self.assertGreaterEqual(lxmf.message_field_count(got), len(MESSAGING_FIELD_KEYS))
            got_fields = lxmf.message_fields_json(got)
            for key in MESSAGING_FIELD_KEYS:
                self.assertIn(key, got_fields, key)
        finally:
            lxmf.message_destroy(got)


if __name__ == "__main__":
    unittest.main()
