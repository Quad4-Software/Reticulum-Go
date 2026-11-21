# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

from . import ffi
from ._buffers import read_c_string, write_bytes
from .errors import Error, map_code


class Envelope:
    def __init__(self, handle: int) -> None:
        self._handle = int(handle)

    @classmethod
    def create(cls, msg_type: int, sender: bytes) -> Envelope:
        if len(sender) != ffi.HASH_LEN:
            raise Error(Error.INVALID_ARG)
        arr = (ffi.ctypes.c_uint8 * ffi.HASH_LEN).from_buffer_copy(sender)
        h = ffi.lib.rrc_envelope_create(msg_type, arr, ffi.HASH_LEN)
        if h == 0:
            raise Error(Error.INTERNAL)
        return cls(h)

    @classmethod
    def unmarshal(cls, data: bytes) -> Envelope:
        arr = (ffi.ctypes.c_uint8 * len(data)).from_buffer_copy(data)
        h = ffi.lib.rrc_envelope_unmarshal(arr, len(data))
        if h == 0:
            raise Error(Error.INVALID_ARG)
        return cls(h)

    def set_room(self, room: str) -> None:
        map_code(ffi.lib.rrc_envelope_set_room(self._handle, room.encode("utf-8")))

    def set_nick(self, nick: str) -> None:
        map_code(ffi.lib.rrc_envelope_set_nick(self._handle, nick.encode("utf-8")))

    def set_body_text(self, text: str) -> None:
        map_code(ffi.lib.rrc_envelope_set_body_text(self._handle, text.encode("utf-8")))

    def set_destination(self, dest: bytes) -> None:
        if len(dest) != ffi.HASH_LEN:
            raise Error(Error.INVALID_ARG)
        arr = (ffi.ctypes.c_uint8 * ffi.HASH_LEN).from_buffer_copy(dest)
        map_code(ffi.lib.rrc_envelope_set_destination(self._handle, arr, ffi.HASH_LEN))

    def msg_type(self) -> int:
        out = ffi.ctypes.c_uint64(0)
        map_code(ffi.lib.rrc_envelope_get_type(self._handle, ffi.ctypes.byref(out)))
        return int(out.value)

    def sender(self) -> bytes:
        buf = bytearray(ffi.HASH_LEN)
        written = ffi.ctypes.c_size_t(0)
        arr = (ffi.ctypes.c_uint8 * ffi.HASH_LEN).from_buffer(buf)
        map_code(
            ffi.lib.rrc_envelope_get_sender(
                self._handle,
                arr,
                ffi.HASH_LEN,
                ffi.ctypes.byref(written),
            )
        )
        return bytes(buf[: written.value])

    def room(self) -> str:
        return read_c_string(
            lambda buf, cap, written: ffi.lib.rrc_envelope_get_room(
                self._handle, buf, cap, written
            )
        )

    def nick(self) -> str:
        return read_c_string(
            lambda buf, cap, written: ffi.lib.rrc_envelope_get_nick(
                self._handle, buf, cap, written
            )
        )

    def body_text(self) -> str:
        return read_c_string(
            lambda buf, cap, written: ffi.lib.rrc_envelope_get_body_text(
                self._handle, buf, cap, written
            )
        )

    def marshal(self) -> bytes:
        return write_bytes(
            lambda arr, cap, written: ffi.lib.rrc_envelope_marshal(
                self._handle, arr, cap, written
            )
        )

    @property
    def handle(self) -> int:
        return self._handle

    def close(self) -> None:
        if self._handle:
            ffi.lib.rrc_envelope_destroy(self._handle)
            self._handle = 0

    def __enter__(self) -> Envelope:
        return self

    def __exit__(self, *args: object) -> None:
        self.close()

    def __del__(self) -> None:
        try:
            self.close()
        except Exception:
            pass

    def __repr__(self) -> str:
        return f"Envelope(handle={self._handle})"
