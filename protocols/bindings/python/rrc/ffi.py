# SPDX-License-Identifier: LicenseRef-Reticulum

"""Raw ctypes layer over include/rrc.h."""

from __future__ import annotations

import ctypes
import threading
from pathlib import Path

from common.platform import lib_candidates

HASH_LEN = 16

RRC_OK = 0
RRC_ERR_INVALID_ARG = 1
RRC_ERR_INVALID_HANDLE = 2
RRC_ERR_NOT_FOUND = 3
RRC_ERR_STATE = 4
RRC_ERR_IO = 5
RRC_ERR_INTERNAL = 6
RRC_ERR_TIMEOUT = 7
RRC_ERR_TRUNCATED = 8

RRC_TYPE_MSG = 20

RRC_EV_WELCOME = 1
RRC_EV_JOINED = 2
RRC_EV_PARTED = 3
RRC_EV_MSG = 4
RRC_EV_NOTICE = 5
RRC_EV_ACTION = 6
RRC_EV_ERROR = 7
RRC_EV_PONG = 8
RRC_EV_HELLO = 9
RRC_EV_JOIN = 10
RRC_EV_PART = 11
RRC_EV_CLOSE = 12
RRC_EV_TIMEOUT = 13


class RrcEvent(ctypes.Structure):
    _fields_ = [
        ("kind", ctypes.c_int),
        ("sender", ctypes.c_uint8 * HASH_LEN),
        ("sender_len", ctypes.c_size_t),
        ("peer", ctypes.c_uint8 * HASH_LEN),
        ("peer_len", ctypes.c_size_t),
        ("room", ctypes.c_char * 128),
        ("room_truncated", ctypes.c_int),
        ("nick", ctypes.c_char * 64),
        ("nick_truncated", ctypes.c_int),
        ("body", ctypes.c_char * 1024),
        ("body_truncated", ctypes.c_int),
        ("msg_type", ctypes.c_uint64),
    ]


_lib: ctypes.CDLL | None = None
_lock = threading.Lock()


def load_library(path: str | None = None) -> ctypes.CDLL:
    if path:
        return ctypes.CDLL(path)
    for candidate in lib_candidates("rrc", "RRC_LIB_PATH", Path(__file__).resolve()):
        if candidate.is_file():
            return ctypes.CDLL(str(candidate))
    return ctypes.CDLL(lib_candidates("rrc", "RRC_LIB_PATH", Path(__file__).resolve())[-1].name)


def _configure(lib: ctypes.CDLL) -> None:
    lib.rrc_version.restype = ctypes.c_char_p
    lib.rrc_last_error.argtypes = [
        ctypes.c_char_p,
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.rrc_last_error.restype = ctypes.c_int

    lib.rrc_envelope_create.argtypes = [ctypes.c_uint64, ctypes.POINTER(ctypes.c_uint8), ctypes.c_size_t]
    lib.rrc_envelope_create.restype = ctypes.c_uint64
    lib.rrc_envelope_set_room.argtypes = [ctypes.c_uint64, ctypes.c_char_p]
    lib.rrc_envelope_set_room.restype = ctypes.c_int
    lib.rrc_envelope_set_nick.argtypes = [ctypes.c_uint64, ctypes.c_char_p]
    lib.rrc_envelope_set_nick.restype = ctypes.c_int
    lib.rrc_envelope_set_body_text.argtypes = [ctypes.c_uint64, ctypes.c_char_p]
    lib.rrc_envelope_set_body_text.restype = ctypes.c_int
    lib.rrc_envelope_set_destination.argtypes = [
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
    ]
    lib.rrc_envelope_set_destination.restype = ctypes.c_int
    lib.rrc_envelope_get_type.argtypes = [ctypes.c_uint64, ctypes.POINTER(ctypes.c_uint64)]
    lib.rrc_envelope_get_type.restype = ctypes.c_int
    lib.rrc_envelope_get_sender.argtypes = [
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.rrc_envelope_get_sender.restype = ctypes.c_int
    lib.rrc_envelope_get_room.argtypes = [
        ctypes.c_uint64,
        ctypes.c_char_p,
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.rrc_envelope_get_room.restype = ctypes.c_int
    lib.rrc_envelope_get_nick.argtypes = [
        ctypes.c_uint64,
        ctypes.c_char_p,
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.rrc_envelope_get_nick.restype = ctypes.c_int
    lib.rrc_envelope_get_body_text.argtypes = [
        ctypes.c_uint64,
        ctypes.c_char_p,
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.rrc_envelope_get_body_text.restype = ctypes.c_int
    lib.rrc_envelope_marshal.argtypes = [
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.rrc_envelope_marshal.restype = ctypes.c_int
    lib.rrc_envelope_unmarshal.argtypes = [ctypes.POINTER(ctypes.c_uint8), ctypes.c_size_t]
    lib.rrc_envelope_unmarshal.restype = ctypes.c_uint64
    lib.rrc_envelope_destroy.argtypes = [ctypes.c_uint64]
    lib.rrc_envelope_destroy.restype = ctypes.c_int

    lib.rrc_normalize_room.argtypes = [
        ctypes.c_char_p,
        ctypes.c_char_p,
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.rrc_normalize_room.restype = ctypes.c_int
    lib.rrc_sanitize_nick.argtypes = [
        ctypes.c_char_p,
        ctypes.c_char_p,
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.rrc_sanitize_nick.restype = ctypes.c_int

    lib.rrc_node_create.argtypes = [ctypes.c_char_p]
    lib.rrc_node_create.restype = ctypes.c_uint64
    for name in ("rrc_node_start", "rrc_node_stop", "rrc_node_destroy"):
        getattr(lib, name).argtypes = [ctypes.c_uint64]
        getattr(lib, name).restype = ctypes.c_int
    lib.rrc_node_set_identity.argtypes = [ctypes.c_uint64, ctypes.c_uint64]
    lib.rrc_node_set_identity.restype = ctypes.c_int
    lib.rrc_node_add_udp_interface.argtypes = [ctypes.c_uint64, ctypes.c_char_p, ctypes.c_char_p, ctypes.c_char_p]
    lib.rrc_node_add_udp_interface.restype = ctypes.c_int
    lib.rrc_node_has_path.argtypes = [
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_int),
    ]
    lib.rrc_node_has_path.restype = ctypes.c_int

    lib.rrc_identity_generate.restype = ctypes.c_uint64
    lib.rrc_identity_load.argtypes = [ctypes.c_char_p]
    lib.rrc_identity_load.restype = ctypes.c_uint64
    lib.rrc_identity_save.argtypes = [ctypes.c_uint64, ctypes.c_char_p]
    lib.rrc_identity_save.restype = ctypes.c_int
    lib.rrc_identity_destroy.argtypes = [ctypes.c_uint64]
    lib.rrc_identity_destroy.restype = ctypes.c_int
    lib.rrc_identity_hash.argtypes = [
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.rrc_identity_hash.restype = ctypes.c_int
    lib.rrc_identity_seed_destination.argtypes = [ctypes.c_uint64, ctypes.POINTER(ctypes.c_uint8), ctypes.c_size_t]
    lib.rrc_identity_seed_destination.restype = ctypes.c_int

    lib.rrc_hub_create.argtypes = [ctypes.c_uint64, ctypes.c_uint64, ctypes.c_char_p, ctypes.c_char_p]
    lib.rrc_hub_create.restype = ctypes.c_uint64
    for name in ("rrc_hub_start", "rrc_hub_announce", "rrc_hub_destroy"):
        getattr(lib, name).argtypes = [ctypes.c_uint64]
        getattr(lib, name).restype = ctypes.c_int
    lib.rrc_hub_hash.argtypes = [
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    lib.rrc_hub_hash.restype = ctypes.c_int
    lib.rrc_hub_peer_count.argtypes = [ctypes.c_uint64, ctypes.POINTER(ctypes.c_size_t)]
    lib.rrc_hub_peer_count.restype = ctypes.c_int
    lib.rrc_hub_event_poll.argtypes = [ctypes.c_uint64, ctypes.c_int, ctypes.POINTER(RrcEvent)]
    lib.rrc_hub_event_poll.restype = ctypes.c_int

    lib.rrc_client_dial.argtypes = [
        ctypes.c_uint64,
        ctypes.c_uint64,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.c_char_p,
        ctypes.c_char_p,
        ctypes.c_char_p,
        ctypes.c_int,
    ]
    lib.rrc_client_dial.restype = ctypes.c_uint64
    lib.rrc_client_join.argtypes = [ctypes.c_uint64, ctypes.c_char_p]
    lib.rrc_client_join.restype = ctypes.c_int
    lib.rrc_client_part.argtypes = [ctypes.c_uint64, ctypes.c_char_p]
    lib.rrc_client_part.restype = ctypes.c_int
    lib.rrc_client_send_msg.argtypes = [ctypes.c_uint64, ctypes.c_char_p, ctypes.c_char_p]
    lib.rrc_client_send_msg.restype = ctypes.c_int
    lib.rrc_client_send_notice.argtypes = [ctypes.c_uint64, ctypes.c_char_p, ctypes.c_char_p]
    lib.rrc_client_send_notice.restype = ctypes.c_int
    lib.rrc_client_send_action.argtypes = [ctypes.c_uint64, ctypes.c_char_p, ctypes.c_char_p]
    lib.rrc_client_send_action.restype = ctypes.c_int
    lib.rrc_client_ping.argtypes = [ctypes.c_uint64]
    lib.rrc_client_ping.restype = ctypes.c_int
    lib.rrc_client_close.argtypes = [ctypes.c_uint64]
    lib.rrc_client_close.restype = ctypes.c_int
    lib.rrc_client_event_poll.argtypes = [ctypes.c_uint64, ctypes.c_int, ctypes.POINTER(RrcEvent)]
    lib.rrc_client_event_poll.restype = ctypes.c_int


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
