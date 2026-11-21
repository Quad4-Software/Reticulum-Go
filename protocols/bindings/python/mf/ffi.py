# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

import ctypes
import threading
from pathlib import Path

from common.platform import lib_candidates

HASH_LEN = 16

MF_OK = 0
MF_ERR_INVALID_ARG = 1
MF_ERR_INTERNAL = 6
MF_ERR_TRUNCATED = 8

_lib: ctypes.CDLL | None = None
_lock = threading.Lock()


def load_library(path: str | None = None) -> ctypes.CDLL:
    if path:
        return ctypes.CDLL(path)
    for candidate in lib_candidates("mf", "MF_LIB_PATH", Path(__file__).resolve()):
        if candidate.is_file():
            return ctypes.CDLL(str(candidate))
    return ctypes.CDLL("libmf.so")


def _configure(lib: ctypes.CDLL) -> None:
    lib.mf_version.restype = ctypes.c_char_p
    lib.mf_last_error.argtypes = [
        ctypes.c_char_p,
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.mf_last_error.restype = ctypes.c_int
    lib.mf_pack.argtypes = [
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.c_char_p,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.mf_pack.restype = ctypes.c_int
    lib.mf_unpack.argtypes = [
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
        ctypes.c_char_p,
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.mf_unpack.restype = ctypes.c_int


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
