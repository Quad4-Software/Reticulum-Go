# SPDX-License-Identifier: LicenseRef-Reticulum

"""Idiomatic Python bindings for the librrc C ABI."""

from .client import Client
from .envelope import Envelope
from .errors import Error, last_error, map_code, version
from .event import Event, EventKind
from .hub import Hub
from .identity import Identity
from .node import Node
from .util import hash_to_hex, hex_to_hash, normalize_room, sanitize_nick

API_VERSION = "1.0"
HASH_LEN = 16

__all__ = [
    "API_VERSION",
    "HASH_LEN",
    "Client",
    "Envelope",
    "Error",
    "Event",
    "EventKind",
    "Hub",
    "Identity",
    "Node",
    "hash_to_hex",
    "hex_to_hash",
    "last_error",
    "map_code",
    "normalize_room",
    "sanitize_nick",
    "version",
]
