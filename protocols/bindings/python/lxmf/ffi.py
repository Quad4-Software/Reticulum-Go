# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

import ctypes
import threading
from pathlib import Path

from common.platform import lib_candidates

HASH_LEN = 16

LXMF_OK = 0
LXMF_ERR_INVALID_ARG = 1
LXMF_ERR_INVALID_HANDLE = 2
LXMF_ERR_INTERNAL = 6
LXMF_ERR_TRUNCATED = 8

_lib: ctypes.CDLL | None = None
_lock = threading.Lock()


def load_library(path: str | None = None) -> ctypes.CDLL:
    if path:
        return ctypes.CDLL(path)
    for candidate in lib_candidates("lxmf", "LXMF_LIB_PATH", Path(__file__).resolve()):
        if candidate.is_file():
            return ctypes.CDLL(str(candidate))
    return ctypes.CDLL("liblxmf.so")


def _configure(lib: ctypes.CDLL) -> None:
    lib.lxmf_version.restype = ctypes.c_char_p
    lib.lxmf_last_error.argtypes = [
        ctypes.c_char_p,
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxmf_last_error.restype = ctypes.c_int
    lib.lxmf_identity_generate.restype = ctypes.c_uint64
    lib.lxmf_identity_destroy.argtypes = [ctypes.c_uint64]
    lib.lxmf_identity_destroy.restype = ctypes.c_int
    lib.lxmf_identity_hash.argtypes = [
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxmf_identity_hash.restype = ctypes.c_int
    lib.lxmf_identity_public_key.argtypes = [
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxmf_identity_public_key.restype = ctypes.c_int
    lib.lxmf_identity_delivery_hash.argtypes = [
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxmf_identity_delivery_hash.restype = ctypes.c_int
    lib.lxmf_identity_register_recall.argtypes = [ctypes.c_uint64]
    lib.lxmf_identity_register_recall.restype = ctypes.c_int
    lib.lxmf_identity_register_recall_source.argtypes = [
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
    ]
    lib.lxmf_identity_register_recall_source.restype = ctypes.c_int
    lib.lxmf_message_create.argtypes = [
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.c_char_p,
        ctypes.c_char_p,
    ]
    lib.lxmf_message_create.restype = ctypes.c_uint64
    lib.lxmf_message_set_fields_json.argtypes = [ctypes.c_uint64, ctypes.c_char_p]
    lib.lxmf_message_set_fields_json.restype = ctypes.c_int
    lib.lxmf_message_pack.argtypes = [
        ctypes.c_uint64,
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxmf_message_pack.restype = ctypes.c_int
    lib.lxmf_message_encrypted_payload.argtypes = [
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxmf_message_encrypted_payload.restype = ctypes.c_int
    lib.lxmf_message_unpack.argtypes = [ctypes.POINTER(ctypes.c_uint8), ctypes.c_size_t]
    lib.lxmf_message_unpack.restype = ctypes.c_uint64
    lib.lxmf_message_unpack_verified.argtypes = [
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.c_uint64,
    ]
    lib.lxmf_message_unpack_verified.restype = ctypes.c_uint64
    lib.lxmf_message_get_dest.argtypes = [
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxmf_message_get_dest.restype = ctypes.c_int
    lib.lxmf_message_get_source.argtypes = [
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxmf_message_get_source.restype = ctypes.c_int
    lib.lxmf_message_get_title.argtypes = [
        ctypes.c_uint64,
        ctypes.c_char_p,
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxmf_message_get_title.restype = ctypes.c_int
    lib.lxmf_message_get_content.argtypes = [
        ctypes.c_uint64,
        ctypes.c_char_p,
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxmf_message_get_content.restype = ctypes.c_int
    lib.lxmf_message_fields_json.argtypes = [
        ctypes.c_uint64,
        ctypes.c_char_p,
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.lxmf_message_fields_json.restype = ctypes.c_int
    lib.lxmf_message_field_count.argtypes = [ctypes.c_uint64, ctypes.POINTER(ctypes.c_size_t)]
    lib.lxmf_message_field_count.restype = ctypes.c_int
    lib.lxmf_message_destroy.argtypes = [ctypes.c_uint64]
    lib.lxmf_message_destroy.restype = ctypes.c_int


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
