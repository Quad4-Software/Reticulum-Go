# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

import ctypes
import threading
from pathlib import Path

from common.platform import lib_candidates

HASH_LEN = 16

LXST_OK = 0
LXST_ERR_INVALID_ARG = 1
LXST_ERR_INVALID_HANDLE = 2
LXST_ERR_INTERNAL = 6
LXST_ERR_TRUNCATED = 8

STATUS_AVAILABLE = 0x03
CODEC_OPUS = 0x01
PROFILE_QUALITY_MEDIUM = 0x40
MODE_FULL_DUPLEX = 0x01
PREFERRED_MODE = 0xF0
PREFERRED_PROFILE = 0xFF

_lib: ctypes.CDLL | None = None
_lock = threading.Lock()


def load_library(path: str | None = None) -> ctypes.CDLL:
    if path:
        return ctypes.CDLL(path)
    for candidate in lib_candidates("lxst", "LXST_LIB_PATH", Path(__file__).resolve()):
        if candidate.is_file():
            return ctypes.CDLL(str(candidate))
    return ctypes.CDLL("liblxst.so")


def _configure(lib: ctypes.CDLL) -> None:
    lib.lxst_version.restype = ctypes.c_char_p
    lib.lxst_last_error.argtypes = [
        ctypes.c_char_p,
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxst_last_error.restype = ctypes.c_int
    lib.lxst_pack_signalling.argtypes = [
        ctypes.POINTER(ctypes.c_int),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxst_pack_signalling.restype = ctypes.c_int
    lib.lxst_pack_frame.argtypes = [
        ctypes.c_uint8,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxst_pack_frame.restype = ctypes.c_int
    lib.lxst_unpack.argtypes = [ctypes.POINTER(ctypes.c_uint8), ctypes.c_size_t]
    lib.lxst_unpack.restype = ctypes.c_uint64
    lib.lxst_packet_destroy.argtypes = [ctypes.c_uint64]
    lib.lxst_packet_destroy.restype = ctypes.c_int
    lib.lxst_packet_signal_count.argtypes = [
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxst_packet_signal_count.restype = ctypes.c_int
    lib.lxst_packet_signal_at.argtypes = [
        ctypes.c_uint64,
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_int),
    ]
    lib.lxst_packet_signal_at.restype = ctypes.c_int
    lib.lxst_packet_frame_count.argtypes = [
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxst_packet_frame_count.restype = ctypes.c_int
    lib.lxst_packet_frame_at.argtypes = [
        ctypes.c_uint64,
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxst_packet_frame_at.restype = ctypes.c_int
    lib.lxst_split_frame.argtypes = [
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxst_split_frame.restype = ctypes.c_int
    lib.lxst_telephony_hash.argtypes = [
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxst_telephony_hash.restype = ctypes.c_int
    lib.lxst_signal_preferred_mode.argtypes = [ctypes.c_int]
    lib.lxst_signal_preferred_mode.restype = ctypes.c_int
    lib.lxst_signal_preferred_profile.argtypes = [ctypes.c_int]
    lib.lxst_signal_preferred_profile.restype = ctypes.c_int
    lib.lxst_profile_from_name.argtypes = [ctypes.c_char_p]
    lib.lxst_profile_from_name.restype = ctypes.c_int


def _load_lib() -> ctypes.CDLL:
    global _lib
    if _lib is not None:
        return _lib
    with _lock:
        if _lib is None:
            loaded = load_library()
            _configure(loaded)
            _lib = loaded
    return _lib


class _LibProxy:
    def __getattr__(self, name: str):
        return getattr(_load_lib(), name)


lib = _LibProxy()
