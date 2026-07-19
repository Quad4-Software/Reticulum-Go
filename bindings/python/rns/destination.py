# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

from __future__ import annotations

import ctypes

from . import ffi
from .errors import Error, map_code
from .identity import Identity
from .node import Node


class Destination:
    def __init__(self, handle: int) -> None:
        self._handle = int(handle)

    @classmethod
    def create(
        cls,
        node: Node,
        identity: Identity | None,
        app_name: str,
        aspects: list[str] | tuple[str, ...],
        accepts_links: bool,
    ) -> Destination:
        if not app_name:
            raise Error(Error.INVALID_ARG)
        aspect_list = list(aspects)
        aspect_ptrs: list[ctypes.c_char_p] = []
        for aspect in aspect_list:
            aspect_ptrs.append(aspect.encode("utf-8"))
        aspect_array = (ctypes.c_char_p * len(aspect_ptrs))(*aspect_ptrs) if aspect_ptrs else None
        h = ffi.lib.rns_destination_create(
            node.handle,
            identity.handle if identity is not None else 0,
            app_name.encode("utf-8"),
            aspect_array,
            len(aspect_ptrs),
            1 if accepts_links else 0,
        )
        if h == 0:
            raise Error(Error.INTERNAL)
        return cls(h)

    def announce(self, app_data: bytes | bytearray | memoryview = b"") -> None:
        payload = bytes(app_data)
        data_ptr = (ctypes.c_uint8 * len(payload)).from_buffer_copy(payload) if payload else None
        map_code(
            ffi.lib.rns_destination_announce(
                self._handle,
                data_ptr,
                len(payload),
            )
        )

    def hash(self) -> bytes:
        out = (ctypes.c_uint8 * ffi.HASH_LEN)()
        written = ctypes.c_size_t(0)
        map_code(
            ffi.lib.rns_destination_hash(self._handle, out, ffi.HASH_LEN, ctypes.byref(written))
        )
        if written.value != ffi.HASH_LEN:
            raise Error(Error.TRUNCATED)
        return bytes(out)

    def register_request_handler(self, path: str) -> None:
        if not path:
            raise Error(Error.INVALID_ARG)
        map_code(ffi.lib.rns_destination_register_request_handler(self._handle, path.encode("utf-8")))

    @property
    def handle(self) -> int:
        return self._handle

    def close(self) -> None:
        if self._handle:
            ffi.lib.rns_destination_destroy(self._handle)
            self._handle = 0

    def __enter__(self) -> Destination:
        return self

    def __exit__(self, *args: object) -> None:
        self.close()

    def __del__(self) -> None:
        try:
            self.close()
        except Exception:
            pass
