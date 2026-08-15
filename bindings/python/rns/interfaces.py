# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

from __future__ import annotations

import ctypes
from dataclasses import dataclass

from . import ffi
from .errors import Error, map_code
from .node import Node


@dataclass
class InterfaceInfo:
    name: str
    type_name: str
    online: bool
    enabled: bool
    rx_bytes: int
    tx_bytes: int
    rx_packets: int
    tx_packets: int


def interfaces_list(node: Node, capacity: int = 32) -> list[InterfaceInfo]:
    if capacity <= 0:
        raise Error(Error.INVALID_ARG)
    Array = ffi.RnsInterfaceEntry * capacity
    raw = Array()
    written = ctypes.c_size_t(0)
    code = ffi.lib.rns_interfaces(node.handle, raw, capacity, ctypes.byref(written))
    if code not in (Error.OK, Error.TRUNCATED):
        map_code(code)
    out: list[InterfaceInfo] = []
    for i in range(written.value):
        e = raw[i]
        out.append(
            InterfaceInfo(
                name=e.name.split(b"\0", 1)[0].decode("utf-8", errors="replace"),
                type_name=e.type_name.split(b"\0", 1)[0].decode("utf-8", errors="replace"),
                online=bool(e.online),
                enabled=bool(e.enabled),
                rx_bytes=int(e.rx_bytes),
                tx_bytes=int(e.tx_bytes),
                rx_packets=int(e.rx_packets),
                tx_packets=int(e.tx_packets),
            )
        )
    return out
