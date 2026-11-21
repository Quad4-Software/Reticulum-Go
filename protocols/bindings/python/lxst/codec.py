# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

import ctypes
from contextlib import contextmanager
from typing import Iterator

from .ffi import (
    HASH_LEN,
    LXST_ERR_TRUNCATED,
    LXST_OK,
    lib,
)

MAX_BUFFER = 16 * 1024 * 1024


class LxstError(OSError):
    def __init__(self, code: int) -> None:
        self.code = int(code)
        msg = last_error()
        super().__init__(msg if msg else f"lxst error {self.code}")


def version() -> str:
    raw = lib.lxst_version()
    if not raw:
        return ""
    return raw.decode("utf-8")


def last_error() -> str:
    buf = ctypes.create_string_buffer(512)
    written = ctypes.c_size_t(0)
    code = lib.lxst_last_error(buf, len(buf), ctypes.byref(written))
    msg = buf.raw[: written.value].decode("utf-8", errors="replace").rstrip("\x00")
    if msg:
        return msg
    if code == LXST_ERR_TRUNCATED:
        return "truncated"
    return ""


def _check(code: int) -> None:
    if code != LXST_OK:
        raise LxstError(code)


def _grow_buffer(fn, initial: int = 512) -> bytes:
    capacity = initial
    while capacity <= MAX_BUFFER:
        out = bytearray(capacity)
        written = ctypes.c_size_t(0)
        out_arr = (ctypes.c_uint8 * capacity).from_buffer(out)
        code = fn(out_arr, capacity, ctypes.byref(written))
        if code == LXST_ERR_TRUNCATED:
            capacity *= 2
            continue
        _check(code)
        return bytes(out[: written.value])
    raise LxstError(LXST_ERR_TRUNCATED)


def pack_signalling(signals: list[int]) -> bytes:
    arr = (ctypes.c_int * len(signals))(*signals)
    return _grow_buffer(
        lambda out, cap, written: lib.lxst_pack_signalling(
            arr, len(signals), out, cap, written
        )
    )


def pack_frame(codec: int, payload: bytes) -> bytes:
    in_arr = (ctypes.c_uint8 * len(payload)).from_buffer_copy(payload)
    return _grow_buffer(
        lambda out, cap, written: lib.lxst_pack_frame(
            ctypes.c_uint8(codec), in_arr, len(payload), out, cap, written
        )
    )


def unpack(data: bytes) -> int:
    in_arr = (ctypes.c_uint8 * len(data)).from_buffer_copy(data)
    handle = lib.lxst_unpack(in_arr, len(data))
    if handle == 0:
        raise LxstError(LXST_ERR_INVALID_ARG)
    return int(handle)


def packet_destroy(handle: int) -> None:
    _check(lib.lxst_packet_destroy(ctypes.c_uint64(handle)))


def packet_signal_count(handle: int) -> int:
    count = ctypes.c_size_t(0)
    _check(lib.lxst_packet_signal_count(ctypes.c_uint64(handle), ctypes.byref(count)))
    return int(count.value)


def packet_signal_at(handle: int, index: int) -> int:
    signal = ctypes.c_int(0)
    _check(
        lib.lxst_packet_signal_at(
            ctypes.c_uint64(handle), ctypes.c_size_t(index), ctypes.byref(signal)
        )
    )
    return int(signal.value)


def packet_frame_count(handle: int) -> int:
    count = ctypes.c_size_t(0)
    _check(lib.lxst_packet_frame_count(ctypes.c_uint64(handle), ctypes.byref(count)))
    return int(count.value)


def packet_frame_at(handle: int, index: int) -> bytes:
    capacity = 512
    while capacity <= MAX_BUFFER:
        out = bytearray(capacity)
        written = ctypes.c_size_t(0)
        out_arr = (ctypes.c_uint8 * capacity).from_buffer(out)
        code = lib.lxst_packet_frame_at(
            ctypes.c_uint64(handle),
            ctypes.c_size_t(index),
            out_arr,
            capacity,
            ctypes.byref(written),
        )
        if code == LXST_ERR_TRUNCATED:
            capacity *= 2
            continue
        _check(code)
        return bytes(out[: written.value])
    raise LxstError(LXST_ERR_TRUNCATED)


def split_frame(frame: bytes) -> tuple[int, bytes]:
    in_arr = (ctypes.c_uint8 * len(frame)).from_buffer_copy(frame)
    codec = ctypes.c_uint8(0)
    capacity = 512
    while capacity <= MAX_BUFFER:
        payload = bytearray(capacity)
        written = ctypes.c_size_t(0)
        payload_arr = (ctypes.c_uint8 * capacity).from_buffer(payload)
        code = lib.lxst_split_frame(
            in_arr,
            len(frame),
            ctypes.byref(codec),
            payload_arr,
            capacity,
            ctypes.byref(written),
        )
        if code == LXST_ERR_TRUNCATED:
            capacity *= 2
            continue
        _check(code)
        return int(codec.value), bytes(payload[: written.value])
    raise LxstError(LXST_ERR_TRUNCATED)


def telephony_hash(identity: bytes) -> bytes:
    if len(identity) != HASH_LEN:
        raise ValueError("identity hash must be 16 bytes")
    in_arr = (ctypes.c_uint8 * HASH_LEN).from_buffer_copy(identity)
    return _grow_buffer(
        lambda out, cap, written: lib.lxst_telephony_hash(
            in_arr, HASH_LEN, out, cap, written
        ),
        initial=HASH_LEN,
    )


def signal_preferred_mode(mode: int) -> int:
    return int(lib.lxst_signal_preferred_mode(ctypes.c_int(mode)))


def signal_preferred_profile(profile: int) -> int:
    return int(lib.lxst_signal_preferred_profile(ctypes.c_int(profile)))


def profile_from_name(name: str) -> int:
    return int(lib.lxst_profile_from_name(name.encode("utf-8")))


@contextmanager
def packet(data: bytes) -> Iterator[int]:
    handle = unpack(data)
    try:
        yield handle
    finally:
        packet_destroy(handle)
