# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

from . import ffi
from .errors import Error, map_code
from .event import poll_hub


class Hub:
    def __init__(self, handle: int) -> None:
        self._handle = int(handle)

    @classmethod
    def create(cls, node_handle: int, identity_handle: int, name: str, version: str) -> Hub:
        h = ffi.lib.rrc_hub_create(
            node_handle,
            identity_handle,
            name.encode("utf-8"),
            version.encode("utf-8"),
        )
        if h == 0:
            raise Error(Error.INTERNAL)
        return cls(h)

    def start(self) -> None:
        map_code(ffi.lib.rrc_hub_start(self._handle))

    def announce(self) -> None:
        map_code(ffi.lib.rrc_hub_announce(self._handle))

    def hash_bytes(self) -> bytes:
        buf = bytearray(ffi.HASH_LEN)
        written = ffi.ctypes.c_size_t(0)
        arr = (ffi.ctypes.c_uint8 * ffi.HASH_LEN).from_buffer(buf)
        map_code(
            ffi.lib.rrc_hub_hash(
                self._handle,
                arr,
                ffi.HASH_LEN,
                ffi.ctypes.byref(written),
            )
        )
        return bytes(buf[: written.value])

    def peer_count(self) -> int:
        count = ffi.ctypes.c_size_t(0)
        map_code(ffi.lib.rrc_hub_peer_count(self._handle, ffi.ctypes.byref(count)))
        return int(count.value)

    def event_poll(self, timeout_ms: int = 0):
        return poll_hub(self._handle, timeout_ms)

    @property
    def handle(self) -> int:
        return self._handle

    def close(self) -> None:
        if self._handle:
            ffi.lib.rrc_hub_destroy(self._handle)
            self._handle = 0

    def __enter__(self) -> Hub:
        return self

    def __exit__(self, *args: object) -> None:
        self.close()

    def __del__(self) -> None:
        try:
            self.close()
        except Exception:
            pass

    def __repr__(self) -> str:
        return f"Hub(handle={self._handle})"
