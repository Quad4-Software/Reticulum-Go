# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

from __future__ import annotations

import ctypes

from . import ffi


class Error(Exception):
    """librns error code."""

    OK = 0
    INVALID_ARG = 1
    INVALID_HANDLE = 2
    NOT_FOUND = 3
    STATE = 4
    IO = 5
    INTERNAL = 6
    TIMEOUT = 7
    TRUNCATED = 8

    def __init__(self, code: int, message: str | None = None) -> None:
        self.code = code
        super().__init__(message or name(code))


def name(code: int) -> str:
    return {
        Error.OK: "ok",
        Error.INVALID_ARG: "invalid argument",
        Error.INVALID_HANDLE: "invalid handle",
        Error.NOT_FOUND: "not found",
        Error.STATE: "invalid state",
        Error.IO: "io error",
        Error.INTERNAL: "internal error",
        Error.TIMEOUT: "timeout",
        Error.TRUNCATED: "truncated",
    }.get(code, "internal error")


def map_code(code: int) -> None:
    if code != Error.OK:
        raise Error(code)


def version() -> str:
    raw = ffi.lib.rns_version()
    return raw.decode("utf-8") if raw else ""


def last_error() -> str:
    buf = ctypes.create_string_buffer(512)
    written = ctypes.c_size_t(0)
    code = ffi.lib.rns_last_error(buf, len(buf), ctypes.byref(written))
    if code != Error.OK or written.value == 0:
        return ""
    return buf.raw[: written.value].decode("utf-8", errors="replace")
