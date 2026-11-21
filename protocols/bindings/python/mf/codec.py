# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

import ctypes

from .ffi import HASH_LEN, MF_ERR_TRUNCATED, MF_OK, lib

MAX_BUFFER = 16 * 1024 * 1024


class MfError(OSError):
    def __init__(self, code: int) -> None:
        self.code = int(code)
        msg = last_error()
        super().__init__(msg if msg else f"mf error {self.code}")


def version() -> str:
    raw = lib.mf_version()
    if not raw:
        return ""
    return raw.decode("utf-8")


def last_error() -> str:
    buf = ctypes.create_string_buffer(512)
    written = ctypes.c_size_t(0)
    code = lib.mf_last_error(buf, len(buf), ctypes.byref(written))
    msg = buf.raw[: written.value].decode("utf-8", errors="replace").rstrip("\x00")
    if msg:
        return msg
    if code == MF_ERR_TRUNCATED:
        return "truncated"
    return ""


def _check(code: int) -> None:
    if code != MF_OK:
        raise MfError(code)


def pack(sender: bytes, text: str) -> bytes:
    if len(sender) != HASH_LEN:
        raise ValueError("sender hash must be 16 bytes")
    l = lib
    arr = (ctypes.c_uint8 * HASH_LEN).from_buffer_copy(sender)
    capacity = 512
    while capacity <= MAX_BUFFER:
        out = bytearray(capacity)
        written = ctypes.c_size_t(0)
        out_arr = (ctypes.c_uint8 * capacity).from_buffer(out)
        code = l.mf_pack(arr, HASH_LEN, text.encode("utf-8"), out_arr, capacity, ctypes.byref(written))
        if code == MF_ERR_TRUNCATED:
            capacity *= 2
            continue
        _check(code)
        return bytes(out[: written.value])
    raise MfError(MF_ERR_TRUNCATED)


def unpack(data: bytes) -> tuple[bytes, str]:
    l = lib
    in_arr = (ctypes.c_uint8 * len(data)).from_buffer_copy(data)
    sender = bytearray(HASH_LEN)
    sender_arr = (ctypes.c_uint8 * HASH_LEN).from_buffer(sender)
    sender_written = ctypes.c_size_t(0)
    capacity = 512
    while capacity <= MAX_BUFFER:
        text_buf = ctypes.create_string_buffer(capacity)
        text_written = ctypes.c_size_t(0)
        code = l.mf_unpack(
            in_arr,
            len(data),
            sender_arr,
            HASH_LEN,
            ctypes.byref(sender_written),
            text_buf,
            capacity,
            ctypes.byref(text_written),
        )
        if code == MF_ERR_TRUNCATED:
            capacity *= 2
            continue
        _check(code)
        text = text_buf.raw[: text_written.value].decode("utf-8").rstrip("\x00")
        return bytes(sender[: sender_written.value]), text
    raise MfError(MF_ERR_TRUNCATED)
