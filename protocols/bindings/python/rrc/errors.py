# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

from . import ffi


class Error(Exception):
    OK = ffi.RRC_OK
    INVALID_ARG = ffi.RRC_ERR_INVALID_ARG
    INVALID_HANDLE = ffi.RRC_ERR_INVALID_HANDLE
    NOT_FOUND = ffi.RRC_ERR_NOT_FOUND
    STATE = ffi.RRC_ERR_STATE
    IO = ffi.RRC_ERR_IO
    INTERNAL = ffi.RRC_ERR_INTERNAL
    TIMEOUT = ffi.RRC_ERR_TIMEOUT
    TRUNCATED = ffi.RRC_ERR_TRUNCATED

    def __init__(self, code: int) -> None:
        self.code = int(code)
        msg = last_error()
        super().__init__(msg if msg else f"rrc error {self.code}")


def version() -> str:
    raw = ffi.lib.rrc_version()
    if not raw:
        return ""
    return raw.decode("utf-8")


def last_error() -> str:
    buf = ffi.ctypes.create_string_buffer(512)
    written = ffi.ctypes.c_size_t(0)
    code = ffi.lib.rrc_last_error(buf, len(buf), ffi.ctypes.byref(written))
    msg = buf.raw[: written.value].decode("utf-8", errors="replace").rstrip("\x00")
    if msg:
        return msg
    if code == Error.TRUNCATED:
        return "truncated"
    return ""


def map_code(code: int) -> None:
    if code == Error.OK:
        return
    raise Error(code)
