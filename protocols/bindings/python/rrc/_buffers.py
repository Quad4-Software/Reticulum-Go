# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

import ctypes

from .errors import Error, map_code

MAX_BUFFER = 16 * 1024 * 1024


def read_c_string(call) -> str:
    capacity = 1024
    while capacity <= MAX_BUFFER:
        buf = ctypes.create_string_buffer(capacity)
        written = ctypes.c_size_t(0)
        code = call(buf, capacity, ctypes.byref(written))
        if code == Error.TRUNCATED:
            capacity *= 2
            continue
        map_code(code)
        return buf.raw[: written.value].decode("utf-8").rstrip("\x00")
    raise Error(Error.TRUNCATED)


def write_bytes(call, initial: int = 65536) -> bytes:
    capacity = initial
    while capacity <= MAX_BUFFER:
        buf = bytearray(capacity)
        written = ctypes.c_size_t(0)
        arr = (ctypes.c_uint8 * capacity).from_buffer(buf)
        code = call(arr, capacity, ctypes.byref(written))
        if code == Error.TRUNCATED:
            capacity *= 2
            continue
        map_code(code)
        return bytes(buf[: written.value])
    raise Error(Error.TRUNCATED)
