# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

from __future__ import annotations

import ctypes

from . import ffi
from .errors import Error, map_code


class Identity:
    def __init__(self, handle: int) -> None:
        self._handle = int(handle)

    @classmethod
    def generate(cls) -> Identity:
        h = ffi.lib.rns_identity_generate()
        if h == 0:
            raise Error(Error.INTERNAL)
        return cls(h)

    @classmethod
    def load(cls, path: str) -> Identity:
        if not path:
            raise Error(Error.INVALID_ARG)
        h = ffi.lib.rns_identity_load(path.encode("utf-8"))
        if h == 0:
            raise Error(Error.IO)
        return cls(h)

    @classmethod
    def from_public_key(cls, pub: bytes | bytearray | memoryview) -> Identity:
        data = bytes(pub)
        if not data:
            raise Error(Error.INVALID_ARG)
        buf = (ctypes.c_uint8 * len(data)).from_buffer_copy(data)
        h = ffi.lib.rns_identity_from_public_key(buf, len(data))
        if h == 0:
            raise Error(Error.INVALID_ARG)
        return cls(h)

    def save(self, path: str) -> None:
        if not path:
            raise Error(Error.INVALID_ARG)
        map_code(ffi.lib.rns_identity_save(self._handle, path.encode("utf-8")))

    def hash_hex(self) -> str:
        buf = ctypes.create_string_buffer(64)
        written = ctypes.c_size_t(0)
        map_code(ffi.lib.rns_identity_hash(self._handle, buf, 64, ctypes.byref(written)))
        return buf.raw[: written.value].decode("ascii")

    def hash_bytes(self) -> bytes:
        out = (ctypes.c_uint8 * ffi.HASH_LEN)()
        written = ctypes.c_size_t(0)
        map_code(
            ffi.lib.rns_identity_hash_bytes(self._handle, out, ffi.HASH_LEN, ctypes.byref(written))
        )
        if written.value != ffi.HASH_LEN:
            raise Error(Error.TRUNCATED)
        return bytes(out)

    def public_key(self) -> bytes:
        out = (ctypes.c_uint8 * 64)()
        written = ctypes.c_size_t(0)
        map_code(ffi.lib.rns_identity_public_key(self._handle, out, 64, ctypes.byref(written)))
        return bytes(out[: written.value])

    def sign(self, data: bytes | bytearray | memoryview) -> bytes:
        payload = bytes(data)
        data_ptr = (ctypes.c_uint8 * len(payload)).from_buffer_copy(payload) if payload else None
        sig = (ctypes.c_uint8 * 64)()
        written = ctypes.c_size_t(0)
        map_code(
            ffi.lib.rns_identity_sign(
                self._handle,
                data_ptr,
                len(payload),
                sig,
                64,
                ctypes.byref(written),
            )
        )
        return bytes(sig[: written.value])

    def verify(self, data: bytes | bytearray | memoryview, signature: bytes | bytearray | memoryview) -> None:
        payload = bytes(data)
        sig = bytes(signature)
        data_ptr = (ctypes.c_uint8 * len(payload)).from_buffer_copy(payload) if payload else None
        sig_ptr = (ctypes.c_uint8 * len(sig)).from_buffer_copy(sig) if sig else None
        map_code(
            ffi.lib.rns_identity_verify(
                self._handle,
                data_ptr,
                len(payload),
                sig_ptr,
                len(sig),
            )
        )

    @property
    def handle(self) -> int:
        return self._handle

    def close(self) -> None:
        if self._handle:
            ffi.lib.rns_identity_destroy(self._handle)
            self._handle = 0

    def __enter__(self) -> Identity:
        return self

    def __exit__(self, *args: object) -> None:
        self.close()

    def __del__(self) -> None:
        try:
            self.close()
        except Exception:
            pass
