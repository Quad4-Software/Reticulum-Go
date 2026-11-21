# SPDX-License-Identifier: LicenseRef-Reticulum

"""Python ctypes bindings for liblxmf."""

from .codec import (
    identity_delivery_hash,
    identity_destroy,
    identity_generate,
    identity_hash,
    identity_public_key,
    identity_register_recall,
    identity_register_recall_source,
    message_create,
    message_destroy,
    message_encrypted_payload,
    message_field_count,
    message_fields_json,
    message_get_content,
    message_get_dest,
    message_get_source,
    message_get_title,
    message_pack,
    message_set_fields_json,
    message_unpack,
    message_unpack_verified,
)

API_VERSION = "1.0"
HASH_LEN = 16

__all__ = [
    "API_VERSION",
    "HASH_LEN",
    "identity_delivery_hash",
    "identity_destroy",
    "identity_generate",
    "identity_hash",
    "identity_public_key",
    "identity_register_recall",
    "identity_register_recall_source",
    "message_create",
    "message_destroy",
    "message_encrypted_payload",
    "message_field_count",
    "message_fields_json",
    "message_get_content",
    "message_get_dest",
    "message_get_source",
    "message_get_title",
    "message_pack",
    "message_set_fields_json",
    "message_unpack",
    "message_unpack_verified",
]
