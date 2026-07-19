# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

"""Idiomatic Python bindings for the librns C ABI."""

from .destination import Destination
from .errors import Error, last_error, map_code, version
from .event import Event, EventKind, hash_eq
from .identity import Identity
from .interfaces import interfaces_list
from .link import Link, request_respond, request_respond_file
from .node import Node
from .path import PathInfo, path_known, path_request, path_table
from .rsg import rsg_create, rsg_validate, rsm_verify
from .util import hash_to_hex, hex_to_hash

API_VERSION = "1.5"
HASH_LEN = 16

__all__ = [
    "API_VERSION",
    "HASH_LEN",
    "Destination",
    "Error",
    "Event",
    "EventKind",
    "Identity",
    "Link",
    "Node",
    "PathInfo",
    "hash_eq",
    "hash_to_hex",
    "hex_to_hash",
    "interfaces_list",
    "last_error",
    "map_code",
    "path_known",
    "path_request",
    "path_table",
    "request_respond",
    "request_respond_file",
    "rsg_create",
    "rsg_validate",
    "rsm_verify",
    "version",
]
