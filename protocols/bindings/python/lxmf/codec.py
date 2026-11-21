# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

import ctypes
import json

from .ffi import HASH_LEN, LXMF_OK, lib


class LxmfError(OSError):
    pass


def _check(code: int) -> None:
    if code != LXMF_OK:
        raise LxmfError(f"lxmf error {code}")


def identity_generate() -> int:
    handle = lib.lxmf_identity_generate()
    if handle == 0:
        raise LxmfError("identity generate failed")
    return int(handle)


def identity_destroy(handle: int) -> None:
    _check(lib.lxmf_identity_destroy(handle))


def identity_hash(handle: int) -> bytes:
    return _read_bytes(handle, lib.lxmf_identity_hash)


def identity_public_key(handle: int) -> bytes:
    return _read_variable_bytes(handle, lib.lxmf_identity_public_key, 128)


def identity_delivery_hash(handle: int) -> bytes:
    return _read_bytes(handle, lib.lxmf_identity_delivery_hash)


def identity_register_recall(handle: int) -> None:
    _check(lib.lxmf_identity_register_recall(handle))


def identity_register_recall_source(source_hash: bytes, public_key: bytes) -> None:
    if len(source_hash) != HASH_LEN:
        raise ValueError("source hash must be 16 bytes")
    if not public_key:
        raise ValueError("public key required")
    src_arr = (ctypes.c_uint8 * HASH_LEN).from_buffer_copy(source_hash)
    pk_arr = (ctypes.c_uint8 * len(public_key)).from_buffer_copy(public_key)
    _check(
        lib.lxmf_identity_register_recall_source(
            src_arr,
            HASH_LEN,
            pk_arr,
            len(public_key),
        )
    )


def message_create(dest: bytes, source: bytes, title: str, content: str) -> int:
    if len(dest) != HASH_LEN or len(source) != HASH_LEN:
        raise ValueError("hash must be 16 bytes")
    dest_arr = (ctypes.c_uint8 * HASH_LEN).from_buffer_copy(dest)
    source_arr = (ctypes.c_uint8 * HASH_LEN).from_buffer_copy(source)
    handle = lib.lxmf_message_create(
        dest_arr,
        HASH_LEN,
        source_arr,
        HASH_LEN,
        title.encode("utf-8"),
        content.encode("utf-8"),
    )
    if handle == 0:
        raise LxmfError("message create failed")
    return int(handle)


def message_set_fields_json(message: int, fields: dict | str) -> None:
    payload = fields if isinstance(fields, str) else json.dumps(fields, separators=(",", ":"))
    _check(lib.lxmf_message_set_fields_json(message, payload.encode("utf-8")))


def message_pack(message: int, identity: int) -> bytes:
    out = bytearray(65536)
    written = ctypes.c_size_t(0)
    out_arr = (ctypes.c_uint8 * len(out)).from_buffer(out)
    _check(lib.lxmf_message_pack(message, identity, out_arr, len(out), ctypes.byref(written)))
    return bytes(out[: written.value])


def message_encrypted_payload(message: int) -> bytes:
    return _read_variable_bytes(message, lib.lxmf_message_encrypted_payload, 65536)


def message_unpack(data: bytes) -> int:
    arr = (ctypes.c_uint8 * len(data)).from_buffer_copy(data)
    handle = lib.lxmf_message_unpack(arr, len(data))
    if handle == 0:
        raise LxmfError("message unpack failed")
    return int(handle)


def message_unpack_verified(data: bytes, identity: int = 0) -> int:
    arr = (ctypes.c_uint8 * len(data)).from_buffer_copy(data)
    handle = lib.lxmf_message_unpack_verified(arr, len(data), identity)
    if handle == 0:
        raise LxmfError("message unpack verified failed")
    return int(handle)


def message_get_dest(message: int) -> bytes:
    return _read_message_bytes(message, lib.lxmf_message_get_dest)


def message_get_source(message: int) -> bytes:
    return _read_message_bytes(message, lib.lxmf_message_get_source)


def message_get_title(message: int) -> str:
    return _read_message_string(message, lib.lxmf_message_get_title)


def message_get_content(message: int) -> str:
    return _read_message_string(message, lib.lxmf_message_get_content)


def message_fields_json(message: int) -> dict:
    capacity = 262144
    while capacity <= 16 * 1024 * 1024:
        buf = ctypes.create_string_buffer(capacity)
        written = ctypes.c_size_t(0)
        code = lib.lxmf_message_fields_json(message, buf, capacity, ctypes.byref(written))
        if code == 8:
            capacity *= 2
            continue
        _check(code)
        raw = buf.raw[: written.value].decode("utf-8").rstrip("\x00")
        return json.loads(raw) if raw else {}
    raise LxmfError(8)


def message_field_count(message: int) -> int:
    count = ctypes.c_size_t(0)
    _check(lib.lxmf_message_field_count(message, ctypes.byref(count)))
    return int(count.value)


def message_destroy(message: int) -> None:
    _check(lib.lxmf_message_destroy(message))


def _read_bytes(handle: int, fn) -> bytes:
    return _read_variable_bytes(handle, fn, HASH_LEN)


def _read_variable_bytes(handle: int, fn, capacity: int) -> bytes:
    while capacity <= 16 * 1024 * 1024:
        buf = bytearray(capacity)
        written = ctypes.c_size_t(0)
        arr = (ctypes.c_uint8 * capacity).from_buffer(buf)
        code = fn(handle, arr, capacity, ctypes.byref(written))
        if code == 8:
            capacity *= 2
            continue
        _check(code)
        return bytes(buf[: written.value])
    raise LxmfError(8)


def _read_message_bytes(message: int, fn) -> bytes:
    buf = bytearray(HASH_LEN)
    written = ctypes.c_size_t(0)
    arr = (ctypes.c_uint8 * HASH_LEN).from_buffer(buf)
    _check(fn(message, arr, HASH_LEN, ctypes.byref(written)))
    return bytes(buf[: written.value])


def _read_message_string(message: int, fn) -> str:
    capacity = 4096
    while capacity <= 16 * 1024 * 1024:
        buf = ctypes.create_string_buffer(capacity)
        written = ctypes.c_size_t(0)
        code = fn(message, buf, capacity, ctypes.byref(written))
        if code == 8:
            capacity *= 2
            continue
        _check(code)
        return buf.raw[: written.value].decode("utf-8").rstrip("\x00")
    raise LxmfError(8)
