#!/usr/bin/env python3
# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

import json
import sys
from typing import Any


def _handle_unpack(req: dict[str, Any]) -> dict[str, Any]:
    import lxmf

    packed = bytes.fromhex(req["packed"])
    lxmf.identity_register_recall_source(
        bytes.fromhex(req["source_hash"]),
        bytes.fromhex(req["public_key"]),
    )
    msg = lxmf.message_unpack_verified(packed)
    try:
        return {
            "ok": True,
            "content": lxmf.message_get_content(msg),
            "title": lxmf.message_get_title(msg),
            "field_count": lxmf.message_field_count(msg),
            "fields": lxmf.message_fields_json(msg),
        }
    finally:
        lxmf.message_destroy(msg)


def _handle_pack(req: dict[str, Any]) -> dict[str, Any]:
    import lxmf

    identity = lxmf.identity_generate()
    try:
        dest = lxmf.identity_delivery_hash(identity)
        source = lxmf.identity_delivery_hash(identity)
        msg = lxmf.message_create(dest, source, req.get("title", ""), req.get("content", ""))
        try:
            fields = req.get("fields")
            if fields:
                lxmf.message_set_fields_json(msg, fields)
            lxmf.identity_register_recall(identity)
            packed = lxmf.message_pack(msg, identity)
            return {
                "ok": True,
                "packed": packed.hex(),
                "source_hash": source.hex(),
                "public_key": lxmf.identity_public_key(identity).hex(),
            }
        finally:
            lxmf.message_destroy(msg)
    finally:
        lxmf.identity_destroy(identity)


def main() -> int:
    try:
        req = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        print(json.dumps({"ok": False, "error": f"invalid json: {exc}"}))
        return 1
    cmd = req.get("cmd")
    try:
        if cmd == "unpack":
            out = _handle_unpack(req)
        elif cmd == "pack":
            out = _handle_pack(req)
        else:
            out = {"ok": False, "error": f"unknown cmd {cmd!r}"}
    except Exception as exc:  # noqa: BLE001
        out = {"ok": False, "error": str(exc)}
    print(json.dumps(out))
    return 0 if out.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
