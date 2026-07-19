# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

from __future__ import annotations

import ctypes
from dataclasses import dataclass

from . import ffi
from .errors import Error, map_code
from .node import Node


@dataclass
class PathInfo:
    hash: bytes
    via: bytes
    hops: int
    iface: str
    timestamp: float
    expires: float


def path_request(node: Node, dest_hash: bytes | bytearray | memoryview) -> None:
    data = bytes(dest_hash)
    if len(data) != ffi.HASH_LEN:
        raise Error(Error.INVALID_ARG)
    buf = (ctypes.c_uint8 * ffi.HASH_LEN).from_buffer_copy(data)
    map_code(ffi.lib.rns_path_request(node.handle, buf))


def path_table(node: Node, capacity: int = 256, max_hops: int = -1) -> list[PathInfo]:
    if capacity <= 0:
        raise Error(Error.INVALID_ARG)
    raw = (ffi.RnsPathEntry * capacity)()
    written = ctypes.c_size_t(0)
    code = ffi.lib.rns_path_table(node.handle, raw, capacity, ctypes.byref(written), int(max_hops))
    if code not in (Error.OK, Error.TRUNCATED):
        map_code(code)
    out: list[PathInfo] = []
    for i in range(written.value):
        entry = raw[i]
        out.append(
            PathInfo(
                hash=bytes(entry.hash[: entry.hash_len]),
                via=bytes(entry.via[: entry.via_len]),
                hops=int(entry.hops),
                iface=ffi.cstr_field(entry.iface),
                timestamp=float(entry.timestamp),
                expires=float(entry.expires),
            )
        )
    return out


def path_known(node: Node, dest_hash: bytes | bytearray | memoryview) -> bool:
    target = bytes(dest_hash)
    if len(target) != ffi.HASH_LEN:
        return False
    try:
        return any(entry.hash == target for entry in path_table(node))
    except Error:
        return False
