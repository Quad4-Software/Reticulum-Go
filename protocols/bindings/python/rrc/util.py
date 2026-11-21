# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

from . import ffi
from ._buffers import read_c_string
from .errors import map_code


def hash_to_hex(data: bytes) -> str:
    return data.hex()


def hex_to_hash(text: str) -> bytes:
    return bytes.fromhex(text)


def normalize_room(name: str) -> str:
    return read_c_string(
        lambda buf, cap, written: ffi.lib.rrc_normalize_room(
            name.encode("utf-8"), buf, cap, written
        )
    )


def sanitize_nick(nick: str) -> str:
    return read_c_string(
        lambda buf, cap, written: ffi.lib.rrc_sanitize_nick(
            nick.encode("utf-8"), buf, cap, written
        )
    )
