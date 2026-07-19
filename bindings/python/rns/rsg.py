# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

from __future__ import annotations

import ctypes

from . import ffi
from .errors import Error, map_code
from .identity import Identity


def rsg_create(identity: Identity, message: bytes | bytearray | memoryview, embed: bool = True) -> bytes:
    payload = bytes(message)
    msg_ptr = (ctypes.c_uint8 * len(payload)).from_buffer_copy(payload) if payload else None
    needed = ctypes.c_size_t(0)
    code = ffi.lib.rns_rsg_create(
        identity.handle,
        msg_ptr,
        len(payload),
        1 if embed else 0,
        None,
        0,
        ctypes.byref(needed),
    )
    if code not in (Error.OK, Error.TRUNCATED):
        map_code(code)
    if needed.value == 0:
        raise Error(Error.INTERNAL)
    out = (ctypes.c_uint8 * needed.value)()
    written = ctypes.c_size_t(0)
    map_code(
        ffi.lib.rns_rsg_create(
            identity.handle,
            msg_ptr,
            len(payload),
            1 if embed else 0,
            out,
            needed.value,
            ctypes.byref(written),
        )
    )
    return bytes(out[: written.value])


def rsg_validate(
    rsg: bytes | bytearray | memoryview,
    message: bytes | bytearray | memoryview,
    required_signer_hash: bytes | bytearray | memoryview = b"",
) -> None:
    rsg_b = bytes(rsg)
    msg_b = bytes(message)
    req_b = bytes(required_signer_hash)
    rsg_ptr = (ctypes.c_uint8 * len(rsg_b)).from_buffer_copy(rsg_b) if rsg_b else None
    msg_ptr = (ctypes.c_uint8 * len(msg_b)).from_buffer_copy(msg_b) if msg_b else None
    req_ptr = (ctypes.c_uint8 * len(req_b)).from_buffer_copy(req_b) if req_b else None
    map_code(
        ffi.lib.rns_rsg_validate(
            rsg_ptr,
            len(rsg_b),
            msg_ptr,
            len(msg_b),
            req_ptr,
            len(req_b),
        )
    )


def rsm_verify(
    rsm: bytes | bytearray | memoryview,
    required_signer_hash: bytes | bytearray | memoryview = b"",
) -> bytes:
    rsm_b = bytes(rsm)
    req_b = bytes(required_signer_hash)
    rsm_ptr = (ctypes.c_uint8 * len(rsm_b)).from_buffer_copy(rsm_b) if rsm_b else None
    req_ptr = (ctypes.c_uint8 * len(req_b)).from_buffer_copy(req_b) if req_b else None
    needed = ctypes.c_size_t(0)
    code = ffi.lib.rns_rsm_verify(
        rsm_ptr,
        len(rsm_b),
        req_ptr,
        len(req_b),
        None,
        0,
        ctypes.byref(needed),
    )
    if code not in (Error.OK, Error.TRUNCATED):
        map_code(code)
    if needed.value == 0:
        return b""
    out = (ctypes.c_uint8 * needed.value)()
    written = ctypes.c_size_t(0)
    map_code(
        ffi.lib.rns_rsm_verify(
            rsm_ptr,
            len(rsm_b),
            req_ptr,
            len(req_b),
            out,
            needed.value,
            ctypes.byref(written),
        )
    )
    return bytes(out[: written.value])
