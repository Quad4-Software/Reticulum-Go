# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

from . import ffi
from .errors import Error, map_code


class Identity:
    def __init__(self, handle: int) -> None:
        self._handle = int(handle)

    @classmethod
    def generate(cls) -> Identity:
        h = ffi.lib.rrc_identity_generate()
        if h == 0:
            raise Error(Error.INTERNAL)
        return cls(h)

    @classmethod
    def load(cls, path: str) -> Identity:
        h = ffi.lib.rrc_identity_load(path.encode("utf-8"))
        if h == 0:
            raise Error(Error.IO)
        return cls(h)

    def save(self, path: str) -> None:
        map_code(ffi.lib.rrc_identity_save(self._handle, path.encode("utf-8")))

    def hash_bytes(self) -> bytes:
        buf = bytearray(ffi.HASH_LEN)
        written = ffi.ctypes.c_size_t(0)
        arr = (ffi.ctypes.c_uint8 * ffi.HASH_LEN).from_buffer(buf)
        map_code(
            ffi.lib.rrc_identity_hash(
                self._handle,
                arr,
                ffi.HASH_LEN,
                ffi.ctypes.byref(written),
            )
        )
        return bytes(buf[: written.value])

    def seed_destination(self, dest_hash: bytes) -> None:
        if len(dest_hash) != ffi.HASH_LEN:
            raise Error(Error.INVALID_ARG)
        arr = (ffi.ctypes.c_uint8 * ffi.HASH_LEN).from_buffer_copy(dest_hash)
        map_code(ffi.lib.rrc_identity_seed_destination(self._handle, arr, ffi.HASH_LEN))

    @property
    def handle(self) -> int:
        return self._handle

    def close(self) -> None:
        if self._handle:
            ffi.lib.rrc_identity_destroy(self._handle)
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

    def __repr__(self) -> str:
        return f"Identity(handle={self._handle})"
