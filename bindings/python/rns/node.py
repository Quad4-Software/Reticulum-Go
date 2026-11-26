# SPDX-License-Identifier: LicenseRef-Reticulum
# Copyright (c) 2024-2026 Quad4.io

from __future__ import annotations

from . import ffi
from .errors import Error, map_code
from .identity import Identity


class Node:
    def __init__(self, handle: int) -> None:
        self._handle = int(handle)

    @classmethod
    def create(cls, config_path: str = "") -> Node:
        h = ffi.lib.rns_node_create(config_path.encode("utf-8"))
        if h == 0:
            raise Error(Error.INTERNAL)
        return cls(h)

    def start(self) -> None:
        map_code(ffi.lib.rns_node_start(self._handle))

    def stop(self) -> None:
        map_code(ffi.lib.rns_node_stop(self._handle))

    def set_identity(self, identity: Identity) -> None:
        map_code(ffi.lib.rns_node_set_identity(self._handle, identity.handle))

    def pause(self) -> None:
        map_code(ffi.lib.rns_node_pause(self._handle))

    def resume(self) -> None:
        map_code(ffi.lib.rns_node_resume(self._handle))

    def reload_config(self) -> None:
        map_code(ffi.lib.rns_node_reload_config(self._handle))

    def packet_send(self, dest_hash: bytes | bytearray | memoryview, plaintext: bytes | bytearray | memoryview) -> None:
        import ctypes

        dest = bytes(dest_hash)
        payload = bytes(plaintext)
        if len(dest) != ffi.HASH_LEN or not payload:
            raise Error(Error.INVALID_ARG)
        dest_ptr = (ctypes.c_uint8 * ffi.HASH_LEN).from_buffer_copy(dest)
        data_ptr = (ctypes.c_uint8 * len(payload)).from_buffer_copy(payload)
        map_code(ffi.lib.rns_packet_send(self._handle, dest_ptr, data_ptr, len(payload)))

    def set_event_callback(self, callback) -> None:
        from .event import set_event_callback

        set_event_callback(self, callback)

    def event_poll(self, timeout_ms: int = 0, app_data_cap: int = 65536):
        from .event import Event

        buf = bytearray(app_data_cap) if app_data_cap > 0 else None
        return Event.poll(self, timeout_ms, buf)

    @property
    def handle(self) -> int:
        return self._handle

    def close(self) -> None:
        if self._handle:
            ffi.lib.rns_node_destroy(self._handle)
            self._handle = 0

    def __enter__(self) -> Node:
        return self

    def __exit__(self, *args: object) -> None:
        self.close()

    def __del__(self) -> None:
        try:
            self.close()
        except Exception:
            pass
