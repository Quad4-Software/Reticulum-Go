# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

from __future__ import annotations

from .errors import Error


def hash_to_hex(data: bytes | bytearray | memoryview) -> str:
    raw = bytes(data)
    if len(raw) != 16:
        raise Error(Error.INVALID_ARG)
    return raw.hex()


def hex_to_hash(text: str) -> bytes:
    if len(text) != 32:
        raise Error(Error.INVALID_ARG)
    try:
        return bytes.fromhex(text)
    except ValueError as exc:
        raise Error(Error.INVALID_ARG) from exc
