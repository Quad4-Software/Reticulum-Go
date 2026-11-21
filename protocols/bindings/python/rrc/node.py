# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

from . import ffi
from .errors import Error, map_code
from .identity import Identity


class Node:
    def __init__(self, handle: int) -> None:
        self._handle = int(handle)

    @classmethod
    def create(cls, config_path: str = "") -> Node:
        h = ffi.lib.rrc_node_create(config_path.encode("utf-8"))
        if h == 0:
            raise Error(Error.INTERNAL)
        return cls(h)

    def start(self) -> None:
        map_code(ffi.lib.rrc_node_start(self._handle))

    def stop(self) -> None:
        map_code(ffi.lib.rrc_node_stop(self._handle))

    def set_identity(self, identity: Identity) -> None:
        map_code(ffi.lib.rrc_node_set_identity(self._handle, identity.handle))

    def add_udp_interface(self, name: str, local_addr: str, peer_addr: str) -> None:
        map_code(
            ffi.lib.rrc_node_add_udp_interface(
                self._handle,
                name.encode("utf-8"),
                local_addr.encode("utf-8"),
                peer_addr.encode("utf-8"),
            )
        )

    def has_path(self, dest_hash: bytes) -> bool:
        if len(dest_hash) != ffi.HASH_LEN:
            raise Error(Error.INVALID_ARG)
        arr = (ffi.ctypes.c_uint8 * ffi.HASH_LEN).from_buffer_copy(dest_hash)
        out = ffi.ctypes.c_int(0)
        map_code(
            ffi.lib.rrc_node_has_path(
                self._handle,
                arr,
                ffi.HASH_LEN,
                ffi.ctypes.byref(out),
            )
        )
        return bool(out.value)

    @property
    def handle(self) -> int:
        return self._handle

    def close(self) -> None:
        if self._handle:
            ffi.lib.rrc_node_destroy(self._handle)
            self._handle = 0

    def __enter__(self) -> Node:
        return self

    def __exit__(self, *args: object) -> None:
        self.close()

    def __del__(self) -> None:
        try:
            self.close()
        except Exception:
            pass
