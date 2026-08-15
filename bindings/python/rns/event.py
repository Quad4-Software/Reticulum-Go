# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

from __future__ import annotations

import ctypes

from . import ffi
from .errors import Error, map_code
from .node import Node


class EventKind:
    NONE = 0
    ANNOUNCE = 1
    LINK_ESTABLISHED = 2
    LINK_FAILED = 3
    LINK_DATA = 4
    LINK_CLOSED = 5
    REQUEST_INCOMING = 6
    REQUEST_RESPONSE = 7
    REQUEST_FAILED = 8
    RESOURCE_STARTED = 9
    RESOURCE_CONCLUDED = 10
    DESTINATION_DATA = 11


class Event:
    def __init__(self, raw: ffi.RnsEvent, app_data: bytes) -> None:
        self._raw = raw
        self._app_data = app_data

    @classmethod
    def poll(cls, node: Node, timeout_ms: int, app_data_buf: bytearray | None = None) -> Event:
        event = ffi.RnsEvent()
        if app_data_buf is not None and len(app_data_buf) > 0:
            buf = (ctypes.c_uint8 * len(app_data_buf)).from_buffer(app_data_buf)
            event.app_data = ctypes.cast(buf, ctypes.POINTER(ctypes.c_uint8))
            event.app_data_cap = len(app_data_buf)
        map_code(ffi.lib.rns_event_poll(node.handle, ctypes.byref(event), int(timeout_ms)))
        data = b""
        if event.app_data and event.app_data_len:
            data = bytes(event.app_data[: event.app_data_len])
        return cls(event, data)

    @property
    def kind(self) -> int:
        return int(self._raw.kind)

    @property
    def hops(self) -> int:
        return int(self._raw.hops)

    def link_id(self) -> bytes:
        return bytes(self._raw.link_id[: self._raw.link_id_len])

    def destination_hash(self) -> bytes:
        return bytes(self._raw.destination_hash[: self._raw.destination_hash_len])

    def identity_hash(self) -> bytes:
        return bytes(self._raw.identity_hash[: self._raw.identity_hash_len])

    def request_id(self) -> bytes:
        return bytes(self._raw.request_id[: self._raw.request_id_len])

    def path(self) -> str:
        return ffi.cstr_field(self._raw.path)

    def error_message(self) -> str:
        return ffi.cstr_field(self._raw.error_message)

    def app_data(self) -> bytes:
        return self._app_data

    @property
    def path_truncated(self) -> bool:
        return bool(self._raw.path_truncated)

    @property
    def error_message_truncated(self) -> bool:
        return bool(self._raw.error_message_truncated)

    @property
    def app_data_truncated(self) -> bool:
        return bool(self._raw.app_data_truncated)


def hash_eq(a: bytes, b: bytes) -> bool:
    return len(a) == ffi.HASH_LEN and len(b) == ffi.HASH_LEN and a == b
