# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

"""Raw ctypes layer over include/rns.h."""

from __future__ import annotations

import ctypes
import os
from pathlib import Path

HASH_LEN = 16

RNS_OK = 0
RNS_ERR_INVALID_ARG = 1
RNS_ERR_INVALID_HANDLE = 2
RNS_ERR_NOT_FOUND = 3
RNS_ERR_STATE = 4
RNS_ERR_IO = 5
RNS_ERR_INTERNAL = 6
RNS_ERR_TIMEOUT = 7
RNS_ERR_TRUNCATED = 8

RNS_EV_DESTINATION_DATA = 11


class RnsEvent(ctypes.Structure):
    _fields_ = [
        ("kind", ctypes.c_int),
        ("link_id", ctypes.c_uint8 * HASH_LEN),
        ("link_id_len", ctypes.c_size_t),
        ("destination_hash", ctypes.c_uint8 * HASH_LEN),
        ("destination_hash_len", ctypes.c_size_t),
        ("identity_hash", ctypes.c_uint8 * HASH_LEN),
        ("identity_hash_len", ctypes.c_size_t),
        ("request_id", ctypes.c_uint8 * HASH_LEN),
        ("request_id_len", ctypes.c_size_t),
        ("hops", ctypes.c_uint8),
        ("path", ctypes.c_char * 256),
        ("path_truncated", ctypes.c_int),
        ("error_message", ctypes.c_char * 256),
        ("error_message_truncated", ctypes.c_int),
        ("app_data", ctypes.POINTER(ctypes.c_uint8)),
        ("app_data_len", ctypes.c_size_t),
        ("app_data_cap", ctypes.c_size_t),
        ("app_data_truncated", ctypes.c_int),
    ]


class RnsPathEntry(ctypes.Structure):
    _fields_ = [
        ("hash", ctypes.c_uint8 * HASH_LEN),
        ("hash_len", ctypes.c_size_t),
        ("via", ctypes.c_uint8 * HASH_LEN),
        ("via_len", ctypes.c_size_t),
        ("hops", ctypes.c_uint8),
        ("iface", ctypes.c_char * 64),
        ("timestamp", ctypes.c_double),
        ("expires", ctypes.c_double),
    ]


class RnsInterfaceEntry(ctypes.Structure):
    _fields_ = [
        ("name", ctypes.c_char * 96),
        ("type_name", ctypes.c_char * 32),
        ("online", ctypes.c_int),
        ("enabled", ctypes.c_int),
        ("rx_bytes", ctypes.c_uint64),
        ("tx_bytes", ctypes.c_uint64),
        ("rx_packets", ctypes.c_uint64),
        ("tx_packets", ctypes.c_uint64),
    ]


def _candidates() -> list[Path]:
    out: list[Path] = []
    env = os.environ.get("RNS_LIB_PATH")
    if env:
        out.append(Path(env))
    root = os.environ.get("RNS_ROOT")
    if root:
        out.append(Path(root) / "bin" / "librns.so")
    here = Path(__file__).resolve()
    out.extend(
        [
            here.parents[3] / "bin" / "librns.so",
            here.parents[2] / "bin" / "librns.so",
            Path("bin/librns.so"),
            Path("../bin/librns.so"),
            Path("../../bin/librns.so"),
        ]
    )
    return out


def load_library(path: str | None = None) -> ctypes.CDLL:
    if path:
        return ctypes.CDLL(path)
    for candidate in _candidates():
        if candidate.is_file():
            return ctypes.CDLL(str(candidate))
    return ctypes.CDLL("librns.so")


_lib = load_library()

_lib.rns_version.restype = ctypes.c_char_p
_lib.rns_last_error.argtypes = [
    ctypes.c_char_p,
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_size_t),
]
_lib.rns_last_error.restype = ctypes.c_int

_lib.rns_node_create.argtypes = [ctypes.c_char_p]
_lib.rns_node_create.restype = ctypes.c_uint64
for name in ("rns_node_start", "rns_node_stop", "rns_node_destroy", "rns_node_resume", "rns_node_pause"):
    getattr(_lib, name).argtypes = [ctypes.c_uint64]
    getattr(_lib, name).restype = ctypes.c_int

_lib.rns_node_set_identity.argtypes = [ctypes.c_uint64, ctypes.c_uint64]
_lib.rns_node_set_identity.restype = ctypes.c_int

_lib.rns_identity_generate.argtypes = []
_lib.rns_identity_generate.restype = ctypes.c_uint64
_lib.rns_identity_load.argtypes = [ctypes.c_char_p]
_lib.rns_identity_load.restype = ctypes.c_uint64
_lib.rns_identity_save.argtypes = [ctypes.c_uint64, ctypes.c_char_p]
_lib.rns_identity_save.restype = ctypes.c_int
_lib.rns_identity_destroy.argtypes = [ctypes.c_uint64]
_lib.rns_identity_destroy.restype = ctypes.c_int
_lib.rns_identity_hash.argtypes = [
    ctypes.c_uint64,
    ctypes.c_char_p,
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_size_t),
]
_lib.rns_identity_hash.restype = ctypes.c_int
_lib.rns_identity_hash_bytes.argtypes = [
    ctypes.c_uint64,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_size_t),
]
_lib.rns_identity_hash_bytes.restype = ctypes.c_int
_lib.rns_identity_public_key.argtypes = [
    ctypes.c_uint64,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_size_t),
]
_lib.rns_identity_public_key.restype = ctypes.c_int
_lib.rns_identity_from_public_key.argtypes = [ctypes.POINTER(ctypes.c_uint8), ctypes.c_size_t]
_lib.rns_identity_from_public_key.restype = ctypes.c_uint64
_lib.rns_identity_sign.argtypes = [
    ctypes.c_uint64,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_size_t),
]
_lib.rns_identity_sign.restype = ctypes.c_int
_lib.rns_identity_verify.argtypes = [
    ctypes.c_uint64,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
]
_lib.rns_identity_verify.restype = ctypes.c_int

_lib.rns_rsg_create.argtypes = [
    ctypes.c_uint64,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.c_int,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_size_t),
]
_lib.rns_rsg_create.restype = ctypes.c_int
_lib.rns_rsg_validate.argtypes = [
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
]
_lib.rns_rsg_validate.restype = ctypes.c_int
_lib.rns_rsm_verify.argtypes = [
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_size_t),
]
_lib.rns_rsm_verify.restype = ctypes.c_int

_lib.rns_interfaces.argtypes = [
    ctypes.c_uint64,
    ctypes.POINTER(RnsInterfaceEntry),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_size_t),
]
_lib.rns_interfaces.restype = ctypes.c_int

_lib.rns_event_poll.argtypes = [ctypes.c_uint64, ctypes.POINTER(RnsEvent), ctypes.c_int]
_lib.rns_event_poll.restype = ctypes.c_int

_lib.rns_node_refresh_paths.argtypes = [
    ctypes.c_uint64,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
]
_lib.rns_node_refresh_paths.restype = ctypes.c_int

_lib.rns_destination_create.argtypes = [
    ctypes.c_uint64,
    ctypes.c_uint64,
    ctypes.c_char_p,
    ctypes.POINTER(ctypes.c_char_p),
    ctypes.c_size_t,
    ctypes.c_int,
]
_lib.rns_destination_create.restype = ctypes.c_uint64
_lib.rns_destination_enable_ratchets.argtypes = [ctypes.c_uint64, ctypes.c_char_p]
_lib.rns_destination_enable_ratchets.restype = ctypes.c_int
_lib.rns_destination_enforce_ratchets.argtypes = [ctypes.c_uint64]
_lib.rns_destination_enforce_ratchets.restype = ctypes.c_int
_lib.rns_destination_announce.argtypes = [
    ctypes.c_uint64,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
]
_lib.rns_destination_announce.restype = ctypes.c_int
_lib.rns_destination_destroy.argtypes = [ctypes.c_uint64]
_lib.rns_destination_destroy.restype = ctypes.c_int
_lib.rns_destination_hash.argtypes = [
    ctypes.c_uint64,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_size_t),
]
_lib.rns_destination_hash.restype = ctypes.c_int
_lib.rns_destination_register_request_handler.argtypes = [ctypes.c_uint64, ctypes.c_char_p]
_lib.rns_destination_register_request_handler.restype = ctypes.c_int

_lib.rns_path_request.argtypes = [ctypes.c_uint64, ctypes.POINTER(ctypes.c_uint8)]
_lib.rns_path_request.restype = ctypes.c_int
_lib.rns_path_table.argtypes = [
    ctypes.c_uint64,
    ctypes.POINTER(RnsPathEntry),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_size_t),
    ctypes.c_int,
]
_lib.rns_path_table.restype = ctypes.c_int

_lib.rns_link_open.argtypes = [ctypes.c_uint64, ctypes.POINTER(ctypes.c_uint8)]
_lib.rns_link_open.restype = ctypes.c_uint64
for name in ("rns_link_send", "rns_link_close"):
    getattr(_lib, name).argtypes = [ctypes.c_uint64]
    if name == "rns_link_send":
        getattr(_lib, name).argtypes = [ctypes.c_uint64, ctypes.POINTER(ctypes.c_uint8), ctypes.c_size_t]
    getattr(_lib, name).restype = ctypes.c_int
_lib.rns_link_send_resource.argtypes = [
    ctypes.c_uint64,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.c_char_p,
]
_lib.rns_link_send_resource.restype = ctypes.c_int
_lib.rns_link_id.argtypes = [
    ctypes.c_uint64,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_size_t),
]
_lib.rns_link_id.restype = ctypes.c_int
_lib.rns_link_request.argtypes = [
    ctypes.c_uint64,
    ctypes.c_uint64,
    ctypes.c_char_p,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.c_int,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_size_t),
]
_lib.rns_link_request.restype = ctypes.c_int

_lib.rns_request_respond.argtypes = [
    ctypes.c_uint64,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
]
_lib.rns_request_respond.restype = ctypes.c_int
_lib.rns_request_respond_file.argtypes = [
    ctypes.c_uint64,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
    ctypes.c_char_p,
    ctypes.POINTER(ctypes.c_uint8),
    ctypes.c_size_t,
]
_lib.rns_request_respond_file.restype = ctypes.c_int


def cstr_field(buf) -> str:
    raw = bytes(buf)
    nul = raw.find(b"\x00")
    if nul >= 0:
        raw = raw[:nul]
    return raw.decode("utf-8", errors="replace")


lib = _lib
