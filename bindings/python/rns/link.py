# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

from __future__ import annotations

import ctypes

from . import ffi
from .errors import Error, map_code
from .node import Node


class Link:
    def __init__(self, handle: int) -> None:
        self._handle = int(handle)

    @classmethod
    def open(cls, node: Node, dest_hash: bytes | bytearray | memoryview) -> Link:
        data = bytes(dest_hash)
        if len(data) != ffi.HASH_LEN:
            raise Error(Error.INVALID_ARG)
        buf = (ctypes.c_uint8 * ffi.HASH_LEN).from_buffer_copy(data)
        h = ffi.lib.rns_link_open(node.handle, buf)
        if h == 0:
            raise Error(Error.INTERNAL)
        return cls(h)

    def send(self, data: bytes | bytearray | memoryview) -> None:
        payload = bytes(data)
        if not payload:
            raise Error(Error.INVALID_ARG)
        buf = (ctypes.c_uint8 * len(payload)).from_buffer_copy(payload)
        map_code(ffi.lib.rns_link_send(self._handle, buf, len(payload)))

    def send_resource(self, data: bytes | bytearray | memoryview, name: str = "") -> None:
        payload = bytes(data)
        name_ptr = name.encode("utf-8") if name else None
        data_ptr = (ctypes.c_uint8 * len(payload)).from_buffer_copy(payload) if payload else None
        map_code(
            ffi.lib.rns_link_send_resource(
                self._handle,
                data_ptr,
                len(payload),
                name_ptr,
            )
        )

    def close(self) -> None:
        if self._handle:
            ffi.lib.rns_link_close(self._handle)
            self._handle = 0

    def id(self) -> bytes:
        out = (ctypes.c_uint8 * ffi.HASH_LEN)()
        written = ctypes.c_size_t(0)
        map_code(ffi.lib.rns_link_id(self._handle, out, ffi.HASH_LEN, ctypes.byref(written)))
        if written.value != ffi.HASH_LEN:
            raise Error(Error.TRUNCATED)
        return bytes(out)

    def request(
        self,
        node: Node,
        path: str,
        data: bytes | bytearray | memoryview = b"",
        timeout_ms: int = 0,
    ) -> bytes:
        if not path:
            raise Error(Error.INVALID_ARG)
        payload = bytes(data)
        data_ptr = (ctypes.c_uint8 * len(payload)).from_buffer_copy(payload) if payload else None
        request_id = (ctypes.c_uint8 * ffi.HASH_LEN)()
        written = ctypes.c_size_t(0)
        map_code(
            ffi.lib.rns_link_request(
                node.handle,
                self._handle,
                path.encode("utf-8"),
                data_ptr,
                len(payload),
                int(timeout_ms),
                request_id,
                ffi.HASH_LEN,
                ctypes.byref(written),
            )
        )
        if written.value != ffi.HASH_LEN:
            raise Error(Error.TRUNCATED)
        return bytes(request_id)

    @property
    def handle(self) -> int:
        return self._handle

    def __enter__(self) -> Link:
        return self

    def __exit__(self, *args: object) -> None:
        self.close()

    def __del__(self) -> None:
        try:
            self.close()
        except Exception:
            pass


def request_respond(node: Node, request_id: bytes | bytearray | memoryview, data: bytes | bytearray | memoryview) -> None:
    req = bytes(request_id)
    if not req:
        raise Error(Error.INVALID_ARG)
    payload = bytes(data)
    req_ptr = (ctypes.c_uint8 * len(req)).from_buffer_copy(req)
    data_ptr = (ctypes.c_uint8 * len(payload)).from_buffer_copy(payload) if payload else None
    map_code(
        ffi.lib.rns_request_respond(
            node.handle,
            req_ptr,
            len(req),
            data_ptr,
            len(payload),
        )
    )


def request_respond_file(
    node: Node,
    request_id: bytes | bytearray | memoryview,
    filename: str,
    data: bytes | bytearray | memoryview,
) -> None:
    req = bytes(request_id)
    if not req or not filename:
        raise Error(Error.INVALID_ARG)
    payload = bytes(data)
    req_ptr = (ctypes.c_uint8 * len(req)).from_buffer_copy(req)
    data_ptr = (ctypes.c_uint8 * len(payload)).from_buffer_copy(payload) if payload else None
    map_code(
        ffi.lib.rns_request_respond_file(
            node.handle,
            req_ptr,
            len(req),
            filename.encode("utf-8"),
            data_ptr,
            len(payload),
        )
    )
